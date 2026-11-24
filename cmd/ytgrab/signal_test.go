//go:build unix

package main

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunStopsOnCtrlC(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "ytgrab.yml", yamlConfig(dir, fakeYtDlp(t, dir), ""))
	urls := writeFile(t, dir, "urls.txt", "https://example.com/slow\n")

	// Ctrl-C arrives while the download is running. run has already turned
	// SIGINT into a context cancel, so the test process is not killed.
	time.AfterFunc(300*time.Millisecond, func() { syscall.Kill(syscall.Getpid(), syscall.SIGINT) })

	var out strings.Builder
	start := time.Now()
	code := run([]string{"-c", cfg, urls}, nil, &out)

	if code != 130 || !strings.Contains(out.String(), "interrupted") || !strings.Contains(out.String(), "1 canceled") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("took %v to stop, want well under the 30s download", elapsed)
	}
}
