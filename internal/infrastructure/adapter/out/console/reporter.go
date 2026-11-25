// Package console is an output adapter implementing out.ProgressReporter on
// a terminal (stderr).
package console

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	portout "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/out"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// Reporter writes one line per event. A mutex keeps lines from different
// goroutines from interleaving.
type Reporter struct {
	mu           sync.Mutex
	w            io.Writer
	verbose      bool
	done         atomic.Int64
	searchFailed int
	hostDataDir  string
}

var _ portout.ProgressReporter = (*Reporter)(nil)

// New returns a Reporter writing to w.
func New(w io.Writer, verbose bool) *Reporter {
	return &Reporter{w: w, verbose: verbose}
}

// SetSearchFailures includes tracks without a usable URL in the final report.
func (r *Reporter) SetSearchFailures(n int) { r.searchFailed = n }

// SetHostDataDir maps /data paths from Docker to the bind mount on the host.
func (r *Reporter) SetHostDataDir(dir string) { r.hostDataDir = dir }

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
		r.printf("  [w%02d] start  line %d: %s\n", worker, job.Line, trackName(job))
	} else {
		r.printf("  start  line %d: %s\n", job.Line, trackName(job))
	}
}

func (r *Reporter) Retrying(job domain.Job, attempt int, err error) {
	r.printf("  retry  %s (attempt %d failed: %v)\n", trackName(job), attempt, err)
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
		label := trackName(o.Job)
		if o.Name != "" {
			label = "ID3: " + o.Name
		}
		r.printf("[%6d] ok      %6.1fs  %s -> %s%s\n", n, secs, label, r.displayPath(o.Path), tries)
	case domain.StatusSkipped:
		if o.Path != "" {
			label := trackName(o.Job)
			if o.Name != "" {
				label = "ID3: " + o.Name
			}
			r.printf("[%6d] skip    %6.1fs  %s -> %s (already downloaded)\n", n, secs, label, r.displayPath(o.Path))
		} else {
			r.printf("[%6d] skip    %6.1fs  %s (archived; file path unavailable)\n", n, secs, trackName(o.Job))
		}
	case domain.StatusFailed:
		r.printf("[%6d] FAIL    %6.1fs  line %d: %s -> %v%s\n", n, secs, o.Job.Line, trackName(o.Job), o.Err, tries)
	case domain.StatusCanceled:
		r.printf("[%6d] cancel  %6.1fs  %s\n", n, secs, trackName(o.Job))
	}
}

func (r *Reporter) displayPath(path string) string {
	if r.hostDataDir == "" || !filepath.IsAbs(r.hostDataDir) {
		return path
	}
	rel, err := filepath.Rel("/data", path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(r.hostDataDir, rel)
}

func (r *Reporter) Summary(s domain.Summary) {
	s.Failed += r.searchFailed
	s.Total += r.searchFailed
	r.printf("\ndone in %.1fs: %d saved, %d skipped, %d failed, %d canceled (of %d; %d duplicates ignored)\n",
		s.Elapsed.Seconds(), s.Saved, s.Skipped, s.Failed, s.Canceled, s.Total, s.Duplicates)
}

func trackName(job domain.Job) string {
	if job.Label != "" {
		return job.Label
	}
	return job.URL
}
