//go:build unix

package ytdlp

import (
	"context"
	"testing"
)

func TestReadMP3Name(t *testing.T) {
	probe := script(t, "ffprobe", `echo '{"format":{"tags":{"ARTIST":"  Alejandro Lerner ","title":"Mira a tu alrededor"}}}'`)
	if got := readMP3Name(context.Background(), probe, "/tmp/song.MP3"); got != "Alejandro Lerner - Mira a tu alrededor" {
		t.Errorf("name = %q", got)
	}
	if got := readMP3Name(context.Background(), probe, "/tmp/video.mp4"); got != "" {
		t.Errorf("video name = %q", got)
	}
	if got := readMP3Name(context.Background(), "/missing/ffprobe", "/tmp/song.mp3"); got != "" {
		t.Errorf("missing probe name = %q", got)
	}
}

func TestReadMP3NameFallsBackWhenTagsAreAbsent(t *testing.T) {
	for _, body := range []string{`echo '{"format":{"tags":{"artist":"Artist"}}}'`, `echo 'not json'`} {
		probe := script(t, "ffprobe", body)
		if got := readMP3Name(context.Background(), probe, "/tmp/song.mp3"); got != "" {
			t.Errorf("name = %q", got)
		}
	}
}
