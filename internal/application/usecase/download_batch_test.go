package usecase

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/application/model"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// scripted returns, per URL, a list of results consumed in order.
type scripted struct {
	mu    sync.Mutex
	steps map[string][]step
	calls atomic.Int64
}

type step struct {
	f   domain.Fetched
	err error
}

func (s *scripted) Download(_ context.Context, job domain.Job) (domain.Fetched, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.steps[job.URL][0]
	s.steps[job.URL] = s.steps[job.URL][1:]
	return st.f, st.err
}

// hang blocks until ctx is done, like a slow yt-dlp.
type hang struct{ calls atomic.Int64 }

func (h *hang) Download(ctx context.Context, _ domain.Job) (domain.Fetched, error) {
	h.calls.Add(1)
	<-ctx.Done()
	return domain.Fetched{}, ctx.Err()
}

// recorder is a ProgressReporter that remembers retries and expansions.
type recorder struct {
	retries  atomic.Int64
	mu       sync.Mutex
	expanded map[string]int
}

func (r *recorder) Expanded(j domain.Job, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.expanded == nil {
		r.expanded = map[string]int{}
	}
	r.expanded[j.URL] = n
}

func (*recorder) Duplicate(domain.Duplicate)        {}
func (*recorder) Started(int, domain.Job)           {}
func (r *recorder) Retrying(domain.Job, int, error) { r.retries.Add(1) }
func (*recorder) Finished(domain.Outcome)           {}
func (*recorder) Summary(domain.Summary)            {}

func saved(p string) step                { return step{f: domain.Fetched{Kind: domain.Saved, Path: p}} }
func present() step                      { return step{f: domain.Fetched{Kind: domain.AlreadyPresent}} }
func failWith(base error, m string) step { return step{err: fmt.Errorf("%w: %s", base, m)} }

// seq turns a slice into the streaming input the use case consumes.
func seq(js []domain.Job) iter.Seq2[domain.Job, error] {
	return func(yield func(domain.Job, error) bool) {
		for _, j := range js {
			if !yield(j, nil) {
				return
			}
		}
	}
}

// run executes the use case and fails the test on a read error.
func run(t *testing.T, uc *DownloadBatch, ctx context.Context, js []domain.Job) domain.Summary {
	t.Helper()
	s, err := uc.Execute(ctx, seq(js))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return s
}

func jobs(urls ...string) []domain.Job {
	out := make([]domain.Job, len(urls))
	for i, u := range urls {
		out[i] = domain.Job{Line: i + 1, URL: u}
	}
	return out
}

func policy(workers, attempts int) model.DownloadPolicy {
	return model.DownloadPolicy{Workers: workers, Attempts: attempts, Backoff: 5 * time.Second, Timeout: 15 * time.Minute}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name      string
		script    map[string][]step
		workers   int
		attempts  int
		want      domain.Summary // Total, Saved, Skipped, Failed, Canceled, Duplicates
		wantCalls int64
	}{
		{
			name: "mixed results",
			script: map[string][]step{
				"https://a.test/1": {saved("a.mp3")},
				"https://a.test/2": {present()},
				"https://a.test/3": {failWith(domain.ErrPermanent, "private video")},
			},
			workers: 3, attempts: 3,
			want:      domain.Summary{Total: 3, Saved: 1, Skipped: 1, Failed: 1},
			wantCalls: 3,
		},
		{
			name: "transient then ok",
			script: map[string][]step{
				"https://a.test/1": {failWith(domain.ErrTransient, "429"), failWith(domain.ErrTransient, "503"), saved("a.mp3")},
			},
			workers: 1, attempts: 3,
			want:      domain.Summary{Total: 1, Saved: 1},
			wantCalls: 3,
		},
		{
			name: "gives up after max attempts",
			script: map[string][]step{
				"https://a.test/1": {failWith(domain.ErrTransient, "x"), failWith(domain.ErrTransient, "y")},
			},
			workers: 4, attempts: 2,
			want:      domain.Summary{Total: 1, Failed: 1},
			wantCalls: 2,
		},
		{
			name:    "empty input",
			script:  map[string][]step{},
			workers: 8, attempts: 1,
			want: domain.Summary{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// synctest: fake clock, so the 5s backoffs take no real time.
			synctest.Test(t, func(t *testing.T) {
				var urls []string
				for u := range tt.script {
					urls = append(urls, u)
				}
				dl := &scripted{steps: tt.script}
				uc := NewDownloadBatch(dl, nil, &recorder{}, policy(tt.workers, tt.attempts))

				got := run(t, uc, t.Context(), jobs(urls...))
				got.Elapsed = 0

				if got != tt.want {
					t.Errorf("summary = %+v, want %+v", got, tt.want)
				}
				if c := dl.calls.Load(); c != tt.wantCalls {
					t.Errorf("calls = %d, want %d", c, tt.wantCalls)
				}
			})
		})
	}
}

func TestBackoffIsLinear(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dl := &scripted{steps: map[string][]step{
			"https://a.test/1": {failWith(domain.ErrTransient, "a"), failWith(domain.ErrTransient, "b"), saved("x")},
		}}
		start := time.Now()
		run(t, NewDownloadBatch(dl, nil, &recorder{}, policy(1, 3)), t.Context(), jobs("https://a.test/1"))
		// 1*5s + 2*5s of (fake) backoff
		if got := time.Since(start); got != 15*time.Second {
			t.Errorf("elapsed = %v, want 15s", got)
		}
	})
}

func TestSameVideoDownloadedOnce(t *testing.T) {
	dl := &scripted{steps: map[string][]step{"https://youtu.be/dQw4w9WgXcQ": {saved("rick.webm")}}}
	s := run(t, NewDownloadBatch(dl, nil, &recorder{}, policy(3, 1)), t.Context(), jobs(
		"https://youtu.be/dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://m.youtube.com/shorts/dQw4w9WgXcQ",
	))
	if s.Total != 1 || s.Saved != 1 || s.Duplicates != 2 || dl.calls.Load() != 1 {
		t.Errorf("summary = %+v, calls = %d", s, dl.calls.Load())
	}
}

func TestTimeoutIsRetriedThenFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hang{}
		rec := &recorder{}
		s := run(t, NewDownloadBatch(h, nil, rec, policy(1, 2)), t.Context(), jobs("https://a.test/1"))
		if s.Failed != 1 || h.calls.Load() != 2 || rec.retries.Load() != 1 {
			t.Errorf("summary = %+v, calls = %d, retries = %d", s, h.calls.Load(), rec.retries.Load())
		}
	})
}

func TestCancelStopsEverything(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)

		s := run(t, NewDownloadBatch(&hang{}, nil, &recorder{}, policy(2, 3)), ctx, jobs("https://a.test/1", "https://a.test/2", "https://a.test/3", "https://a.test/4", "https://a.test/5"))

		if s.Canceled != 5 { // 2 in flight + 3 never started
			t.Errorf("canceled = %d, want 5 (%+v)", s.Canceled, s)
		}
		if s.Elapsed != time.Second {
			t.Errorf("elapsed = %v, want 1s", s.Elapsed)
		}
	})
}

func TestManyJobsManyWorkers(t *testing.T) {
	const n = 1000
	steps := make(map[string][]step, n)
	urls := make([]string, n)
	for i := range n {
		urls[i] = fmt.Sprintf("https://a.test/%d", i)
		steps[urls[i]] = []step{saved("x")}
	}
	s := run(t, NewDownloadBatch(&scripted{steps: steps}, nil, &recorder{}, policy(32, 1)), t.Context(), jobs(urls...))
	if s.Saved != n {
		t.Errorf("saved = %d, want %d", s.Saved, n)
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("yt-dlp: %w", domain.ErrTransient), true},
		{context.DeadlineExceeded, true},
		{fmt.Errorf("%w: private", domain.ErrPermanent), false},
		{context.Canceled, false},
		{errors.New("other"), false},
	}
	for _, tt := range tests {
		if got := retryable(tt.err); got != tt.want {
			t.Errorf("retryable(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func BenchmarkExecute(b *testing.B) {
	const n = 256
	urls := make([]string, n)
	for i := range n {
		urls[i] = fmt.Sprintf("https://a.test/%d", i)
	}
	in := jobs(urls...)
	uc := NewDownloadBatch(instant{}, nil, &recorder{}, policy(16, 1))
	b.ReportAllocs()
	for b.Loop() {
		uc.Execute(context.Background(), seq(in))
	}
}

type instant struct{}

func (instant) Download(context.Context, domain.Job) (domain.Fetched, error) {
	return domain.Fetched{Kind: domain.Saved, Path: "x"}, nil
}

// playlists resolves playlist URLs from a fixed table; unknown ones fail.
type playlists struct {
	lists map[string][]string
	errs  map[string]error
	calls atomic.Int64
}

func (p *playlists) Resolve(_ context.Context, url string) ([]string, error) {
	p.calls.Add(1)
	if err, ok := p.errs[url]; ok {
		return nil, err
	}
	return p.lists[url], nil
}

const (
	plA = "https://www.youtube.com/playlist?list=PLaaaa"
	plB = "https://www.youtube.com/playlist?list=PLbbbb"
)

func TestPlaylistsAreExpanded(t *testing.T) {
	v1, v2, v3 := "https://www.youtube.com/watch?v=aaaaaaaaaa1", "https://www.youtube.com/shorts/aaaaaaaaaa2", "https://youtu.be/aaaaaaaaaa3"
	pl := &playlists{lists: map[string][]string{
		plA: {v1, v2},
		plB: {v2, v3}, // v2 repeated across playlists
	}}
	dl := &scripted{steps: map[string][]step{v1: {saved("1")}, v2: {saved("2")}, v3: {present()}}}
	rec := &recorder{}

	s := run(t, NewDownloadBatch(dl, pl, rec, policy(4, 1)), t.Context(), []domain.Job{
		{Line: 1, URL: plA},
		// Same video as v1 inside plA. With streaming, whichever copy reaches
		// the deduper first wins; either way it is downloaded exactly once.
		{Line: 2, URL: v1},
		{Line: 3, URL: plB},
	})

	want := domain.Summary{Total: 3, Saved: 2, Skipped: 1, Duplicates: 2}
	s.Elapsed = 0
	if s != want {
		t.Errorf("summary = %+v, want %+v", s, want)
	}
	if dl.calls.Load() != 3 || pl.calls.Load() != 2 {
		t.Errorf("download calls = %d, resolve calls = %d", dl.calls.Load(), pl.calls.Load())
	}
	if rec.expanded[plA] != 2 || rec.expanded[plB] != 2 {
		t.Errorf("expanded = %v", rec.expanded)
	}
}

func TestPlaylistFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pl := &playlists{errs: map[string]error{
			plA: fmt.Errorf("%w: HTTP Error 400", domain.ErrPermanent),
			plB: fmt.Errorf("%w: HTTP Error 429", domain.ErrTransient),
		}}
		dl := &scripted{steps: map[string][]step{"https://a.test/ok": {saved("x")}}}

		s := run(t, NewDownloadBatch(dl, pl, &recorder{}, policy(2, 3)), t.Context(), []domain.Job{
			{Line: 1, URL: plA}, {Line: 2, URL: "https://a.test/ok"}, {Line: 3, URL: plB},
		})

		s.Elapsed = 0
		if want := (domain.Summary{Total: 3, Saved: 1, Failed: 2}); s != want {
			t.Errorf("summary = %+v, want %+v", s, want)
		}
		if c := pl.calls.Load(); c != 4 { // plA once (permanent), plB 3 times (transient)
			t.Errorf("resolve calls = %d, want 4", c)
		}
	})
}

func TestPlaylistsNotExpandedWhenDisabled(t *testing.T) {
	dl := &scripted{steps: map[string][]step{plA: {saved("whole-playlist")}}}
	s := run(t, NewDownloadBatch(dl, nil, &recorder{}, policy(1, 1)), t.Context(), []domain.Job{{Line: 1, URL: plA}})
	if s.Total != 1 || s.Saved != 1 {
		t.Errorf("summary = %+v", s)
	}
}

func TestCancelDuringExpansion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)
		s := run(t, NewDownloadBatch(&hang{}, hangResolver{}, &recorder{}, policy(2, 3)), ctx, []domain.Job{{Line: 1, URL: plA}, {Line: 2, URL: "https://a.test/x"}})
		if s.Canceled != 2 || s.Failed != 0 {
			t.Errorf("summary = %+v", s)
		}
	})
}

type hangResolver struct{}

func (hangResolver) Resolve(ctx context.Context, _ string) ([]string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestInvalidRowsAreReportedAndSkipped(t *testing.T) {
	dl := &scripted{steps: map[string][]step{"https://a.test/ok": {saved("x")}}}
	in := func(yield func(domain.Job, error) bool) {
		_ = yield(domain.Job{Line: 1, URL: "not-a-url"}, fmt.Errorf("%w: line 1: not a URL", domain.ErrInvalidInput)) &&
			yield(domain.Job{Line: 2, URL: "https://a.test/ok"}, nil)
	}
	s, err := NewDownloadBatch(dl, nil, &recorder{}, policy(2, 1)).Execute(t.Context(), in)
	if err != nil || s.Total != 2 || s.Saved != 1 || s.Failed != 1 {
		t.Errorf("summary = %+v, err = %v", s, err)
	}
}

func TestFatalReadErrorStopsAndIsReturned(t *testing.T) {
	dl := &scripted{steps: map[string][]step{"https://a.test/1": {saved("x")}}}
	boom := errors.New("disk on fire")
	in := func(yield func(domain.Job, error) bool) {
		_ = yield(domain.Job{Line: 1, URL: "https://a.test/1"}, nil) &&
			yield(domain.Job{}, boom) &&
			yield(domain.Job{Line: 3, URL: "https://a.test/never"}, nil)
	}
	s, err := NewDownloadBatch(dl, nil, &recorder{}, policy(1, 1)).Execute(t.Context(), in)
	if !errors.Is(err, boom) || s.Saved != 1 || dl.calls.Load() != 1 {
		t.Errorf("summary = %+v, err = %v, calls = %d", s, err, dl.calls.Load())
	}
}

// An endless input must not be buffered: the reader is held back by the
// bounded channels (backpressure) and stops as soon as ctx is canceled.
func TestEndlessInputIsBackpressured(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var produced int
		endless := func(yield func(domain.Job, error) bool) {
			for i := 0; ; i++ {
				produced++
				if !yield(domain.Job{Line: i + 1, URL: fmt.Sprintf("https://a.test/%d", i)}, nil) {
					return
				}
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Minute, cancel)

		s, err := NewDownloadBatch(&hang{}, nil, &recorder{}, policy(4, 1)).Execute(ctx, endless)

		// 4 downloading + 3 channels of 8 + 1 being sent: a few dozen at most.
		if err != nil || produced > 40 {
			t.Errorf("produced %d jobs (want <= 40), err = %v", produced, err)
		}
		if s.Canceled != s.Total || s.Total == 0 {
			t.Errorf("summary = %+v", s)
		}
	})
}

func TestStreamOrderIndependent(t *testing.T) {
	urls := []string{"https://a.test/1", "https://a.test/2", "https://a.test/3"}
	steps := map[string][]step{}
	for _, u := range urls {
		steps[u] = []step{saved(u)}
	}
	rev := slices.Clone(urls)
	slices.Reverse(rev)
	s := run(t, NewDownloadBatch(&scripted{steps: steps}, nil, &recorder{}, policy(3, 1)), t.Context(), jobs(rev...))
	if s.Saved != 3 {
		t.Errorf("summary = %+v", s)
	}
}
