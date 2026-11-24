package domain

import (
	"slices"
	"testing"
)

func TestVideoKey(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		{"https://youtube.com/watch?feature=share&v=dQw4w9WgXcQ&t=42", "youtube:dQw4w9WgXcQ"},
		{"http://m.youtube.com/watch?v=dQw4w9WgXcQ#t=1", "youtube:dQw4w9WgXcQ"},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ&list=RD", "youtube:dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?si=abc", "youtube:dQw4w9WgXcQ"},
		{"https://YOUTU.BE/dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		{"https://WWW.YouTube.com/watch?v=dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		{"https://www.youtube.com/embed/dQw4w9WgXcQ?start=3", "youtube:dQw4w9WgXcQ"},
		{"https://www.youtube.com/live/dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		{"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		{"https://www.youtube.com:443/watch?v=dQw4w9WgXcQ", "youtube:dQw4w9WgXcQ"},
		// not a recognizable video id: keyed by the raw URL
		{"https://www.youtube.com/watch?v=short", "url:https://www.youtube.com/watch?v=short"},
		{"https://www.youtube.com/playlist?list=PL123", "url:https://www.youtube.com/playlist?list=PL123"},
		{"https://soundcloud.com/a/b", "url:https://soundcloud.com/a/b"},
		{"https://youtu.be/", "url:https://youtu.be/"},
		{"https://notyoutube.com/watch?v=dQw4w9WgXcQ", "url:https://notyoutube.com/watch?v=dQw4w9WgXcQ"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := VideoKey(tt.url); got != tt.want {
				t.Errorf("VideoKey(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestIsPlaylist(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/playlist?list=PLM6OA8eOe83s", true},
		{"https://music.youtube.com/playlist?list=OLAK5uy_abc", true},
		{"https://youtube.com/playlist/?list=PL1&si=x", true},
		{"https://www.youtube.com/watch?v=yJmZWvwF1OI&list=RDyJmZWvwF1OI", false}, // video inside a mix
		{"https://youtu.be/yJmZWvwF1OI?list=RDyJmZWvwF1OI", false},
		{"https://www.youtube.com/playlist", false}, // no list id
		{"https://www.youtube.com/playlist?list=", false},
		{"https://soundcloud.com/a/sets/b", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		if got := IsPlaylist(tt.url); got != tt.want {
			t.Errorf("IsPlaylist(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsSearchPage(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/results?search_query=mecano+barco+a+venus", true},
		{"https://youtube.com/results/?search_query=x", true},
		{"https://music.youtube.com/search?q=x", true},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"https://www.youtube.com/playlist?list=PL1", false},
		{"https://example.com/results?q=x", false},
	}
	for _, tt := range tests {
		if got := IsSearchPage(tt.url); got != tt.want {
			t.Errorf("IsSearchPage(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestUnique(t *testing.T) {
	in := []Job{
		{Line: 1, URL: "https://youtu.be/dQw4w9WgXcQ"},
		{Line: 2, URL: "https://soundcloud.com/a/b"},
		{Line: 3, URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=5"},
		{Line: 4, URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw"},
		{Line: 5, URL: "https://soundcloud.com/a/b"},
	}
	keep, dups := Unique(in)

	var lines []int
	for _, j := range keep {
		lines = append(lines, j.Line)
	}
	if want := []int{1, 2, 4}; !slices.Equal(lines, want) {
		t.Errorf("kept lines = %v, want %v", lines, want)
	}

	var pairs [][2]int
	for _, d := range dups {
		pairs = append(pairs, [2]int{d.Job.Line, d.FirstLine})
	}
	if want := [][2]int{{3, 1}, {5, 2}}; !slices.Equal(pairs, want) {
		t.Errorf("dups = %v, want %v", pairs, want)
	}
}

func TestDeduperStreaming(t *testing.T) {
	d := NewDeduper()
	steps := []struct {
		job      Job
		wantDup  bool
		wantLine int
	}{
		{Job{Line: 1, URL: "https://youtu.be/dQw4w9WgXcQ"}, false, 0},
		{Job{Line: 2, URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=1"}, true, 1},
		{Job{Line: 3, URL: "https://soundcloud.com/a"}, false, 0},
		{Job{Line: 4, URL: "https://soundcloud.com/a"}, true, 3},
		{Job{Line: 5, URL: "https://soundcloud.com/b"}, false, 0},
	}
	for _, s := range steps {
		line, dup := d.Check(s.job)
		if dup != s.wantDup || line != s.wantLine {
			t.Errorf("Check(line %d) = %d, %v; want %d, %v", s.job.Line, line, dup, s.wantLine, s.wantDup)
		}
	}
}

func TestYouTubeID(t *testing.T) {
	if id, ok := YouTubeID("https://youtu.be/dQw4w9WgXcQ?si=x"); !ok || id != "dQw4w9WgXcQ" {
		t.Errorf("got %q, %v", id, ok)
	}
	if _, ok := YouTubeID("https://soundcloud.com/a"); ok {
		t.Error("soundcloud is not a youtube id")
	}
}

func BenchmarkDeduperCheck(b *testing.B) {
	d := NewDeduper()
	j := Job{Line: 1, URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}
	b.ReportAllocs()
	for b.Loop() {
		d.Check(j)
	}
}

func TestSummary(t *testing.T) {
	var s Summary
	for _, st := range []Status{StatusSaved, StatusSaved, StatusSkipped, StatusFailed, StatusCanceled} {
		s.Add(Outcome{Status: st})
	}
	if s.Saved != 2 || s.Skipped != 1 || s.Failed != 1 || s.Canceled != 1 || s.Seen() != 5 {
		t.Errorf("unexpected summary %+v", s)
	}
}

func BenchmarkVideoKey(b *testing.B) {
	for b.Loop() {
		VideoKey("https://www.youtube.com/watch?feature=share&v=dQw4w9WgXcQ&t=42")
	}
}

func BenchmarkUnique(b *testing.B) {
	jobs := make([]Job, 1000)
	for i := range jobs {
		jobs[i] = Job{Line: i + 1, URL: "https://youtu.be/dQw4w9WgXc" + string(rune('A'+i%26))}
	}
	b.ReportAllocs()
	for b.Loop() {
		Unique(jobs)
	}
}
