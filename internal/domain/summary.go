package domain

import "time"

// Summary counts the outcomes of a batch.
type Summary struct {
	Total      int // unique jobs (duplicates excluded)
	Saved      int
	Skipped    int
	Failed     int
	Canceled   int
	Duplicates int // dropped by the Deduper, not part of Total
	Elapsed    time.Duration
}

// Add counts one outcome.
func (s *Summary) Add(o Outcome) {
	switch o.Status {
	case StatusSaved:
		s.Saved++
	case StatusSkipped:
		s.Skipped++
	case StatusFailed:
		s.Failed++
	case StatusCanceled:
		s.Canceled++
	}
}

// Seen returns how many jobs reached a final state.
func (s Summary) Seen() int {
	return s.Saved + s.Skipped + s.Failed + s.Canceled
}
