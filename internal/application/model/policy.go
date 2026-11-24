// Package model holds application-level policies and commands.
package model

import "time"

// DownloadPolicy tells DownloadBatch how to run.
type DownloadPolicy struct {
	Workers  int           // parallel downloads, >= 1
	Attempts int           // total tries per job, first one included, >= 1
	Backoff  time.Duration // linear: retry N waits N * Backoff
	Timeout  time.Duration // deadline for a single attempt
}
