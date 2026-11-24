package ytdlp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func songBin(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSongResolverChoosesBestOfThree(t *testing.T) {
	bin := songBin(t, `case "$*" in
*ytsearch10:*) echo '{"entries":[{"id":"aaaaaaaaaaa","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"},{"id":"bbbbbbbbbbb","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"},{"id":"ccccccccccc","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"},{"id":"ddddddddddd","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"}]}' ;;
*aaaaaaaaaaa*) echo '{"id":"aaaaaaaaaaa","title":"Blondie - Call Me (Official Audio)","channel":"Blondie","formats":[{"acodec":"opus","vcodec":"none","abr":128}]}' ;;
*bbbbbbbbbbb*) echo '{"id":"bbbbbbbbbbb","title":"Blondie - Call Me (Official Audio)","channel":"Blondie","formats":[{"acodec":"opus","vcodec":"none","abr":256}]}' ;;
*ccccccccccc*) echo '{"id":"ccccccccccc","title":"Blondie - Call Me (Live Official Audio)","channel":"Blondie","formats":[{"acodec":"opus","vcodec":"none","abr":320}]}' ;;
*) exit 1 ;;
esac`)
	url, err := NewSongResolver(bin).Resolve(context.Background(), "Blondie", "Call Me")
	if err != nil || url != "https://www.youtube.com/watch?v=bbbbbbbbbbb" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestSongResolverNoMatchAndSearchErrors(t *testing.T) {
	for _, tt := range []struct {
		script  string
		wantErr bool
	}{
		{`echo '{"entries":[]}'`, false},
		{`echo 'invalid json'`, true},
		{`echo problem >&2; exit 1`, true},
	} {
		url, err := NewSongResolver(songBin(t, tt.script)).Resolve(context.Background(), "Blondie", "Call Me")
		if url != "" || (err != nil) != tt.wantErr {
			t.Fatalf("url=%q err=%v wantErr=%v", url, err, tt.wantErr)
		}
	}
}

func TestSongResolverReportsInspectionFailure(t *testing.T) {
	bin := songBin(t, `case "$*" in
*ytsearch10:*) echo '{"entries":[{"id":"aaaaaaaaaaa","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"}]}' ;;
*) echo unavailable >&2; exit 1 ;;
esac`)
	_, err := NewSongResolver(bin).Resolve(context.Background(), "Blondie", "Call Me")
	if err == nil || !strings.Contains(err.Error(), "could not inspect") {
		t.Fatalf("err=%v", err)
	}
}

func TestSongResolverCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewSongResolver(songBin(t, `exit 0`)).Resolve(ctx, "Blondie", "Call Me")
	if err == nil {
		t.Fatal("expected cancellation")
	}
}
