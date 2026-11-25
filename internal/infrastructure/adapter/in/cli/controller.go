package cli

import (
	"context"
	"io"

	portin "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/in"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// Controller translates the command line into a call to the input port and
// the Summary into an exit code. No business rules here.
type Controller struct {
	useCase   portin.DownloadBatchUseCase
	stdin     io.Reader
	urlColumn string
}

// NewController returns a Controller. urlColumn is the CSV column holding URLs.
func NewController(uc portin.DownloadBatchUseCase, stdin io.Reader, urlColumn string) *Controller {
	return &Controller{useCase: uc, stdin: stdin, urlColumn: urlColumn}
}

// Handle streams the input at path into the use case.
func (c *Controller) Handle(ctx context.Context, input string) (domain.Summary, error) {
	return c.useCase.Execute(ctx, Jobs(input, c.stdin, c.urlColumn))
}

// HandleResolved downloads one URL while reporting the user's song query.
func (c *Controller) HandleResolved(ctx context.Context, url, label string) (domain.Summary, error) {
	return c.useCase.Execute(ctx, func(yield func(domain.Job, error) bool) {
		job, err := record(1, url)
		job.Label = label
		yield(job, err)
	})
}

// Exit codes.
const (
	ExitOK          = 0
	ExitFailed      = 1
	ExitUsage       = 2
	ExitInterrupted = 130
)

// ExitCode maps a Summary to the process exit code.
func ExitCode(s domain.Summary) int {
	switch {
	case s.Canceled > 0:
		return ExitInterrupted
	case s.Failed > 0:
		return ExitFailed
	}
	return ExitOK
}
