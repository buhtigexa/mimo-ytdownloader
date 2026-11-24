package cli

import (
	"errors"
	"io"
	"iter"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

var errDisk = errors.New("disk error")

func TestCSVBrokenQuoteIsReportedAndSkipped(t *testing.T) {
	in := "url\n\"bad\"x\nhttps://a.test/1\n"
	got, err := collect(t, csvJobs(strings.NewReader(in), "url"))
	if err != nil || len(got) != 2 || !got[0].bad || got[1] != (row{3, "https://a.test/1", false}) {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestCSVBrokenHeader(t *testing.T) {
	_, err := collect(t, csvJobs(strings.NewReader("\"url\n"), "url"))
	if err == nil || !strings.Contains(err.Error(), "csv header") {
		t.Errorf("err = %v", err)
	}
}

func TestReadErrorsStopTheStream(t *testing.T) {
	failing := func(prefix string) io.Reader {
		return io.MultiReader(strings.NewReader(prefix), iotest.ErrReader(errDisk))
	}
	tests := map[string]iter.Seq2[domain.Job, error]{
		"text": textJobs(failing("https://a.test/1\n")),
		"csv":  csvJobs(failing("url\nhttps://a.test/1\n"), "url"),
	}
	for name, jobs := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := collect(t, jobs)
			if !errors.Is(err, errDisk) || len(got) != 1 {
				t.Errorf("got %v, %v; want 1 row then %v", got, err, errDisk)
			}
		})
	}
}

// The consumer may stop early (for example on Ctrl-C). Every kind of record
// must respect that and stop producing.
func TestStreamsStopWhenConsumerStops(t *testing.T) {
	tests := map[string]iter.Seq2[domain.Job, error]{
		"text":           textJobs(strings.NewReader("https://a.test/1\nhttps://a.test/2\n")),
		"csv valid":      csvJobs(strings.NewReader("url\nhttps://a.test/1\nhttps://a.test/2\n"), "url"),
		"csv bad quote":  csvJobs(strings.NewReader("url\n\"x\"y\n\"x\"y\n"), "url"),
		"csv no column":  csvJobs(strings.NewReader("a,url\nonly\nonly\n"), "url"),
		"missing file":   Jobs("/nonexistent/list.txt", nil, "url"),
		"text read fail": textJobs(iotest.ErrReader(errDisk)),
	}
	for name, jobs := range tests {
		t.Run(name, func(t *testing.T) {
			n := 0
			for range jobs {
				n++
				break
			}
			if n != 1 {
				t.Errorf("got %d records before stopping, want 1", n)
			}
		})
	}
}
