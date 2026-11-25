package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	portout "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/out"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// waitDelay bounds how long we wait for pipes after the process is told to
// stop (a grandchild like ffmpeg may keep them open).
const waitDelay = 3 * time.Second

// Downloader runs one yt-dlp process per job, except for videos already in
// the download archive, which are answered from memory. Safe for concurrent
// use: bin, args and ffmpeg are read-only after New; archive has its own lock.
type Downloader struct {
	bin     string
	args    []string // built once, shared read-only by every call
	ffmpeg  string   // binary to preflight; "" when not transcoding
	archive *archive
}

var _ portout.AudioDownloader = (*Downloader)(nil)

// New builds the adapter and loads the download archive (if any) into memory.
func New(p Properties) (*Downloader, error) {
	a, err := loadArchive(p.ArchiveFile)
	if err != nil {
		return nil, fmt.Errorf("read download archive: %w", err)
	}
	d := &Downloader{bin: p.Bin, args: BuildArgs(p), archive: a}
	if p.Video || p.Format.NeedsFFmpeg() {
		d.ffmpeg = "ffmpeg"
		if p.FFmpegLocation != "" {
			d.ffmpeg = p.FFmpegLocation
			if isDir(p.FFmpegLocation) {
				d.ffmpeg = filepath.Join(p.FFmpegLocation, "ffmpeg")
			}
		}
	}
	return d, nil
}

// isDir reports whether p is an existing directory.
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Archived returns how many videos the archive already holds.
func (d *Downloader) Archived() int { return d.archive.len() }

// Preflight fails fast, before any worker starts. It returns the yt-dlp version.
func (d *Downloader) Preflight(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, d.bin, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp not usable (install it or set ytdlp.path): %w", err)
	}
	if d.ffmpeg != "" {
		if err := exec.CommandContext(ctx, d.ffmpeg, "-version").Run(); err != nil {
			return "", fmt.Errorf("ffmpeg not usable for the selected download mode (install it or set ytdlp.ffmpeg_location): %w", err)
		}
	}
	return string(bytes.TrimSpace(out)), nil
}

// Download implements out.AudioDownloader. The process (and its children,
// see proc_unix.go) is stopped as soon as ctx is done.
func (d *Downloader) Download(ctx context.Context, job domain.Job) (domain.Fetched, error) {
	id, isYouTube := domain.YouTubeID(job.URL)
	if isYouTube && d.archive.has(id) {
		return domain.Fetched{Kind: domain.AlreadyPresent}, nil // no process spawned
	}

	stdout, err := run(ctx, d.bin, d.args, job.URL)
	if err != nil {
		return domain.Fetched{}, err
	}

	// --print after_move:filepath prints the final path; silence == archive hit.
	if path := lastLine(stdout); path != "" {
		path, err = filepath.Abs(path)
		if err != nil {
			return domain.Fetched{}, fmt.Errorf("resolve downloaded file path: %w", err)
		}
		if isYouTube {
			d.archive.add(id)
		}
		return domain.Fetched{Kind: domain.Saved, Path: path}, nil
	}
	return domain.Fetched{Kind: domain.AlreadyPresent}, nil
}

// run executes `bin args... -- url` bound to ctx: when ctx is done the whole
// process group is stopped (see proc_unix.go). It returns stdout, or an error
// that is ctx.Err() or wraps domain.ErrTransient / domain.ErrPermanent.
func run(ctx context.Context, bin string, args []string, url string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	argv := make([]string, 0, len(args)+2)
	argv = append(argv, args...)
	argv = append(argv, "--", url)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = waitDelay
	killGroupOnCancel(cmd)

	err := cmd.Run()
	if cerr := ctx.Err(); cerr != nil {
		return nil, cerr // canceled or attempt deadline exceeded
	}
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return nil, classify(stderr.Bytes(), exitErr.ExitCode())
		}
		return nil, fmt.Errorf("%w: run yt-dlp: %v", domain.ErrPermanent, err)
	}
	return stdout.Bytes(), nil
}

// BuildArgs turns Properties into yt-dlp argv (without the URL).
func BuildArgs(p Properties) []string {
	args := make([]string, 0, 40)
	args = append(args, "--no-playlist", "--no-progress", "--ignore-config")
	if p.Video {
		args = append(args, "--format", "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4]/bv*+ba/b", "--merge-output-format", "mp4")
	} else {
		args = append(args, "--format", "bestaudio/best")
	}
	args = append(args, "--paths", p.OutputDir)
	args = append(args, "--output", p.OutputTemplate)
	args = append(args, "--print", "after_move:filepath")
	args = append(args, "--concurrent-fragments", strconv.Itoa(max(p.ConcurrentFragments, 1)))
	// Retries inside yt-dlp are cheap (same process); the use case retries on top.
	args = append(args, "--retries", "3", "--fragment-retries", "3")

	if p.ArchiveFile != "" {
		args = append(args, "--download-archive", p.ArchiveFile)
	}
	if !p.Video && p.Format.NeedsFFmpeg() {
		args = append(args, "--extract-audio", "--audio-format", string(p.Format), "--audio-quality", p.Quality)
	}
	if p.EmbedMetadata {
		args = append(args, "--embed-metadata")
	}
	if p.EmbedThumbnail {
		args = append(args, "--embed-thumbnail")
	}
	if p.FFmpegLocation != "" {
		args = append(args, "--ffmpeg-location", p.FFmpegLocation)
	}
	if p.CookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", p.CookiesFromBrowser)
	}
	return append(args, p.ExtraArgs...)
}

func lastLine(b []byte) string {
	b = bytes.TrimSpace(b)
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		b = bytes.TrimSpace(b[i+1:])
	}
	return string(b)
}
