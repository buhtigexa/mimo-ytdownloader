package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure/adapter/out/ytdlp"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // "" = must succeed
	}{
		{"minimal", "output_dir: /m\n", ""},
		{"full", "output_dir: /m\nworkers: 5\nattempts: 5\ntimeout_secs: 60\naudio:\n  format: wav\nytdlp:\n  skip_existing: false\n  extra_args: [\"--limit-rate\", \"5M\"]\n", ""},
		{"missing output_dir", "workers: 2\n", "output_dir is required"},
		{"empty file", "", "output_dir is required"},
		{"zero workers", "output_dir: /m\nworkers: 0\n", "workers must be"},
		{"six workers exceed download limit", "output_dir: /m\nworkers: 6\n", "workers must be 1..5"},
		{"too many workers", "output_dir: /m\nworkers: 999\n", "workers must be"},
		{"zero timeout", "output_dir: /m\ntimeout_secs: 0\n", "timeout_secs"},
		{"unknown key", "output_dir: /m\ntypo: 1\n", "not found"},
		{"bad format", "output_dir: /m\naudio:\n  format: ogg\n", "audio.format: unknown audio format"},
		{"bad yaml", "output_dir: [\n", "yaml"},
		{"zero attempts", "output_dir: /m\nattempts: 0\n", "attempts must be"},
		{"negative backoff", "output_dir: /m\nretry_backoff_secs: -1\n", "retry_backoff_secs"},
		{"empty yt-dlp path", "output_dir: /m\nytdlp:\n  path: \" \"\n", "ytdlp.path is empty"},
		{"empty template", "output_dir: /m\nytdlp:\n  output_template: \"\"\n", "ytdlp.output_template is empty"},
		{"custom archive", "output_dir: /m\nytdlp:\n  archive_file: /a.txt\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultsAndBeans(t *testing.T) {
	c, err := Parse([]byte("output_dir: /m\nworkers: 5\naudio:\n  format: wav\n"))
	if err != nil {
		t.Fatal(err)
	}
	p := c.DownloadPolicy()
	if p.Workers != 5 || p.Attempts != 3 || p.Timeout != 900*time.Second || p.Backoff != 5*time.Second {
		t.Errorf("policy = %+v", p)
	}
	if !c.ExpandPlaylists {
		t.Error("expand_playlists must default to true")
	}
	if c2, _ := Parse([]byte("output_dir: /m\nexpand_playlists: false\n")); c2.ExpandPlaylists {
		t.Error("expand_playlists: false ignored")
	}
	yp := c.YtDlpProperties()
	if yp.Format != ytdlp.WAV || yp.ArchiveFile != "/m/.mimo-ytdownloader-archive.txt" || yp.Bin != "yt-dlp" {
		t.Errorf("props = %+v", yp)
	}

	c, _ = Parse([]byte("output_dir: /m\nytdlp:\n  skip_existing: false\n  archive_file: /x\n"))
	if c.YtDlp.ArchiveFile != "" {
		t.Errorf("skip_existing=false must disable the archive, got %q", c.YtDlp.ArchiveFile)
	}
}

func TestArchiveNameKeepsExistingHistory(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".ytgrab-archive.txt")
	if err := os.WriteFile(legacy, []byte("old downloads\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Parse([]byte("output_dir: " + dir + "\n"))
	if err != nil || c.YtDlp.ArchiveFile != legacy {
		t.Fatalf("archive=%q err=%v, want existing %q", c.YtDlp.ArchiveFile, err, legacy)
	}
	current := filepath.Join(dir, ".mimo-ytdownloader-archive.txt")
	if err := os.WriteFile(current, []byte("new downloads\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = Parse([]byte("output_dir: " + dir + "\n"))
	if err != nil || c.YtDlp.ArchiveFile != current {
		t.Fatalf("archive=%q err=%v, want current %q", c.YtDlp.ArchiveFile, err, current)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct{ in, want string }{
		{"~/m", filepath.Join(home, "m")}, {"~", home}, {"/abs", "/abs"}, {"rel", "rel"}, {"", ""},
	}
	for _, tt := range tests {
		if got := expandHome(tt.in); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yml")
	os.WriteFile(p, []byte("output_dir: /m\n"), 0o600)
	if _, err := Load(p); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Error("want error for missing file")
	}
	bad := filepath.Join(t.TempDir(), "bad.yml")
	os.WriteFile(bad, []byte("workers: 2\n"), 0o600)
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("err = %v, want it to name the file", err)
	}
}

func TestExpandHomeWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := expandHome("~/m"); got != "~/m" {
		t.Errorf("expandHome without HOME = %q, want it unchanged", got)
	}
}
