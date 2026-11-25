// Package ytdlp is an output adapter implementing out.AudioDownloader with
// yt-dlp (+ ffmpeg when transcoding).
package ytdlp

import (
	"fmt"
	"strings"
)

// AudioFormat is what ends up on disk.
type AudioFormat string

const (
	Original AudioFormat = "original" // YouTube's stream untouched: best fidelity, no ffmpeg
	MP3      AudioFormat = "mp3"
	M4A      AudioFormat = "m4a"
	Opus     AudioFormat = "opus"
	FLAC     AudioFormat = "flac"
	WAV      AudioFormat = "wav"
)

// ParseAudioFormat accepts original|best|mp3|m4a|aac|opus|flac|wav.
func ParseAudioFormat(s string) (AudioFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "original", "best":
		return Original, nil
	case "mp3":
		return MP3, nil
	case "m4a", "aac":
		return M4A, nil
	case "opus":
		return Opus, nil
	case "flac":
		return FLAC, nil
	case "wav":
		return WAV, nil
	}
	return "", fmt.Errorf("unknown audio format %q (want original|mp3|m4a|opus|flac|wav)", s)
}

// NeedsFFmpeg reports whether yt-dlp must transcode.
func (f AudioFormat) NeedsFFmpeg() bool { return f != Original }

// Properties is everything the adapter needs, in its own terms.
type Properties struct {
	Bin                 string
	OutputDir           string
	OutputTemplate      string
	Format              AudioFormat
	Video               bool
	Quality             string
	EmbedMetadata       bool
	EmbedThumbnail      bool
	ArchiveFile         string // "" disables the download archive
	FFmpegLocation      string
	ConcurrentFragments int
	CookiesFromBrowser  string
	ExtraArgs           []string
}
