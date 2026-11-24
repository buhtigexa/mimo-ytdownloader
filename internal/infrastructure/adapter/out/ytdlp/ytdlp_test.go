package ytdlp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

func mustNew(t *testing.T, p Properties) *Downloader {
	t.Helper()
	d, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func props(bin string, f AudioFormat) Properties {
	return Properties{
		Bin: bin, OutputDir: "/out", OutputTemplate: "%(title)s.%(ext)s", Format: f, Quality: "0",
		EmbedMetadata: true, ArchiveFile: "/out/.archive", ConcurrentFragments: 4,
		ExtraArgs: []string{"--limit-rate", "1M"},
	}
}

func hasSeq(args, seq []string) bool {
	for i := 0; i+len(seq) <= len(args); i++ {
		if slices.Equal(args[i:i+len(seq)], seq) {
			return true
		}
	}
	return false
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name   string
		format AudioFormat
		want   [][]string
		not    []string
	}{
		{
			name:   "mp3 transcodes",
			format: MP3,
			want: [][]string{
				{"--extract-audio", "--audio-format", "mp3", "--audio-quality", "0"},
				{"--paths", "/out"},
				{"--download-archive", "/out/.archive"},
				{"--limit-rate", "1M"},
			},
		},
		{
			name:   "original keeps stream",
			format: Original,
			want:   [][]string{{"--no-playlist"}, {"--format", "bestaudio/best"}},
			not:    []string{"--extract-audio", "--audio-quality"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := BuildArgs(props("yt-dlp", tt.format))
			for _, seq := range tt.want {
				if !hasSeq(args, seq) {
					t.Errorf("missing %v in %v", seq, args)
				}
			}
			for _, f := range tt.not {
				if slices.Contains(args, f) {
					t.Errorf("unexpected %s", f)
				}
			}
		})
	}
}

func TestParseAudioFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    AudioFormat
		wantErr bool
	}{
		{"mp3", MP3, false}, {" WAV ", WAV, false}, {"best", Original, false}, {"aac", M4A, false}, {"ogg", "", true},
	}
	for _, tt := range tests {
		got, err := ParseAudioFormat(tt.in)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("ParseAudioFormat(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		stderr  string
		kind    error
		wantMsg string
	}{
		{"ERROR: [youtube] x: Private video", domain.ErrPermanent, "[youtube] x: Private video"},
		{"WARNING: w\nERROR: HTTP Error 429: Too Many Requests\n", domain.ErrTransient, "HTTP Error 429"},
		{"ERROR: first\nsome trailing noise\n", domain.ErrPermanent, "first"},
		{"just noise\n", domain.ErrPermanent, "just noise"},
		{"", domain.ErrPermanent, "exited with code 1"},
	}
	for _, tt := range tests {
		err := classify([]byte(tt.stderr), 1)
		if !errors.Is(err, tt.kind) || !strings.Contains(err.Error(), tt.wantMsg) {
			t.Errorf("classify(%q) = %v, want %v containing %q", tt.stderr, err, tt.kind, tt.wantMsg)
		}
	}
}

// fakeYtDlp writes a shell script that behaves like yt-dlp based on the URL.
// "slow" spawns a grandchild (like ffmpeg) and records its pid.
func fakeYtDlp(t *testing.T) (bin, pidFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "yt-dlp")
	pidFile = filepath.Join(dir, "grandchild.pid")
	script := `#!/bin/sh
for last; do :; done
case "$last" in
  *playlist?list=PLok*) printf 'https://www.youtube.com/watch?v=aaaaaaaaaa1\nhttps://www.youtube.com/shorts/aaaaaaaaaa2\n\n' ;;
  *playlist?list=PLbad*) echo 'ERROR: [youtube:tab] PLbad: Unable to download API page: HTTP Error 400: Bad Request' >&2; exit 1 ;;
  *saved*) echo /out/song.mp3 ;;
  *skip*)  : ;;
  *429*)   echo 'ERROR: HTTP Error 429: Too Many Requests' >&2; exit 1 ;;
  *slow*)  sleep 30 & echo $! > ` + pidFile + `; wait ;;
  *)       echo 'ERROR: [youtube] x: Video unavailable' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, pidFile
}

func TestDownloadAgainstFakeBinary(t *testing.T) {
	bin, _ := fakeYtDlp(t)
	d := mustNew(t, props(bin, Original))

	tests := []struct {
		url     string
		timeout time.Duration
		want    domain.Fetched
		wantErr error
	}{
		{"https://saved", 5 * time.Second, domain.Fetched{Kind: domain.Saved, Path: "/out/song.mp3"}, nil},
		{"https://skip", 5 * time.Second, domain.Fetched{Kind: domain.AlreadyPresent}, nil},
		{"https://bad", 5 * time.Second, domain.Fetched{}, domain.ErrPermanent},
		{"https://429", 5 * time.Second, domain.Fetched{}, domain.ErrTransient},
		{"https://slow", 100 * time.Millisecond, domain.Fetched{}, context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), tt.timeout)
			defer cancel()
			start := time.Now()
			got, err := d.Download(ctx, domain.Job{Line: 1, URL: tt.url})
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Errorf("Download = %+v, %v; want %+v, %v", got, err, tt.want, tt.wantErr)
			}
			if el := time.Since(start); el > 2*time.Second {
				t.Errorf("took %v", el)
			}
		})
	}
}

// Cancel must stop yt-dlp AND its grandchildren (ffmpeg) right away.
func TestCancelKillsProcessGroup(t *testing.T) {
	bin, pidFile := fakeYtDlp(t)
	d := mustNew(t, props(bin, Original))

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)

	start := time.Now()
	_, err := d.Download(ctx, domain.Job{Line: 1, URL: "https://slow"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("cancel took %v", el)
	}

	raw, rerr := os.ReadFile(pidFile)
	if rerr != nil {
		t.Fatalf("grandchild pid: %v", rerr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil {
		t.Errorf("grandchild %d still alive after cancel", pid)
	}
}

func TestPreflightMissingBinary(t *testing.T) {
	_, err := mustNew(t, props("/nonexistent/yt-dlp", Original)).Preflight(t.Context())
	if err == nil || !strings.Contains(err.Error(), "yt-dlp not usable") {
		t.Errorf("err = %v", err)
	}
}

func TestResolverAgainstFakeBinary(t *testing.T) {
	bin, _ := fakeYtDlp(t)
	r := NewResolver(props(bin, Original))

	urls, err := r.Resolve(t.Context(), "https://www.youtube.com/playlist?list=PLok")
	want := []string{"https://www.youtube.com/watch?v=aaaaaaaaaa1", "https://www.youtube.com/shorts/aaaaaaaaaa2"}
	if err != nil || !slices.Equal(urls, want) {
		t.Errorf("Resolve = %v, %v; want %v", urls, err, want)
	}

	_, err = r.Resolve(t.Context(), "https://www.youtube.com/playlist?list=PLbad")
	if !errors.Is(err, domain.ErrPermanent) || !strings.Contains(err.Error(), "HTTP Error 400") {
		t.Errorf("bad playlist err = %v", err)
	}

	if !hasSeq(r.args, []string{"--flat-playlist", "--yes-playlist"}) || !hasSeq(r.args, []string{"--print", "url"}) {
		t.Errorf("args = %v", r.args)
	}
}

func TestArchiveSkipsWithoutSpawning(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "archive.txt")
	os.WriteFile(arch, []byte("youtube dQw4w9WgXcQ\nsoundcloud 123\n\n"), 0o600)

	p := props("/nonexistent/yt-dlp", Original) // would fail if spawned
	p.ArchiveFile = arch
	d := mustNew(t, p)

	if d.Archived() != 1 {
		t.Errorf("archived = %d, want 1", d.Archived())
	}
	got, err := d.Download(t.Context(), domain.Job{Line: 1, URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=3"})
	if err != nil || got.Kind != domain.AlreadyPresent {
		t.Errorf("archived video: %+v, %v", got, err)
	}
	if _, err := d.Download(t.Context(), domain.Job{Line: 2, URL: "https://youtu.be/jNQXAC9IVRw"}); err == nil {
		t.Error("non-archived video must spawn yt-dlp (and fail here)")
	}
}

func TestArchiveRecordsSavedVideos(t *testing.T) {
	bin, _ := fakeYtDlp(t)
	d := mustNew(t, props(bin, Original))
	// The fake prints a path for any URL containing "saved".
	if _, err := d.Download(t.Context(), domain.Job{URL: "https://youtu.be/savedaaaaaa"}); err != nil {
		t.Fatal(err)
	}
	if !d.archive.has("savedaaaaaa") {
		t.Error("saved video not recorded in the in-memory archive")
	}
}

func TestMissingArchiveIsEmpty(t *testing.T) {
	p := props("yt-dlp", Original)
	p.ArchiveFile = filepath.Join(t.TempDir(), "nope.txt")
	if d := mustNew(t, p); d.Archived() != 0 {
		t.Error("missing archive must be empty")
	}
}
