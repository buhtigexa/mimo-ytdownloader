//go:build unix

package ytdlp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

func TestArchivedMP3ReportsExistingFileAndTagsWithoutDownloading(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, ".mimo-ytdownloader-archive.txt")
	if err := os.WriteFile(archivePath, []byte("youtube wgVBhJlZnys\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Sleeping Satellite [wgVBhJlZnys].mp3")
	if err := os.WriteFile(path, []byte("mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := script(t, "ffprobe", `echo '{"format":{"tags":{"artist":"Tasmin Archer","title":"Sleeping Satellite"}}}'`)
	p := props("/nonexistent/yt-dlp", MP3)
	p.OutputDir, p.ArchiveFile = dir, archivePath
	p.FFmpegLocation = filepath.Join(filepath.Dir(probe), "ffmpeg")
	d := mustNew(t, p)
	got, err := d.Download(t.Context(), domain.Job{URL: "https://www.youtube.com/watch?v=wgVBhJlZnys"})
	if err != nil || got.Kind != domain.AlreadyPresent || got.Path != path || got.Name != "Tasmin Archer - Sleeping Satellite" {
		t.Fatalf("fetched = %+v, err = %v", got, err)
	}
}

func TestArchivedFileMissingAndVideoMode(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive")
	if err := os.WriteFile(archivePath, []byte("youtube wgVBhJlZnys\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Song [wgVBhJlZnys].mp3", "Song [wgVBhJlZnys].mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("media"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := props("/nonexistent/yt-dlp", MP3)
	p.OutputDir, p.ArchiveFile, p.Video = dir, archivePath, true
	d := mustNew(t, p)
	got, err := d.Download(t.Context(), domain.Job{URL: "https://youtu.be/wgVBhJlZnys"})
	if err != nil || got.Path != filepath.Join(dir, "Song [wgVBhJlZnys].mp4") || got.Name != "" {
		t.Fatalf("video fetched = %+v, err = %v", got, err)
	}
	missing := mustNew(t, p)
	missing.outputDir = t.TempDir()
	got, err = missing.Download(t.Context(), domain.Job{URL: "https://youtu.be/wgVBhJlZnys"})
	if err != nil || got.Kind != domain.AlreadyPresent || got.Path != "" {
		t.Fatalf("missing file fetched = %+v, err = %v", got, err)
	}
}
