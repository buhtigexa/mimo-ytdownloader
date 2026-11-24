package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		args    []string
		want    Flags
		wantErr string
	}{
		{[]string{"-c", "x.yml", "-v", "urls.txt"}, Flags{Config: "x.yml", Input: "urls.txt", Verbose: true, URLColumn: "url"}, ""},
		{[]string{"-config=y.yml", "urls.txt"}, Flags{Config: "y.yml", Input: "urls.txt", URLColumn: "url"}, ""},
		{[]string{"urls.txt"}, Flags{Config: "ytgrab.yml", Input: "urls.txt", URLColumn: "url"}, ""},
		{[]string{"-"}, Flags{Config: "ytgrab.yml", Input: "-", URLColumn: "url"}, ""},
		{[]string{}, Flags{}, "missing input"},
		{[]string{"a", "b"}, Flags{}, "only one input"},
		{[]string{"-nope", "a"}, Flags{}, "not defined"},
		{[]string{"a", "-c"}, Flags{}, "only one input"}, // flag pkg stops at first positional
		{[]string{"-c"}, Flags{}, "needs an argument"},
	}
	for _, tt := range tests {
		got, err := ParseFlags(tt.args, io.Discard)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%v: err = %v, want %q", tt.args, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%v: got %+v, %v; want %+v", tt.args, got, err, tt.want)
		}
	}
	if _, err := ParseFlags([]string{"-h"}, io.Discard); !errors.Is(err, ErrHelp) {
		t.Errorf("-h: err = %v", err)
	}
}

type row struct {
	line int
	url  string
	bad  bool
}

func collect(t *testing.T, seq func(func(domain.Job, error) bool)) ([]row, error) {
	t.Helper()
	var rows []row
	for j, err := range seq {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			rows = append(rows, row{j.Line, j.URL, true})
		case err != nil:
			return rows, err
		default:
			rows = append(rows, row{j.Line, j.URL, false})
		}
	}
	return rows, nil
}

func TestTextJobs(t *testing.T) {
	in := "# beats\nhttps://y.com/watch?v=a\n\nhttps://youtu.be/b   # nice\nyoutube.com/x\n  https://youtu.be/c"
	got, err := collect(t, textJobs(strings.NewReader(in)))
	want := []row{{2, "https://y.com/watch?v=a", false}, {4, "https://youtu.be/b", false}, {5, "youtube.com/x", true}, {6, "https://youtu.be/c", false}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("got %v, %v; want %v", got, err, want)
	}
}

func TestCSVJobs(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		col     string
		want    []row
		wantErr string
	}{
		{
			name: "url column, quoted fields, extra columns",
			in:   "year,artist,URL,title\n1985,a-ha,https://youtu.be/djV11Xbc914,\"Take On Me\"\n1986,\"Bananarama, UK\",https://youtu.be/aaaaaaaaaa1,Venus\n",
			col:  "url",
			want: []row{{2, "https://youtu.be/djV11Xbc914", false}, {3, "https://youtu.be/aaaaaaaaaa1", false}},
		},
		{
			name: "custom column, blank cell skipped, bad url reported",
			in:   "\ufefflink,note\nhttps://a.test/1,x\n,empty\nnot-a-url,y\n",
			col:  "link",
			want: []row{{2, "https://a.test/1", false}, {4, "not-a-url", true}},
		},
		{
			name: "ragged row is reported, stream continues",
			in:   "a,url\nonly-one-field\nx,https://a.test/2\n",
			col:  "url",
			want: []row{{2, "", true}, {3, "https://a.test/2", false}},
		},
		{
			name: "multi-line quoted field keeps correct line numbers",
			in:   "note,url\n\"two\nlines\",https://a.test/3\n",
			col:  "url",
			want: []row{{2, "https://a.test/3", false}},
		},
		{
			name:    "no url column: clear error, no search",
			in:      "year,rank,artist,title\n1980,1,Blondie,Call Me\n",
			col:     "url",
			wantErr: "does not search by artist or title",
		},
		{
			name: "search pages are rejected, stream continues",
			in:   "url\nhttps://www.youtube.com/results?search_query=mecano\nhttps://youtu.be/dQw4w9WgXcQ\n",
			col:  "url",
			want: []row{{2, "https://www.youtube.com/results?search_query=mecano", true}, {3, "https://youtu.be/dQw4w9WgXcQ", false}},
		},
		{name: "empty file", in: "", col: "url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collect(t, csvJobs(strings.NewReader(tt.in), tt.col))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestJobsPicksFormatByExtension(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "list.CSV")
	os.WriteFile(csvPath, []byte("url\nhttps://a.test/1\n"), 0o600)
	txtPath := filepath.Join(dir, "list.txt")
	os.WriteFile(txtPath, []byte("url\nhttps://a.test/1\n"), 0o600) // "url" is a bad line in text mode

	got, err := collect(t, Jobs(csvPath, nil, "url"))
	if err != nil || len(got) != 1 || got[0].bad {
		t.Errorf("csv: %v, %v", got, err)
	}
	got, err = collect(t, Jobs(txtPath, nil, "url"))
	if err != nil || len(got) != 2 || !got[0].bad {
		t.Errorf("text: %v, %v", got, err)
	}
	if _, err := collect(t, Jobs(filepath.Join(dir, "missing.csv"), nil, "url")); err == nil {
		t.Error("want error for missing file")
	}
	got, err = collect(t, Jobs("-", strings.NewReader("https://a.test/x\n"), "url"))
	if err != nil || len(got) != 1 {
		t.Errorf("stdin: %v, %v", got, err)
	}
}

type countingUseCase struct{ got int }

func (c *countingUseCase) Execute(_ context.Context, jobs iter.Seq2[domain.Job, error]) (domain.Summary, error) {
	for _, err := range jobs {
		if err != nil {
			return domain.Summary{}, err
		}
		c.got++
	}
	return domain.Summary{Total: c.got, Saved: c.got}, nil
}

func TestControllerHandle(t *testing.T) {
	file := filepath.Join(t.TempDir(), "urls.csv")
	os.WriteFile(file, []byte("artist,url\nx,https://youtu.be/dQw4w9WgXcQ\ny,https://youtu.be/jNQXAC9IVRw\n"), 0o600)

	uc := &countingUseCase{}
	if s, err := NewController(uc, nil, "url").Handle(t.Context(), file); err != nil || s.Saved != 2 {
		t.Errorf("csv: %+v, %v", s, err)
	}
}

func BenchmarkCSVJobs(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("year,rank,artist,title,url\n")
	for i := range 10000 {
		fmt.Fprintf(&sb, "1985,%d,Some Artist,\"Some, Title\",https://www.youtube.com/watch?v=aaaaaaaa%03d\n", i, i%1000)
	}
	data := sb.String()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		for range csvJobs(strings.NewReader(data), "url") {
		}
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		s    domain.Summary
		want int
	}{
		{domain.Summary{Saved: 3}, ExitOK},
		{domain.Summary{Saved: 1, Failed: 1}, ExitFailed},
		{domain.Summary{Failed: 1, Canceled: 2}, ExitInterrupted},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.s); got != tt.want {
			t.Errorf("ExitCode(%+v) = %d, want %d", tt.s, got, tt.want)
		}
	}
}
