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

// DownloadBatch runs a streaming pipeline with bounded channels between
// stages, so memory stays flat no matter how big the input is:
//
//	read ──► (playlist? ──► resolvers ×W) ──► dedupe ──► downloaders ×W ──► collect
//	  │                         │                                │
//	  └── invalid rows ─────────┴── unresolvable playlists ──────┴──► results
//
// Each arrow is a channel of capacity ~2W: a slow stage pushes back on the
// ones before it (backpressure) instead of buffering the input.
type DownloadBatch struct {
	downloader portout.AudioDownloader
	playlists  portout.PlaylistResolver // nil: playlists are not expanded
	reporter   portout.ProgressReporter
	policy     model.DownloadPolicy
}

// compile-time check that DownloadBatch implements the input port.
var _ portin.DownloadBatchUseCase = (*DownloadBatch)(nil)

// NewDownloadBatch wires the use case. playlists may be nil to disable
// playlist expansion.
func NewDownloadBatch(
	d portout.AudioDownloader,
	playlists portout.PlaylistResolver,
	r portout.ProgressReporter,
	p model.DownloadPolicy,
) *DownloadBatch {
	p.Workers = max(p.Workers, 1)
	p.Attempts = max(p.Attempts, 1)
	return &DownloadBatch{downloader: d, playlists: playlists, reporter: r, policy: p}
}

// Execute implements portin.DownloadBatchUseCase.
func (b *DownloadBatch) Execute(ctx context.Context, jobs iter.Seq2[domain.Job, error]) (domain.Summary, error) {
	start := time.Now()
	w := b.policy.Workers
	buf := 2 * w

	var (
		lists   = make(chan domain.Job, buf)     // playlist URLs to resolve
		expand  = make(chan domain.Job, buf)     // videos, before dedupe
		queue   = make(chan domain.Job, buf)     // unique videos to download
		results = make(chan domain.Outcome, buf) // every final outcome

		readErr    error
		duplicates int
	)

	// Producers of `expand`: the reader and the resolvers.
	var feeders sync.WaitGroup
	// Producers of `results`: reader, resolvers and downloaders.
	var reporters sync.WaitGroup

	// Stage 1: read the input. Playlists go to the resolvers, videos onward.
	feeders.Add(1)
	reporters.Add(1)
	go func() {
		defer reporters.Done()
		defer feeders.Done()
		defer close(lists)
		for job, err := range jobs {
			if ctx.Err() != nil {
				return // canceled: stop reading new input
			}
			switch {
			case errors.Is(err, domain.ErrInvalidInput):
				results <- domain.Outcome{Job: job, Status: domain.StatusFailed, Err: err, Attempts: 1}
			case err != nil:
				readErr = fmt.Errorf("read input: %w", err)
				return
			case b.playlists != nil && domain.IsPlaylist(job.URL):
				lists <- job
			default:
				expand <- job
			}
		}
	}()

	// Stage 2: resolve playlists into videos, W at a time.
	for range w {
		feeders.Add(1)
		reporters.Add(1)
		go func() {
			defer reporters.Done()
			defer feeders.Done()
			for pl := range lists {
				videos, failed := b.resolve(ctx, pl)
				if failed != nil {
					results <- *failed
					continue
				}
				for _, v := range videos {
					expand <- v
				}
			}
		}()
	}
	go func() {
		feeders.Wait()
		close(expand)
	}()

	// Stage 3: drop duplicate videos (single goroutine owns the Deduper).
	go func() {
		defer close(queue)
		d := domain.NewDeduper()
		for job := range expand {
			if line, dup := d.Check(job); dup {
				duplicates++
				b.reporter.Duplicate(domain.Duplicate{Job: job, FirstLine: line})
				continue
			}
			queue <- job
		}
	}()

	// Stage 4: download, W at a time.
	for id := 1; id <= w; id++ {
		reporters.Add(1)
		go func() {
			defer reporters.Done()
			for job := range queue {
				if ctx.Err() != nil {
					// Keep draining so upstream never blocks; already-queued
					// jobs are reported as canceled.
					results <- domain.Outcome{Job: job, Status: domain.StatusCanceled}
					continue
				}
				b.reporter.Started(id, job)
				results <- b.process(ctx, job)
			}
		}()
	}
	go func() {
		reporters.Wait()
		close(results)
	}()

	// Stage 5: collect.
	var sum domain.Summary
	for o := range results {
		sum.Add(o)
		b.reporter.Finished(o)
	}
	// results is closed only after every stage finished, so readErr and
	// duplicates are safe to read here (happens-after via the WaitGroups).
	sum.Total = sum.Seen()
	sum.Duplicates = duplicates
	sum.Elapsed = time.Since(start)
	b.reporter.Summary(sum)
	return sum, readErr
}

func (b *DownloadBatch) resolve(ctx context.Context, playlist domain.Job) ([]domain.Job, *domain.Outcome) {
	start := time.Now()
	var urls []string
	attempts, err := b.withRetry(ctx, playlist, func(actx context.Context) error {
		var err error
		urls, err = b.playlists.Resolve(actx, playlist.URL)
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

func (b *DownloadBatch) process(ctx context.Context, job domain.Job) domain.Outcome {
	start := time.Now()
	var f domain.Fetched
	attempts, err := b.withRetry(ctx, job, func(actx context.Context) error {
		var err error
		f, err = b.downloader.Download(actx, job)
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

// withRetry runs fn with a per-attempt deadline, retrying retryable errors
// with linear backoff until it succeeds, the attempts run out or ctx ends.
func (b *DownloadBatch) withRetry(ctx context.Context, job domain.Job, fn func(context.Context) error) (attempts int, err error) {
	for attempts = 1; ; attempts++ {
		actx, cancel := context.WithTimeout(ctx, b.policy.Timeout)
		err = fn(actx)
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

// failure maps a final error to a failed or canceled outcome.
func failure(ctx context.Context, job domain.Job, err error) domain.Outcome {
	if canceled(ctx, err) {
		return domain.Outcome{Job: job, Status: domain.StatusCanceled}
	}
	return domain.Outcome{Job: job, Status: domain.StatusFailed, Err: err}
}

// canceled: the batch itself was canceled. The child may die from the same
// Ctrl-C before we notice, so trust ctx over the error.
func canceled(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
}

// retryable: transient adapter errors and per-attempt timeouts.
func retryable(err error) bool {
	return errors.Is(err, domain.ErrTransient) || errors.Is(err, context.DeadlineExceeded)
}

// sleep waits d or until ctx is done, whichever comes first.
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
