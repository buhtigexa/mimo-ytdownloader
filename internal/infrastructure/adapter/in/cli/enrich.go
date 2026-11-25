package cli

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/application/model"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// SongResolver is the lookup needed when a CSV lists songs rather than URLs.
type SongResolver interface {
	Resolve(context.Context, string, string) (string, error)
}

// QueryResolver searches for a song from a free-text title and artist query.
type QueryResolver interface {
	ResolveQuery(context.Context, string) (string, error)
}

// EnrichCSV fills the url column in a song CSV before the usual download
// pipeline reads it. The original file is replaced atomically only after all
// searches finish, so an interrupted lookup does not leave a partial CSV.
func EnrichCSV(ctx context.Context, path string, resolver SongResolver, report io.Writer, policy model.DownloadPolicy) (int, error) {
	if isDirectURL(path) || !strings.EqualFold(filepath.Ext(path), ".csv") {
		return 0, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return 0, nil
		}
		return 0, fmt.Errorf("read CSV header: %w", err)
	}
	col := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")), name) {
				return i
			}
		}
		return -1
	}
	artistCol, titleCol, urlCol := col("artist"), col("title"), col("url")
	if artistCol < 0 || titleCol < 0 {
		return 0, nil // ordinary URL CSV: retain streaming behavior
	}
	rows, err := r.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("read song CSV: %w", err)
	}
	changed := urlCol < 0
	if changed {
		urlCol = len(header)
		header = append(header, "url")
	}
	cache := make(map[string]string)
	found, failed := 0, 0
	fmt.Fprintf(report, "mimo-ytdownloader: searching %d tracks in %s\n", len(rows), path)
	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if len(row) <= max(artistCol, titleCol) {
			return 0, fmt.Errorf("CSV row %d lacks artist or title", i+2)
		}
		if len(row) <= urlCol {
			row = append(row, make([]string, urlCol+1-len(row))...)
			rows[i] = row
		}
		if strings.TrimSpace(row[urlCol]) != "" {
			found++
			fmt.Fprintf(report, "  search [%d/%d] skip  line %d: %s / %s (URL already present)\n", i+1, len(rows), i+2, row[artistCol], row[titleCol])
			continue
		}
		artist, title := strings.TrimSpace(row[artistCol]), strings.TrimSpace(row[titleCol])
		label := artist + " / " + title
		fmt.Fprintf(report, "  search [%d/%d] start line %d: %s\n", i+1, len(rows), i+2, label)
		if artist == "" || title == "" {
			failed++
			fmt.Fprintf(report, "  search [%d/%d] FAIL  line %d: missing artist or title\n", i+1, len(rows), i+2)
			continue
		}
		key := strings.ToLower(artist) + "\x00" + strings.ToLower(title)
		url, ok := cache[key]
		if !ok {
			url, err = resolveWithRetry(ctx, resolver, artist, title, policy, report, i+1, len(rows))
			if err != nil {
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				failed++
				fmt.Fprintf(report, "  search [%d/%d] FAIL  line %d: %s -> %v\n", i+1, len(rows), i+2, label, err)
				continue
			}
			cache[key] = url
		}
		row[urlCol] = url
		if url == "" {
			failed++
			fmt.Fprintf(report, "  search [%d/%d] FAIL  line %d: %s -> no verified studio video at 128 kbps or higher\n", i+1, len(rows), i+2, label)
		} else {
			found++
			changed = true
			fmt.Fprintf(report, "  search [%d/%d] ok    line %d: %s -> %s\n", i+1, len(rows), i+2, label, url)
		}
	}
	if !changed {
		fmt.Fprintf(report, "mimo-ytdownloader: search complete: %d URLs, %d failed\n", found, failed)
		return failed, nil
	}
	if err := replaceCSV(path, header, rows); err != nil {
		return 0, err
	}
	fmt.Fprintf(report, "mimo-ytdownloader: CSV updated: %d URLs, %d failed (%s)\n", found, failed, path)
	return failed, nil
}

func resolveWithRetry(ctx context.Context, resolver SongResolver, artist, title string, policy model.DownloadPolicy, report io.Writer, current, total int) (string, error) {
	return lookupWithRetry(ctx, func(ctx context.Context) (string, error) {
		return resolver.Resolve(ctx, artist, title)
	}, artist+" / "+title, policy, report, current, total)
}

// ResolveQueryWithRetry applies the same timeout and transient-error policy
// used for song CSV lookups.
func ResolveQueryWithRetry(ctx context.Context, resolver QueryResolver, query string, policy model.DownloadPolicy, report io.Writer) (string, error) {
	return lookupWithRetry(ctx, func(ctx context.Context) (string, error) {
		return resolver.ResolveQuery(ctx, query)
	}, query, policy, report, 1, 1)
}

func lookupWithRetry(ctx context.Context, lookup func(context.Context) (string, error), label string, policy model.DownloadPolicy, report io.Writer, current, total int) (string, error) {
	attempts := max(policy.Attempts, 1)
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
		url, err := lookup(attemptCtx)
		cancel()
		if err == nil || ctx.Err() != nil || attempt == attempts ||
			(!errors.Is(err, domain.ErrTransient) && !errors.Is(err, context.DeadlineExceeded)) {
			return url, err
		}
		fmt.Fprintf(report, "  search [%d/%d] retry %s (attempt %d failed: %v)\n", current, total, label, attempt, err)
		timer := time.NewTimer(policy.Backoff * time.Duration(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	panic("unreachable")
}

func replaceCSV(path string, header []string, rows [][]string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mimo-ytdownloader-*.csv")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	w := csv.NewWriter(tmp)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
