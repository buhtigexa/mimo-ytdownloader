package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// SongResolver finds a studio recording and inspects up to three candidates'
// available audio formats before returning a video URL.
type SongResolver struct{ bin string }

func NewSongResolver(bin string) *SongResolver { return &SongResolver{bin: bin} }

type songInfo struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Channel    string       `json:"channel"`
	Uploader   string       `json:"uploader"`
	LiveStatus string       `json:"live_status"`
	IsLive     bool         `json:"is_live"`
	WasLive    bool         `json:"was_live"`
	Entries    []songInfo   `json:"entries"`
	Formats    []songFormat `json:"formats"`
}

type songFormat struct {
	ACodec string  `json:"acodec"`
	VCodec string  `json:"vcodec"`
	ABR    float64 `json:"abr"`
	TBR    float64 `json:"tbr"`
}

// Resolve returns the best verified source at or above 128 kbps. An empty URL
// means that no candidate met the metadata and bitrate rules.
func (r *SongResolver) Resolve(ctx context.Context, artist, title string) (string, error) {
	query := "ytsearch10:" + artist + " " + title + " official audio"
	out, err := run(ctx, r.bin, []string{"--ignore-config", "--flat-playlist", "--no-warnings", "--dump-single-json"}, query)
	if err != nil {
		return "", fmt.Errorf("search %q / %q: %w", artist, title, err)
	}
	var results songInfo
	if err := json.Unmarshal(out, &results); err != nil {
		return "", fmt.Errorf("decode search results: %w", err)
	}
	var candidates []string
	for _, entry := range results.Entries {
		if url := studioURL(entry, artist, title); url != "" {
			candidates = append(candidates, url)
		}
		if len(candidates) == 3 {
			break
		}
	}

	bestURL, bestBitrate := "", 0.0
	inspectErrors := 0
	for _, url := range candidates {
		out, err := run(ctx, r.bin, []string{"--ignore-config", "--no-warnings", "--no-playlist", "--skip-download", "--dump-single-json"}, url)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			inspectErrors++
			continue
		}
		var detail songInfo
		if err := json.Unmarshal(out, &detail); err != nil {
			inspectErrors++
			continue
		}
		if studioURL(detail, artist, title) != url {
			continue
		}
		if bitrate := bestAudioBitrate(detail); bitrate >= 128 && bitrate > bestBitrate {
			bestURL, bestBitrate = url, bitrate
		}
	}
	if bestURL == "" && inspectErrors > 0 {
		return "", fmt.Errorf("could not inspect audio for %q / %q (%d of %d candidates failed)", artist, title, inspectErrors, len(candidates))
	}
	return bestURL, nil
}

func studioURL(info songInfo, artist, title string) string {
	if info.IsLive || info.WasLive || info.LiveStatus == "is_live" || info.LiveStatus == "was_live" || info.LiveStatus == "is_upcoming" {
		return ""
	}
	if _, ok := domain.YouTubeID("https://www.youtube.com/watch?v=" + info.ID); !ok {
		return ""
	}
	name := normalizeSongText(info.Title)
	song := normalizeSongText(title)
	performer := normalizeSongText(artist)
	if song == "" || performer == "" || !containsWords(name, song) {
		return ""
	}
	if !containsWords(name, performer) && !containsWords(normalizeSongText(info.Channel+" "+info.Uploader), performer) {
		return ""
	}
	withoutSong := strings.Replace(" "+name+" ", " "+song+" ", " ", 1)
	for _, marker := range []string{"live", "en vivo", "ao vivo", "directo", "directa", "concert", "concierto", "performance", "unplugged", "acoustic", "acustico", "acustica", "session", "sesion"} {
		if containsWords(withoutSong, marker) {
			return ""
		}
	}
	official := false
	for _, marker := range []string{"official audio", "audio oficial", "official music video", "video oficial", "official video", "studio version"} {
		official = official || containsWords(name, marker)
	}
	if !official {
		return ""
	}
	return "https://www.youtube.com/watch?v=" + info.ID
}

func bestAudioBitrate(info songInfo) float64 {
	audioOnly, combined := 0.0, 0.0
	for _, f := range info.Formats {
		if f.ACodec == "" || f.ACodec == "none" {
			continue
		}
		if f.VCodec == "none" {
			bitrate := f.ABR
			if bitrate == 0 {
				bitrate = f.TBR
			}
			audioOnly = max(audioOnly, bitrate)
		} else {
			combined = max(combined, f.ABR)
		}
	}
	if audioOnly > 0 {
		return audioOnly
	}
	return combined
}

func containsWords(haystack, needle string) bool {
	return strings.Contains(" "+strings.TrimSpace(haystack)+" ", " "+needle+" ")
}

func normalizeSongText(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'á', 'à', 'ä', 'â', 'ã', 'å':
			r = 'a'
		case 'é', 'è', 'ë', 'ê':
			r = 'e'
		case 'í', 'ì', 'ï', 'î':
			r = 'i'
		case 'ó', 'ò', 'ö', 'ô', 'õ':
			r = 'o'
		case 'ú', 'ù', 'ü', 'û':
			r = 'u'
		case 'ñ':
			r = 'n'
		case 'ç':
			r = 'c'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
