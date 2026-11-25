package ytdlp

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// readMP3Name reads the completed file's ID3 tags. Missing or unreadable tags
// only affect the display label; a successful download stays successful.
func readMP3Name(ctx context.Context, ffprobe, path string) string {
	if !strings.EqualFold(filepath.Ext(path), ".mp3") {
		return ""
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, ffprobe, "-v", "error", "-show_entries", "format_tags=artist,title", "-of", "json", path).Output()
	if err != nil {
		return ""
	}
	var result struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if json.Unmarshal(out, &result) != nil {
		return ""
	}
	artist, title := "", ""
	for key, value := range result.Format.Tags {
		value = strings.Join(strings.Fields(value), " ")
		switch {
		case strings.EqualFold(key, "artist"):
			artist = value
		case strings.EqualFold(key, "title"):
			title = value
		}
	}
	if title == "" {
		return ""
	}
	if artist != "" {
		return artist + " - " + title
	}
	return title
}
