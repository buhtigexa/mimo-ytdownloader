package domain

import "hash/maphash"

// Duplicate is a job dropped because an earlier line points at the same video.
type Duplicate struct {
	Job       Job
	FirstLine int
}

// Deduper enforces, in streaming, the rule "a video is downloaded at most
// once per batch", whatever URL form was used.
//
// It keeps only a 64-bit hash of each video's identity plus the line where
// it first appeared (~12 bytes of payload per unique video), so it can track
// tens of millions of videos in a few hundred MB. The chance of two different
// videos colliding is ~n²/2⁶⁵: below one in a million for 10M videos.
//
// Not safe for concurrent use: the pipeline runs it in a single goroutine.
type Deduper struct {
	seed  maphash.Seed
	first map[uint64]int32
}

// NewDeduper returns an empty Deduper.
func NewDeduper() *Deduper {
	return &Deduper{seed: maphash.MakeSeed(), first: make(map[uint64]int32, 1024)}
}

// Check records job and reports whether an earlier job pointed at the same
// video (and at which line).
func (d *Deduper) Check(job Job) (firstLine int, dup bool) {
	k := d.key(job.URL)
	if line, ok := d.first[k]; ok {
		return int(line), true
	}
	d.first[k] = int32(job.Line)
	return 0, false
}

// key hashes VideoKey(url) without building the string.
func (d *Deduper) key(url string) uint64 {
	var h maphash.Hash
	h.SetSeed(d.seed)
	if id, ok := youtubeID(url); ok {
		h.WriteString("youtube:")
		h.WriteString(id)
	} else {
		h.WriteString("url:")
		h.WriteString(url)
	}
	return h.Sum64()
}

// Unique applies the Deduper to a slice, keeping order. Handy for small
// in-memory batches and tests.
func Unique(jobs []Job) (keep []Job, dups []Duplicate) {
	d := NewDeduper()
	keep = make([]Job, 0, len(jobs))
	for _, j := range jobs {
		if line, dup := d.Check(j); dup {
			dups = append(dups, Duplicate{Job: j, FirstLine: line})
			continue
		}
		keep = append(keep, j)
	}
	return keep, dups
}
