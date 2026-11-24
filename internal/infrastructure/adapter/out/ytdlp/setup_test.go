package ytdlp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// script writes an executable shell script named name and returns its path.
func script(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewPicksFFmpeg(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name     string
		format   AudioFormat
		location string
		want     string
	}{
		{"original needs no ffmpeg", Original, "", ""},
		{"default ffmpeg on PATH", MP3, "", "ffmpeg"},
		{"location is a folder", MP3, dir, filepath.Join(dir, "ffmpeg")},
		{"location is the binary", MP3, "/opt/ffmpeg", "/opt/ffmpeg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := props("yt-dlp", tt.format)
			p.FFmpegLocation = tt.location
			if got := mustNew(t, p).ffmpeg; got != tt.want {
				t.Errorf("ffmpeg = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewFailsOnUnreadableArchive(t *testing.T) {
	p := props("yt-dlp", Original)
	p.ArchiveFile = t.TempDir() // a folder, not a file
	if _, err := New(p); err == nil || !strings.Contains(err.Error(), "read download archive") {
		t.Errorf("err = %v", err)
	}
}

func TestPreflight(t *testing.T) {
	ytdlp := script(t, "yt-dlp", "echo 2026.11.01")
	goodFFmpeg := script(t, "ffmpeg", "exit 0")
	badFFmpeg := script(t, "ffmpeg", "exit 1")

	tests := []struct {
		name    string
		format  AudioFormat
		ffmpeg  string
		wantErr string
	}{
		{"original skips ffmpeg", Original, "", ""},
		{"working ffmpeg", MP3, goodFFmpeg, ""},
		{"broken ffmpeg", MP3, badFFmpeg, "ffmpeg not usable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := props(ytdlp, tt.format)
			p.FFmpegLocation = tt.ffmpeg
			version, err := mustNew(t, p).Preflight(t.Context())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || version != "2026.11.01" {
				t.Errorf("Preflight() = %q, %v", version, err)
			}
		})
	}
}

func TestBuildArgsOptionalFlags(t *testing.T) {
	p := props("yt-dlp", Original)
	p.ArchiveFile = ""
	p.EmbedMetadata = false
	p.EmbedThumbnail = true
	p.FFmpegLocation = "/opt/ffmpeg"
	p.CookiesFromBrowser = "firefox"
	p.ConcurrentFragments = 0

	args := BuildArgs(p)
	for _, want := range [][]string{
		{"--embed-thumbnail"},
		{"--ffmpeg-location", "/opt/ffmpeg"},
		{"--cookies-from-browser", "firefox"},
		{"--concurrent-fragments", "1"},
	} {
		if !hasSeq(args, want) {
			t.Errorf("args %v lack %v", args, want)
		}
	}
	for _, unwanted := range []string{"--download-archive", "--embed-metadata", "--extract-audio"} {
		if slices.Contains(args, unwanted) {
			t.Errorf("args %v must not contain %s", args, unwanted)
		}
	}
}

func TestResolverPassesCookies(t *testing.T) {
	p := props("yt-dlp", Original)
	p.CookiesFromBrowser = "chrome"
	if r := NewResolver(p); !hasSeq(r.args, []string{"--cookies-from-browser", "chrome"}) {
		t.Errorf("args = %v", r.args)
	}
}

func TestParseAudioFormatAll(t *testing.T) {
	for in, want := range map[string]AudioFormat{
		"original": Original, "best": Original, "mp3": MP3, "m4a": M4A, "aac": M4A,
		"opus": Opus, "flac": FLAC, "wav": WAV,
	} {
		if got, err := ParseAudioFormat(in); got != want || err != nil {
			t.Errorf("ParseAudioFormat(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestLastLine(t *testing.T) {
	for in, want := range map[string]string{
		"":                      "",
		"/out/a.mp3\n":          "/out/a.mp3",
		"noise\n/out/b.mp3\n\n": "/out/b.mp3",
		"  /out/c.mp3  ":        "/out/c.mp3",
	} {
		if got := lastLine([]byte(in)); got != want {
			t.Errorf("lastLine(%q) = %q, want %q", in, got, want)
		}
	}
}
