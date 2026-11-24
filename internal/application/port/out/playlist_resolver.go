package out

import "context"

// PlaylistResolver lists the videos of a playlist without downloading them.
// It must return promptly with ctx.Err() once ctx is done.
//
// Errors should wrap domain.ErrTransient or domain.ErrPermanent.
type PlaylistResolver interface {
	Resolve(ctx context.Context, playlistURL string) (videoURLs []string, err error)
}
