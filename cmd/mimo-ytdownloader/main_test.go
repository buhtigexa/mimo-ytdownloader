package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsageAndConfigErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yml")
	os.WriteFile(bad, []byte("output_dir: "+dir+"\naudio:\n  format: ogg\n"), 0o600)
	noBin := filepath.Join(dir, "nobin.yml")
	os.WriteFile(noBin, []byte("output_dir: "+dir+"\nytdlp:\n  path: /nonexistent/yt-dlp\n"), 0o600)

	tests := []struct {
		args     []string
		wantCode int
		wantOut  string
	}{
		{[]string{}, 2, "missing URL or input file"},
		{[]string{"-h"}, 0, "usage: mimo-ytdownloader"},
		{[]string{"-c", bad, "urls.txt"}, 2, "audio.format: unknown audio format"},
		{[]string{"-c", noBin, "urls.txt"}, 2, "yt-dlp not usable"},
	}
	for _, tt := range tests {
		var out strings.Builder
		code := run(tt.args, io.MultiReader(), &out)
		if code != tt.wantCode || !strings.Contains(out.String(), tt.wantOut) {
			t.Errorf("run(%v) = %d, %q; want %d containing %q", tt.args, code, out.String(), tt.wantCode, tt.wantOut)
		}
		if n := strings.Count(out.String(), "usage: mimo-ytdownloader"); n > 1 {
			t.Errorf("run(%v) printed usage %d times", tt.args, n)
		}
	}
}

func TestRunSearchesSongCSVAndDownloadsInOneInvocation(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
case "$1" in
  --version) echo fake-version; exit 0 ;;
esac
for arg in "$@"; do
  case "$arg" in
    ytsearch10:*)
      echo '{"entries":[{"id":"aaaaaaaaaaa","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"},{"id":"bbbbbbbbbbb","title":"Blondie - Call Me (Official Audio)","channel":"Blondie"},{"id":"ccccccccccc","title":"Blondie - Call Me (Live Official Audio)","channel":"Blondie"}]}'
      exit 0 ;;
  esac
done
is_info=0
for arg in "$@"; do
  [ "$arg" = '--dump-single-json' ] && is_info=1
done
if [ "$is_info" = 1 ]; then
  case "$*" in
    *aaaaaaaaaaa*) echo '{"id":"aaaaaaaaaaa","title":"Blondie - Call Me (Official Audio)","channel":"Blondie","formats":[{"acodec":"opus","vcodec":"none","abr":128}]}' ;;
    *bbbbbbbbbbb*) echo '{"id":"bbbbbbbbbbb","title":"Blondie - Call Me (Official Audio)","channel":"Blondie","formats":[{"acodec":"opus","vcodec":"none","abr":256}]}' ;;
    *) exit 1 ;;
  esac
  exit 0
fi
echo '/tmp/Call Me.mp3'
`
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "mimo-ytdownloader.yml")
	if err := os.WriteFile(cfg, []byte("output_dir: "+dir+"\naudio:\n  format: original\nytdlp:\n  path: "+bin+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	csvPath := filepath.Join(dir, "songs.csv")
	if err := os.WriteFile(csvPath, []byte("artist,title,youtube_search\nBlondie,Call Me,https://www.youtube.com/results?search_query=Blondie+Call+Me\nUnknown,Song,https://www.youtube.com/results?search_query=Unknown+Song\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if code := run([]string{"-c", cfg, csvPath}, strings.NewReader(""), &out); code != 1 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "https://www.youtube.com/watch?v=bbbbbbbbbbb") ||
		!strings.Contains(out.String(), "search [1/2] ok") ||
		!strings.Contains(out.String(), "search [2/2] FAIL") ||
		!strings.Contains(out.String(), "start  line 2: Blondie / Call Me") ||
		!strings.Contains(out.String(), "1 saved, 0 skipped, 1 failed") {
		t.Fatalf("CSV=%s\noutput=%s", data, out.String())
	}
}
