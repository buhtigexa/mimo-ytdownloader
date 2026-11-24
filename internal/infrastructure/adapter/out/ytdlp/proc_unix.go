//go:build unix

package ytdlp

import (
	"os"
	"os/exec"
	"syscall"
)

// killGroupOnCancel runs the child in its own process group and, when ctx is
// done, sends SIGTERM to the whole group: yt-dlp and any ffmpeg it spawned.
// If they are still alive after WaitDelay, exec kills the child with SIGKILL.
//
// Being in its own group also means a terminal Ctrl-C reaches only ytgrab;
// ytgrab then cancels ctx and stops every download cleanly.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
