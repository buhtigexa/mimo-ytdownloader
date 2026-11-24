package ytdlp

import (
	"bytes"
	"context"

	portout "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/out"
)

// Resolver implements out.PlaylistResolver with `yt-dlp --flat-playlist`:
// it lists the videos of a playlist without downloading anything (one fast
// request per page of 100 entries).
type Resolver struct {
	bin  string
	args []string // built once, shared read-only by every call
}

var _ portout.PlaylistResolver = (*Resolver)(nil)

// NewResolver builds the adapter from the same Properties as the Downloader.
func NewResolver(p Properties) *Resolver {
	args := []string{
		"--flat-playlist", "--yes-playlist", "--ignore-config", "--no-warnings",
		"--print", "url",
	}
	if p.CookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", p.CookiesFromBrowser)
	}
	return &Resolver{bin: p.Bin, args: args}
}

// Resolve implements out.PlaylistResolver.
func (r *Resolver) Resolve(ctx context.Context, playlistURL string) ([]string, error) {
	out, err := run(ctx, r.bin, r.args, playlistURL)
	if err != nil {
		return nil, err
	}
	var urls []string
	for line := range bytes.Lines(out) {
		if u := string(bytes.TrimSpace(line)); u != "" && u != "NA" {
			urls = append(urls, u)
		}
	}
	return urls, nil
}
