package out

import "github.com/buhtigexa/mimo-ytdownloader/internal/domain"

// ProgressReporter receives batch events, possibly from several goroutines at
// once. Implementations must be safe for concurrent use. The total is not
// known up front: input is streamed.
type ProgressReporter interface {
	Expanded(playlist domain.Job, videos int)
	Duplicate(d domain.Duplicate)
	Started(worker int, job domain.Job)
	Retrying(job domain.Job, attempt int, err error)
	Finished(o domain.Outcome)
	Summary(s domain.Summary)
}
