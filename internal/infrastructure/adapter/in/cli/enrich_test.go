package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/application/model"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

type fakeSongResolver func(context.Context, string, string) (string, error)

func (f fakeSongResolver) Resolve(ctx context.Context, artist, title string) (string, error) {
	return f(ctx, artist, title)
}

type fakeQueryResolver func(context.Context, string) (string, error)

func (f fakeQueryResolver) ResolveQuery(ctx context.Context, query string) (string, error) {
	return f(ctx, query)
}

func TestResolveQueryWithRetry(t *testing.T) {
	calls := 0
	resolver := fakeQueryResolver(func(_ context.Context, query string) (string, error) {
		if query != "Mira a tu alrededor Alejandro Lerner" {
			t.Errorf("query = %q", query)
		}
		calls++
		if calls == 1 {
			return "", domain.ErrTransient
		}
		return "https://youtu.be/aaaaaaaaaaa", nil
	})
	var report strings.Builder
	url, err := ResolveQueryWithRetry(context.Background(), resolver, "Mira a tu alrededor Alejandro Lerner", model.DownloadPolicy{Attempts: 2, Timeout: time.Minute}, &report)
	if err != nil || url != "https://youtu.be/aaaaaaaaaaa" || calls != 2 || !strings.Contains(report.String(), "retry") {
		t.Fatalf("url=%q err=%v calls=%d report=%q", url, err, calls, report.String())
	}
}

func TestEnrichCSVAddsURLAndKeepsUnmatchedSong(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songs.csv")
	if err := os.WriteFile(path, []byte("artist,title,youtube_search\nBlondie,Call Me,search\nPink Floyd,Another Brick in the Wall,search\n"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := fakeSongResolver(func(_ context.Context, artist, _ string) (string, error) {
		if artist == "Blondie" {
			return "https://www.youtube.com/watch?v=abcdefghijk", nil
		}
		return "", nil
	})
	var output strings.Builder
	failed, err := EnrichCSV(context.Background(), path, resolver, &output, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 || !strings.Contains(output.String(), "search [2/2] FAIL") {
		t.Fatalf("failed=%d, output=%s", failed, output.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "artist,title,youtube_search,url\n") ||
		!strings.Contains(string(data), "Blondie,Call Me,search,https://www.youtube.com/watch?v=abcdefghijk") ||
		!strings.Contains(string(data), "Pink Floyd,Another Brick in the Wall,search,\n") {
		t.Fatalf("unexpected enriched CSV: %s", data)
	}
}

func TestEnrichCSVReportsSearchErrorAndKeepsGoing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songs.csv")
	original := []byte("artist,title\nBlondie,Call Me\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := EnrichCSV(context.Background(), path, fakeSongResolver(func(context.Context, string, string) (string, error) {
		return "", errors.New("network unavailable")
	}), &strings.Builder{}, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "artist,title,url") || !strings.Contains(string(data), "Blondie,Call Me,\n") {
		t.Fatalf("failed track not retained in CSV: %s", data)
	}
}

func TestEnrichCSVReportsRetryAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songs.csv")
	if err := os.WriteFile(path, []byte("artist,title\nBlondie,Call Me\n"), 0644); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	resolver := fakeSongResolver(func(context.Context, string, string) (string, error) {
		attempts++
		if attempts == 1 {
			return "", domain.ErrTransient
		}
		return "https://www.youtube.com/watch?v=abcdefghijk", nil
	})
	var output strings.Builder
	failed, err := EnrichCSV(context.Background(), path, resolver, &output, model.DownloadPolicy{Attempts: 3, Timeout: time.Minute})
	if err != nil || failed != 0 || attempts != 2 || !strings.Contains(output.String(), "retry Blondie / Call Me") {
		t.Fatalf("failed=%d attempts=%d err=%v output=%s", failed, attempts, err, output.String())
	}
}

func TestEnrichCSVSkipsOrdinaryInputs(t *testing.T) {
	resolver := fakeSongResolver(func(context.Context, string, string) (string, error) { t.Fatal("unexpected lookup"); return "", nil })
	for name, content := range map[string]string{
		"urls.txt":  "https://youtu.be/abcdefghijk\n",
		"empty.csv": "",
		"urls.csv":  "url\nhttps://youtu.be/abcdefghijk\n",
	} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		failed, err := EnrichCSV(context.Background(), path, resolver, &strings.Builder{}, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
		if err != nil || failed != 0 {
			t.Fatalf("%s: failed=%d err=%v", name, failed, err)
		}
	}
}

func TestEnrichCSVUsesExistingURLAndLookupCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songs.csv")
	content := "artist,title,url\nBlondie,Call Me,https://youtu.be/abcdefghijk\nBlondie,One Way or Another,\nBlondie,One Way or Another,\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	resolver := fakeSongResolver(func(context.Context, string, string) (string, error) {
		calls++
		return "https://youtu.be/bbbbbbbbbbb", nil
	})
	var output strings.Builder
	failed, err := EnrichCSV(context.Background(), path, resolver, &output, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
	if err != nil || failed != 0 || calls != 1 || !strings.Contains(output.String(), "URL already present") {
		t.Fatalf("failed=%d calls=%d err=%v output=%s", failed, calls, err, output.String())
	}
}

func TestEnrichCSVBadRowAndCanceled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songs.csv")
	if err := os.WriteFile(path, []byte("artist,title\nBlondie\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := EnrichCSV(context.Background(), path, nil, &strings.Builder{}, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "lacks artist") {
		t.Fatalf("err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = EnrichCSV(ctx, path, nil, &strings.Builder{}, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestEnrichCSVReportsMissingArtistAndKeepsExistingHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songs.csv")
	if err := os.WriteFile(path, []byte("artist,title,url\n,Call Me,\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	failed, err := EnrichCSV(context.Background(), path, nil, &output, model.DownloadPolicy{Attempts: 1, Timeout: time.Minute})
	if err != nil || failed != 1 || !strings.Contains(output.String(), "missing artist") {
		t.Fatalf("failed=%d err=%v output=%s", failed, err, output.String())
	}
}

func TestResolveWithRetryStopsOnPermanentError(t *testing.T) {
	calls := 0
	resolver := fakeSongResolver(func(context.Context, string, string) (string, error) { calls++; return "", domain.ErrPermanent })
	_, err := resolveWithRetry(context.Background(), resolver, "Blondie", "Call Me", model.DownloadPolicy{Attempts: 3, Timeout: time.Minute}, &strings.Builder{}, 1, 1)
	if !errors.Is(err, domain.ErrPermanent) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
