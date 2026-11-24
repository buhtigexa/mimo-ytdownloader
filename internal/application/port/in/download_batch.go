// Package in declares the input ports: the operations the system offers to
// driving adapters (CLI, HTTP, scheduler...).
package in

import (
	"context"
	"iter"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// DownloadBatchUseCase downloads every job of a stream, at most once per video.
//
// jobs is consumed lazily, so inputs of any size are processed with bounded
// memory. A per-record error wrapping domain.ErrInvalidInput is reported as a
// failed job and the batch carries on; any other error stops reading and is
// returned. Per-job download errors end up in the Summary, not in the error.
type DownloadBatchUseCase interface {
	Execute(ctx context.Context, jobs iter.Seq2[domain.Job, error]) (domain.Summary, error)
}
