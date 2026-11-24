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
		{[]string{}, 2, "missing input file"},
		{[]string{"-h"}, 0, "usage: ytgrab"},
		{[]string{"-c", bad, "urls.txt"}, 2, "audio.format: unknown audio format"},
		{[]string{"-c", noBin, "urls.txt"}, 2, "yt-dlp not usable"},
	}
	for _, tt := range tests {
		var out strings.Builder
		code := run(tt.args, io.MultiReader(), &out)
		if code != tt.wantCode || !strings.Contains(out.String(), tt.wantOut) {
			t.Errorf("run(%v) = %d, %q; want %d containing %q", tt.args, code, out.String(), tt.wantCode, tt.wantOut)
		}
		if n := strings.Count(out.String(), "usage: ytgrab"); n > 1 {
			t.Errorf("run(%v) printed usage %d times", tt.args, n)
		}
	}
}
