//go:build !unix

package ytdlp

import "os/exec"

// killGroupOnCancel: outside unix, exec's default (Process.Kill) is used.
func killGroupOnCancel(*exec.Cmd) {}
