package console

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

func TestReporter(t *testing.T) {
	var b strings.Builder
	r := New(&b, true)
	r.Expanded(domain.Job{Line: 4, URL: "pl"}, 15)
	r.Duplicate(domain.Duplicate{Job: domain.Job{Line: 3, URL: "u3"}, FirstLine: 1})
	r.Started(2, domain.Job{Line: 1, URL: "u1"})
	r.Finished(domain.Outcome{Job: domain.Job{Line: 1, URL: "u1"}, Status: domain.StatusSaved, Path: "/a.mp3", Attempts: 2, Elapsed: 1500 * time.Millisecond})
	r.Finished(domain.Outcome{Job: domain.Job{Line: 2, URL: "u2"}, Status: domain.StatusFailed, Err: errors.New("boom"), Attempts: 1})
	r.Summary(domain.Summary{Total: 12, Saved: 1, Failed: 1, Duplicates: 1})

	for _, want := range []string{
		"dup    line 3: u3 (same video as line 1, ignored)",
		"list   line 4: pl -> 15 videos",
		"[w02] start  line 1: u1",
		"[     1] ok         1.5s  u1 -> /a.mp3 (2 tries)",
		"[     2] FAIL",
		"line 2: u2 -> boom",
		"1 saved, 0 skipped, 1 failed, 0 canceled (of 12; 1 duplicates ignored)",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("output missing %q:\n%s", want, b.String())
		}
	}
}

func TestReporterShowsEachTrackWithoutVerbose(t *testing.T) {
	var b strings.Builder
	r := New(&b, false)
	r.SetSearchFailures(1)
	job := domain.Job{Line: 4, URL: "https://youtu.be/abcdefghijk", Label: "Blondie / Call Me"}
	r.Started(1, job)
	r.Retrying(job, 1, errors.New("timeout"))
	r.Finished(domain.Outcome{Job: job, Status: domain.StatusSkipped})
	r.Finished(domain.Outcome{Job: job, Status: domain.StatusCanceled})
	r.Summary(domain.Summary{Total: 2, Skipped: 1, Canceled: 1})
	for _, want := range []string{"start  line 4: Blondie / Call Me", "retry  Blondie / Call Me", "skip", "cancel", "1 failed", "(of 3;"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in %s", want, b.String())
		}
	}
}
