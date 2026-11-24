// Package usecase implements the input ports.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/application/model"
	portin "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/in"
	portout "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/out"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// DownloadBatch downloads a stream of jobs with a pipeline of stages joined by
// small channels:
//
//	read ──► (playlist? ──► resolve ×W) ──► dedupe ──► download ×W ──► collect
//	  │                        │                           │
//	  └── bad rows ────────────┴── failed playlists ───────┴──► results
//
// Every channel holds at most 2W items. When a stage is slow, the stages before
// it wait (backpressure), so memory stays flat no matter how big the input is.
type DownloadBatch struct {
	downloader portout.AudioDownloader
	playlists  portout.PlaylistResolver // nil: playlists are not expanded
	reporter   portout.ProgressReporter
	policy     model.DownloadPolicy
}

var _ portin.DownloadBatchUseCase = (*DownloadBatch)(nil)

// NewDownloadBatch builds the use case. playlists may be nil to turn off
// playlist expansion.
func NewDownloadBatch(
	downloader portout.AudioDownloader,
	playlists portout.PlaylistResolver,
	reporter portout.ProgressReporter,
	policy model.DownloadPolicy,
) *DownloadBatch {
	policy.Workers = max(policy.Workers, 1)
	policy.Attempts = max(policy.Attempts, 1)
	return &DownloadBatch{downloader: downloader, playlists: playlists, reporter: reporter, policy: policy}
}

// pipeline holds the channels and counters of one Execute call.
type pipeline struct {
	lists   chan domain.Job     // playlist URLs waiting to be resolved
	videos  chan domain.Job     // video URLs, before dedupe
	queue   chan domain.Job     // unique videos waiting to be downloaded
	results chan domain.Outcome // final outcome of every job

	readErr    error // written by the reader only
	duplicates int   // written by dedupe only
}

// Execute implements portin.DownloadBatchUseCase.
func (b *DownloadBatch) Execute(ctx context.Context, jobs iter.Seq2[domain.Job, error]) (domain.Summary, error) {
	start := time.Now()
	size := 2 * b.policy.Workers
	p := &pipeline{
		lists:   make(chan domain.Job, size),
		videos:  make(chan domain.Job, size),
		queue:   make(chan domain.Job, size),
		results: make(chan domain.Outcome, size),
	}

	// The reader and the resolvers send to p.videos.
	// Every stage except dedupe sends to p.results.
	var feeders, finishers sync.WaitGroup
	feed := func(stage func()) {
		feeders.Add(1)
		finishers.Go(func() {
			defer feeders.Done()
			stage()
		})
	}

	feed(func() { b.read(ctx, jobs, p) })
	for range b.policy.Workers {
		feed(func() { b.resolveAll(ctx, p) })
	}
	go func() {
		feeders.Wait()
		close(p.videos)
	}()

	go b.dedupe(p)

	for worker := 1; worker <= b.policy.Workers; worker++ {
		finishers.Go(func() { b.downloadAll(ctx, worker, p) })
	}
	go func() {
		finishers.Wait()
		close(p.results)
	}()

	sum := b.collect(p)
	// p.results closes only after every stage is done, so reading readErr and
	// duplicates here is safe.
	sum.Duplicates = p.duplicates
	sum.Elapsed = time.Since(start)
	b.reporter.Summary(sum)
	return sum, p.readErr
}

// read sends playlists to the resolvers and videos straight to dedupe. Bad
// rows become failed outcomes; any other read error stops the batch.
func (b *DownloadBatch) read(ctx context.Context, jobs iter.Seq2[domain.Job, error], p *pipeline) {
	defer close(p.lists)
	for job, err := range jobs {
		if ctx.Err() != nil {
			return // canceled: stop reading
		}
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			p.results <- domain.Outcome{Job: job, Status: domain.StatusFailed, Err: err, Attempts: 1}
		case err != nil:
			p.readErr = fmt.Errorf("read input: %w", err)
			return
		case b.playlists != nil && domain.IsPlaylist(job.URL):
			p.lists <- job
		default:
			p.videos <- job
		}
	}
}

// resolveAll turns each playlist into one job per video.
func (b *DownloadBatch) resolveAll(ctx context.Context, p *pipeline) {
	for playlist := range p.lists {
		videos, failed := b.resolve(ctx, playlist)
		if failed != nil {
			p.results <- *failed
			continue
		}
		for _, v := range videos {
			p.videos <- v
		}
	}
}

// dedupe lets each video through once. It runs in a single goroutine, so the
// Deduper needs no lock.
func (b *DownloadBatch) dedupe(p *pipeline) {
	defer close(p.queue)
	seen := domain.NewDeduper()
	for job := range p.videos {
		if line, dup := seen.Check(job); dup {
			p.duplicates++
			b.reporter.Duplicate(domain.Duplicate{Job: job, FirstLine: line})
			continue
		}
		p.queue <- job
	}
}

// downloadAll downloads queued jobs one by one. After a cancel it keeps
// draining the queue, so no earlier stage gets stuck.
func (b *DownloadBatch) downloadAll(ctx context.Context, worker int, p *pipeline) {
	for job := range p.queue {
		if ctx.Err() != nil {
			p.results <- domain.Outcome{Job: job, Status: domain.StatusCanceled}
			continue
		}
		b.reporter.Started(worker, job)
		p.results <- b.download(ctx, job)
	}
}

// collect counts every outcome until all stages are done.
func (b *DownloadBatch) collect(p *pipeline) domain.Summary {
	var sum domain.Summary
	for o := range p.results {
		sum.Add(o)
		b.reporter.Finished(o)
	}
	sum.Total = sum.Seen()
	return sum
}

func (b *DownloadBatch) resolve(ctx context.Context, playlist domain.Job) ([]domain.Job, *domain.Outcome) {
	start := time.Now()
	var urls []string
	attempts, err := b.withRetry(ctx, playlist, func(ctx context.Context) error {
		var err error
		urls, err = b.playlists.Resolve(ctx, playlist.URL)
		return err
	})
	if err != nil {
		o := failure(ctx, playlist, err)
		o.Attempts, o.Elapsed = attempts, time.Since(start)
		return nil, &o
	}

	b.reporter.Expanded(playlist, len(urls))
	videos := make([]domain.Job, len(urls))
	for i, u := range urls {
		videos[i] = domain.Job{Line: playlist.Line, URL: u, Playlist: playlist.URL}
	}
	return videos, nil
}

func (b *DownloadBatch) download(ctx context.Context, job domain.Job) domain.Outcome {
	start := time.Now()
	var f domain.Fetched
	attempts, err := b.withRetry(ctx, job, func(ctx context.Context) error {
		var err error
		f, err = b.downloader.Download(ctx, job)
		return err
	})

	var o domain.Outcome
	switch {
	case err != nil:
		o = failure(ctx, job, err)
	case f.Kind == domain.AlreadyPresent:
		o = domain.Outcome{Job: job, Status: domain.StatusSkipped}
	default:
		o = domain.Outcome{Job: job, Status: domain.StatusSaved, Path: f.Path}
	}
	o.Attempts, o.Elapsed = attempts, time.Since(start)
	return o
}

// withRetry runs fn with a deadline per attempt. Retryable errors are retried
// with a growing wait (1×, 2×, 3× Backoff...) until fn succeeds, the attempts
// run out, or ctx is done.
func (b *DownloadBatch) withRetry(ctx context.Context, job domain.Job, fn func(context.Context) error) (attempts int, err error) {
	for attempts = 1; ; attempts++ {
		attemptCtx, cancel := context.WithTimeout(ctx, b.policy.Timeout)
		err = fn(attemptCtx)
		cancel()

		if err == nil || canceled(ctx, err) || !retryable(err) || attempts >= b.policy.Attempts {
			return attempts, err
		}
		b.reporter.Retrying(job, attempts, err)
		if err := sleep(ctx, b.policy.Backoff*time.Duration(attempts)); err != nil {
			return attempts, err
		}
	}
}

// failure turns a final error into a failed or canceled outcome.
func failure(ctx context.Context, job domain.Job, err error) domain.Outcome {
	if canceled(ctx, err) {
		return domain.Outcome{Job: job, Status: domain.StatusCanceled}
	}
	return domain.Outcome{Job: job, Status: domain.StatusFailed, Err: err}
}

// canceled reports whether the whole batch was canceled. The child process
// may die from the same Ctrl-C before ctx says so, so both are checked.
func canceled(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
}

// retryable reports whether err is worth another try: a temporary adapter
// error or a timed-out attempt.
func retryable(err error) bool {
	return errors.Is(err, domain.ErrTransient) || errors.Is(err, context.DeadlineExceeded)
}

// sleep waits for d, or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
