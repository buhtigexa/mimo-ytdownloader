package domain

// Job is one URL of the batch and the input line it came from.
type Job struct {
	Line     int    // 1-based line number in the source, for error messages
	URL      string // video URL (or playlist URL before expansion)
	Playlist string // playlist this video was expanded from; "" if listed directly
	Label    string // human-readable artist / title for CSV tracks
}
