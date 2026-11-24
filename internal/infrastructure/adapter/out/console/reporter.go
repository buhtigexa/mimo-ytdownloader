// Package console is an output adapter implementing out.ProgressReporter on
// a terminal (stderr).
package console

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	portout "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/out"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// Reporter writes one line per event. A mutex keeps lines from different
// goroutines from interleaving.
type Reporter struct {
	mu      sync.Mutex
	w       io.Writer
	verbose bool
	done    atomic.Int64
}

var _ portout.ProgressReporter = (*Reporter)(nil)

// New returns a Reporter writing to w.
func New(w io.Writer, verbose bool) *Reporter {
	return &Reporter{w: w, verbose: verbose}
}

func (r *Reporter) printf(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.w, format, a...)
}

func (r *Reporter) Expanded(playlist domain.Job, videos int) {
	r.printf("  list   line %d: %s -> %d videos\n", playlist.Line, playlist.URL, videos)
}

func (r *Reporter) Duplicate(d domain.Duplicate) {
	r.printf("  dup    line %d: %s (same video as line %d, ignored)\n", d.Job.Line, d.Job.URL, d.FirstLine)
}

func (r *Reporter) Started(worker int, job domain.Job) {
	if r.verbose {
		r.printf("  [w%02d] start  %s\n", worker, job.URL)
	}
}

func (r *Reporter) Retrying(job domain.Job, attempt int, err error) {
	r.printf("  retry  %s (attempt %d failed: %v)\n", job.URL, attempt, err)
}

func (r *Reporter) Finished(o domain.Outcome) {
	n := r.done.Add(1) // input is streamed: the total is unknown until the end
	secs := o.Elapsed.Seconds()
	tries := ""
	if o.Attempts > 1 {
		tries = fmt.Sprintf(" (%d tries)", o.Attempts)
	}

	switch o.Status {
	case domain.StatusSaved:
		r.printf("[%6d] ok      %6.1fs  %s%s\n", n, secs, o.Path, tries)
	case domain.StatusSkipped:
		r.printf("[%6d] skip    %6.1fs  %s (already downloaded)\n", n, secs, o.Job.URL)
	case domain.StatusFailed:
		r.printf("[%6d] FAIL    %6.1fs  line %d: %s -> %v%s\n", n, secs, o.Job.Line, o.Job.URL, o.Err, tries)
	case domain.StatusCanceled:
		r.printf("[%6d] cancel  %6.1fs  %s\n", n, secs, o.Job.URL)
	}
}

func (r *Reporter) Summary(s domain.Summary) {
	r.printf("\ndone in %.1fs: %d saved, %d skipped, %d failed, %d canceled (of %d; %d duplicates ignored)\n",
		s.Elapsed.Seconds(), s.Saved, s.Skipped, s.Failed, s.Canceled, s.Total, s.Duplicates)
}
