//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeYtDlp writes a yt-dlp stand-in: it prints a version, expands one
// playlist into two videos, and "saves" any other URL.
func fakeYtDlp(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
[ "$1" = --version ] && { echo 2026.11.01; exit 0; }
for last; do :; done
case "$last" in
  *slow*) exec sleep 30 ;;
  *playlist?list=*) printf 'https://youtu.be/aaaaaaaaaa1\nhttps://youtu.be/aaaaaaaaaa2\n' ;;
  *) echo "/out/${last##*/}.opus" ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// writeFile creates dir/name with content and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func yamlConfig(dir, bin, extra string) string {
	return "output_dir: " + filepath.Join(dir, "out") + "\naudio:\n  format: original\nytdlp:\n  path: " + bin + "\n" + extra
}

func TestRunDownloadsEverything(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "mimo-ytdownloader.yml", yamlConfig(dir, fakeYtDlp(t, dir), ""))
	urls := writeFile(t, dir, "urls.txt",
		"https://youtu.be/bbbbbbbbbb1\nhttps://www.youtube.com/playlist?list=PL1\nhttps://youtu.be/aaaaaaaaaa1\n")
	profile := filepath.Join(dir, "cpu.pprof")

	var out strings.Builder
	code := run([]string{"-c", cfg, "-v", "-cpuprofile", profile, urls}, nil, &out)

	if code != 0 || !strings.Contains(out.String(), "3 saved") || !strings.Contains(out.String(), "1 duplicates") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if fi, err := os.Stat(profile); err != nil || fi.Size() == 0 {
		t.Errorf("CPU profile not written: %v", err)
	}
}

func TestRunDownloadsDirectURL(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "mimo-ytdownloader.yml", yamlConfig(dir, fakeYtDlp(t, dir), ""))
	url := "https://youtu.be/bbbbbbbbbb1"
	var out strings.Builder
	code := run([]string{"-c", cfg, url}, nil, &out)
	if code != 0 || !strings.Contains(out.String(), "1 saved") || !strings.Contains(out.String(), url) {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
}

func TestRunVideoUsesSeparateArchive(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, ".mimo-ytdownloader-archive.txt"), []byte("youtube dQw4w9WgXcQ\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := writeFile(t, dir, "yt-dlp", "#!/bin/sh\n[ \"$1\" = --version ] && { echo fake-version; exit 0; }\necho /out/video.mp4\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	ffmpeg := writeFile(t, dir, "ffmpeg", "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(ffmpeg, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeFile(t, dir, "mimo-ytdownloader.yml", yamlConfig(dir, bin, "  ffmpeg_location: "+ffmpeg+"\n"))
	url := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	var videoOut strings.Builder
	if code := run([]string{"-c", cfg, "--video", url}, nil, &videoOut); code != 0 || !strings.Contains(videoOut.String(), "as video") || !strings.Contains(videoOut.String(), "1 saved") {
		t.Fatalf("video exit %d:\n%s", code, videoOut.String())
	}
	var audioOut strings.Builder
	if code := run([]string{"-c", cfg, "--mp3", url}, nil, &audioOut); code != 0 || !strings.Contains(audioOut.String(), "as mp3") || !strings.Contains(audioOut.String(), "1 skipped") {
		t.Fatalf("audio exit %d:\n%s", code, audioOut.String())
	}
}

func TestRunReadsStdinWithoutPlaylists(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "mimo-ytdownloader.yml", yamlConfig(dir, fakeYtDlp(t, dir), "expand_playlists: false\n"))

	var out strings.Builder
	code := run([]string{"-c", cfg, "-"}, strings.NewReader("https://youtu.be/bbbbbbbbbb1\nnot-a-url\n"), &out)

	if code != 1 || !strings.Contains(out.String(), "1 saved") || !strings.Contains(out.String(), "1 failed") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
}

func TestRunSetupErrors(t *testing.T) {
	dir := t.TempDir()
	bin := fakeYtDlp(t, dir)
	blocker := writeFile(t, dir, "file", "") // a file where a folder is needed

	noOutDir := writeFile(t, dir, "no-out.yml", "output_dir: "+blocker+"/out\nytdlp:\n  path: "+bin+"\n")
	badArchive := writeFile(t, dir, "bad-archive.yml", yamlConfig(dir, bin, "  archive_file: "+dir+"\n"))
	good := writeFile(t, dir, "good.yml", yamlConfig(dir, bin, ""))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"-nope", "urls.txt"}, "flag provided but not defined"},
		{"missing config", []string{"-c", filepath.Join(dir, "none.yml"), "urls.txt"}, "read config"},
		{"output folder can't be created", []string{"-c", noOutDir, "urls.txt"}, "not a directory"},
		{"unreadable archive", []string{"-c", badArchive, "urls.txt"}, "read download archive"},
		{"bad profile path", []string{"-c", good, "-cpuprofile", blocker + "/cpu.pprof", "urls.txt"}, "not a directory"},
		{"missing input file", []string{"-c", good, filepath.Join(dir, "none.txt")}, "no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			code := run(tt.args, nil, &out)
			if code != 2 || !strings.Contains(out.String(), tt.want) {
				t.Errorf("exit %d, output:\n%s\nwant exit 2 containing %q", code, out.String(), tt.want)
			}
		})
	}
}

func TestOnlyOneCPUProfileAtATime(t *testing.T) {
	dir := t.TempDir()
	stop, err := startCPUProfile(filepath.Join(dir, "a.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if _, err := startCPUProfile(filepath.Join(dir, "b.pprof")); err == nil {
		t.Error("a second CPU profile must fail while one is running")
	}
}
