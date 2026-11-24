// Package out declares the output ports: the capabilities the use cases need
// from the outside world. Adapters implement them.
package out

import (
	"context"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// AudioDownloader fetches one job to disk. It is called from many goroutines
// at once and must return promptly with ctx.Err() once ctx is done.
//
// Errors should wrap domain.ErrTransient or domain.ErrPermanent.
type AudioDownloader interface {
	Download(ctx context.Context, job domain.Job) (domain.Fetched, error)
}
