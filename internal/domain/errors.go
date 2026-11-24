package domain

import "errors"

// Error classification. Adapters wrap their errors with one of these so the
// use case can decide what to do with errors.Is, without knowing the adapter:
//
//	return fmt.Errorf("%w: %s", domain.ErrTransient, msg)
//
// Cancellation and per-attempt timeouts use context.Canceled and
// context.DeadlineExceeded from the standard library.
var (
	// ErrTransient: worth retrying (throttling, 5xx, network hiccups).
	ErrTransient = errors.New("transient")
	// ErrPermanent: retrying will not help (private video, bad URL...).
	ErrPermanent = errors.New("permanent")
	// ErrInvalidInput: one input record is unusable (not a URL, malformed
	// CSV row). The batch reports it and carries on with the next record.
	ErrInvalidInput = errors.New("invalid input")
)
