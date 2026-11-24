package domain

import "time"

// FetchKind says what a successful download produced.
type FetchKind uint8

const (
	// Saved: a new file was written.
	Saved FetchKind = iota + 1
	// AlreadyPresent: downloaded in a previous run (archive hit).
	AlreadyPresent
)

// Fetched is the result of a successful download.
type Fetched struct {
	Kind FetchKind
	Path string // set when Kind == Saved
}

// Status is the final state of a job.
type Status uint8

const (
	StatusSaved Status = iota + 1
	StatusSkipped
	StatusFailed
	StatusCanceled
)

// Outcome is what happened to one job.
type Outcome struct {
	Job      Job
	Status   Status
	Path     string // StatusSaved only
	Err      error  // StatusFailed only
	Attempts int
	Elapsed  time.Duration
}
