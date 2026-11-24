//go:build !unix

package ytdlp

import (
	"os"
	"os/exec"
)

// killGroupOnCancel: outside unix, exec's default (Process.Kill) is used.
func killGroupOnCancel(*exec.Cmd) {}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
