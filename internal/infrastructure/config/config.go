// Package config loads the YAML file into Config and builds the values the
// rest of the program needs from it ("beans"): the use case policy and the
// yt-dlp adapter properties. Neither the application nor the adapters know
// that YAML exists.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/buhtigexa/mimo-ytdownloader/internal/application/model"
	"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure/adapter/out/ytdlp"
)

// MaxWorkers bounds simultaneous yt-dlp download processes.
const MaxWorkers = 5

// Config mirrors the file one to one.
type Config struct {
	OutputDir        string `yaml:"output_dir"`
	Workers          int    `yaml:"workers"`
	Attempts         int    `yaml:"attempts"`
	RetryBackoffSecs int    `yaml:"retry_backoff_secs"`
	TimeoutSecs      int    `yaml:"timeout_secs"`
	ExpandPlaylists  bool   `yaml:"expand_playlists"`
	Audio            Audio  `yaml:"audio"`
	YtDlp            YtDlp  `yaml:"ytdlp"`
}

// Audio is the `audio:` section.
type Audio struct {
	Format         string `yaml:"format"`
	Quality        string `yaml:"quality"`
	EmbedMetadata  bool   `yaml:"embed_metadata"`
	EmbedThumbnail bool   `yaml:"embed_thumbnail"`
}

// YtDlp is the `ytdlp:` section.
type YtDlp struct {
	Path                string   `yaml:"path"`
	FFmpegLocation      string   `yaml:"ffmpeg_location"`
	OutputTemplate      string   `yaml:"output_template"`
	SkipExisting        bool     `yaml:"skip_existing"`
	ArchiveFile         string   `yaml:"archive_file"`
	ConcurrentFragments int      `yaml:"concurrent_fragments"`
	CookiesFromBrowser  string   `yaml:"cookies_from_browser"`
	ExtraArgs           []string `yaml:"extra_args"`
}

// Default returns the config used for every key the file omits.
func Default() Config {
	return Config{
		Workers:          4,
		Attempts:         3,
		RetryBackoffSecs: 5,
		TimeoutSecs:      900,
		ExpandPlaylists:  true,
		Audio:            Audio{Format: "mp3", Quality: "0", EmbedMetadata: true},
		YtDlp: YtDlp{
			Path:                "yt-dlp",
			OutputTemplate:      "%(title)s [%(id)s].%(ext)s",
			SkipExisting:        true,
			ConcurrentFragments: 1,
		},
	}
}

// Load reads and parses the file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

// Parse decodes (rejecting unknown keys), validates and normalizes YAML.
func Parse(data []byte) (Config, error) {
	c := Default()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("yaml: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	c.normalize()
	return c, nil
}

// Validate checks required fields and ranges.
func (c Config) Validate() error {
	switch {
	case strings.TrimSpace(c.OutputDir) == "":
		return errors.New("output_dir is required")
	case c.Workers < 1 || c.Workers > MaxWorkers:
		return fmt.Errorf("workers must be 1..%d, got %d", MaxWorkers, c.Workers)
	case c.Attempts < 1:
		return errors.New("attempts must be >= 1")
	case c.RetryBackoffSecs < 0:
		return errors.New("retry_backoff_secs must be >= 0")
	case c.TimeoutSecs < 1:
		return errors.New("timeout_secs must be >= 1")
	case strings.TrimSpace(c.YtDlp.Path) == "":
		return errors.New("ytdlp.path is empty")
	case strings.TrimSpace(c.YtDlp.OutputTemplate) == "":
		return errors.New("ytdlp.output_template is empty")
	}
	if _, err := ytdlp.ParseAudioFormat(c.Audio.Format); err != nil {
		return fmt.Errorf("audio.format: %w", err)
	}
	return nil
}

// normalize expands ~, derives the default archive path, clamps small values.
func (c *Config) normalize() {
	c.OutputDir = expandHome(c.OutputDir)
	c.YtDlp.Path = expandHome(c.YtDlp.Path)
	c.YtDlp.FFmpegLocation = expandHome(c.YtDlp.FFmpegLocation)
	switch {
	case !c.YtDlp.SkipExisting:
		c.YtDlp.ArchiveFile = ""
	case c.YtDlp.ArchiveFile != "":
		c.YtDlp.ArchiveFile = expandHome(c.YtDlp.ArchiveFile)
	default:
		c.YtDlp.ArchiveFile = filepath.Join(c.OutputDir, ".mimo-ytdownloader-archive.txt")
		legacy := filepath.Join(c.OutputDir, ".ytgrab-archive.txt")
		if _, err := os.Stat(c.YtDlp.ArchiveFile); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(legacy); err == nil {
				c.YtDlp.ArchiveFile = legacy
			}
		}
	}
	c.YtDlp.ConcurrentFragments = max(c.YtDlp.ConcurrentFragments, 1)
}

// DownloadPolicy is the use case bean.
func (c Config) DownloadPolicy() model.DownloadPolicy {
	return model.DownloadPolicy{
		Workers:  c.Workers,
		Attempts: c.Attempts,
		Backoff:  time.Duration(c.RetryBackoffSecs) * time.Second,
		Timeout:  time.Duration(c.TimeoutSecs) * time.Second,
	}
}

// YtDlpProperties is the yt-dlp adapter bean.
func (c Config) YtDlpProperties() ytdlp.Properties {
	format, _ := ytdlp.ParseAudioFormat(c.Audio.Format) // validated in Parse
	return ytdlp.Properties{
		Bin:                 c.YtDlp.Path,
		OutputDir:           c.OutputDir,
		OutputTemplate:      c.YtDlp.OutputTemplate,
		Format:              format,
		Quality:             c.Audio.Quality,
		EmbedMetadata:       c.Audio.EmbedMetadata,
		EmbedThumbnail:      c.Audio.EmbedThumbnail,
		ArchiveFile:         c.YtDlp.ArchiveFile,
		FFmpegLocation:      c.YtDlp.FFmpegLocation,
		ConcurrentFragments: c.YtDlp.ConcurrentFragments,
		CookiesFromBrowser:  c.YtDlp.CookiesFromBrowser,
		ExtraArgs:           c.YtDlp.ExtraArgs,
	}
}

// expandHome turns "~/x" into "$HOME/x". Anything else is returned untouched.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
}
