package cli

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// readBuf is the read buffer for input files: big enough for sequential
// throughput, small enough to keep memory flat.
const readBuf = 1 << 20

// Jobs streams the jobs in path ("-" = stdin) without loading the file:
// memory stays constant whatever its size.
//
//   - *.csv: RFC 4180 CSV with a header row; URLs are taken from urlColumn
//     (case-insensitive). Other columns are ignored.
//   - anything else: one URL per line; blank lines and '#' comments ignored,
//     a trailing " # comment" stripped.
//
// A bad record yields an error wrapping domain.ErrInvalidInput and the stream
// continues; I/O errors and a missing URL column end it.
func Jobs(path string, stdin io.Reader, urlColumn string) iter.Seq2[domain.Job, error] {
	return func(yield func(domain.Job, error) bool) {
		var r io.Reader = stdin
		if path != "-" {
			f, err := os.Open(path)
			if err != nil {
				yield(domain.Job{}, err)
				return
			}
			defer f.Close()
			r = f
		}
		br := bufio.NewReaderSize(r, readBuf)

		if strings.EqualFold(filepath.Ext(path), ".csv") {
			csvJobs(br, urlColumn)(yield)
			return
		}
		textJobs(br)(yield)
	}
}

// textJobs parses one URL per line.
func textJobs(r io.Reader) iter.Seq2[domain.Job, error] {
	return func(yield func(domain.Job, error) bool) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for n := 1; sc.Scan(); n++ {
			url, _, _ := strings.Cut(sc.Text(), " #")
			url = strings.TrimSpace(url)
			if url == "" || strings.HasPrefix(url, "#") {
				continue
			}
			if !yield(record(n, url)) {
				return
			}
		}
		if err := sc.Err(); err != nil {
			yield(domain.Job{}, err)
		}
	}
}

// csvJobs parses a CSV with a header row and a URL column.
func csvJobs(r io.Reader, urlColumn string) iter.Seq2[domain.Job, error] {
	return func(yield func(domain.Job, error) bool) {
		cr := csv.NewReader(r)
		cr.ReuseRecord = true   // no per-row slice allocation
		cr.FieldsPerRecord = -1 // tolerate ragged rows; we only need one column
		cr.TrimLeadingSpace = true

		header, err := cr.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return // empty file
			}
			yield(domain.Job{}, fmt.Errorf("csv header: %w", err))
			return
		}
		col := -1
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")), urlColumn) {
				col = i
				break
			}
		}
		if col < 0 {
			yield(domain.Job{}, fmt.Errorf(
				"csv has no %q column (columns: %s). ytgrab downloads URLs; it does not search by artist or title",
				urlColumn, strings.Join(header, ", ")))
			return
		}

		for {
			rec, err := cr.Read()
			if errors.Is(err, io.EOF) {
				return
			}
			line, _ := cr.FieldPos(0)
			if pe, ok := errors.AsType[*csv.ParseError](err); ok {
				if !yield(domain.Job{Line: pe.Line}, fmt.Errorf("%w: line %d: %v", domain.ErrInvalidInput, pe.Line, pe.Err)) {
					return
				}
				continue
			}
			if err != nil {
				yield(domain.Job{}, err)
				return
			}
			if col >= len(rec) {
				if !yield(domain.Job{Line: line}, fmt.Errorf("%w: line %d: missing %q column", domain.ErrInvalidInput, line, urlColumn)) {
					return
				}
				continue
			}
			url := strings.TrimSpace(rec[col])
			if url == "" {
				continue
			}
			// Clone: the field shares memory with the whole row; keeping it
			// would pin every row in memory.
			if !yield(record(line, strings.Clone(url))) {
				return
			}
		}
	}
}

// record validates one URL.
func record(line int, url string) (domain.Job, error) {
	j := domain.Job{Line: line, URL: url}
	if !isHTTPURL(url) {
		return j, fmt.Errorf("%w: line %d: not a URL: %q", domain.ErrInvalidInput, line, url)
	}
	if domain.IsSearchPage(url) {
		return j, fmt.Errorf("%w: line %d: search pages are not downloaded, give a video URL: %q", domain.ErrInvalidInput, line, url)
	}
	return j, nil
}

func isHTTPURL(s string) bool {
	return (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")) &&
		!strings.ContainsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' })
}
