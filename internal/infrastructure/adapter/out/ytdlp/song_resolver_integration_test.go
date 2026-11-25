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

func TestSongResolverFreeTextQuery(t *testing.T) {
	bin := songBin(t, `case "$*" in
*ytsearch10:*) echo '{"entries":[{"id":"aaaaaaaaaaa","title":"Alejandro Lerner - Mira a tu alrededor (Live Official Audio)","channel":"Alejandro Lerner"},{"id":"bbbbbbbbbbb","title":"Mira a tu alrededor","channel":"Alejandro Lerner - Topic"},{"id":"ccccccccccc","title":"Alejandro Lerner - Mira a tu alrededor (Official Audio)","channel":"Alejandro Lerner"}]}' ;;
*bbbbbbbbbbb*) echo '{"id":"bbbbbbbbbbb","title":"Mira a tu alrededor","channel":"Alejandro Lerner - Topic","formats":[{"acodec":"opus","vcodec":"none","abr":160}]}' ;;
*ccccccccccc*) echo '{"id":"ccccccccccc","title":"Alejandro Lerner - Mira a tu alrededor (Official Audio)","channel":"Alejandro Lerner","formats":[{"acodec":"opus","vcodec":"none","abr":128}]}' ;;
*) exit 1 ;;
esac`)
	url, err := NewSongResolver(bin).ResolveQuery(context.Background(), "mira a tu alrededor alejandro lerner")
	if err != nil || url != "https://www.youtube.com/watch?v=bbbbbbbbbbb" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestSongResolverFreeTextNoMatch(t *testing.T) {
	bin := songBin(t, `echo '{"entries":[{"id":"aaaaaaaaaaa","title":"Another Song (Official Audio)","channel":"Alejandro Lerner"}]}'`)
	url, err := NewSongResolver(bin).ResolveQuery(context.Background(), "mira a tu alrededor alejandro lerner")
	if err != nil || url != "" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestFreeTextCandidateChecks(t *testing.T) {
	query := "mira a tu alrededor alejandro lerner"
	base := songInfo{ID: "aaaaaaaaaaa", Title: "Alejandro Lerner - Mira a tu alrededor (Official Audio)", Channel: "Alejandro Lerner"}
	for _, tt := range []struct {
		name string
		edit func(*songInfo)
		want bool
	}{
		{"official", func(*songInfo) {}, true},
		{"live", func(i *songInfo) { i.IsLive = true }, false},
		{"bad ID", func(i *songInfo) { i.ID = "bad" }, false},
		{"wrong song", func(i *songInfo) { i.Title = "Other song (Official Audio)" }, false},
		{"cover", func(i *songInfo) { i.Title += " Cover" }, false},
		{"unverified", func(i *songInfo) { i.Title = "Alejandro Lerner - Mira a tu alrededor" }, false},
		{"topic channel", func(i *songInfo) { i.Title = "Mira a tu alrededor"; i.Channel = "Alejandro Lerner - Topic" }, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := base
			tt.edit(&info)
			if got := queryStudioURL(info, query) != ""; got != tt.want {
				t.Errorf("matched = %t, want %t", got, tt.want)
			}
		})
	}
}
