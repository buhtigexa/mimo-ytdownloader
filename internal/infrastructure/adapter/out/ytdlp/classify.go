package ytdlp

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// transient lists substrings of yt-dlp errors worth retrying.
var transient = [...]string{
	"HTTP Error 429",
	"HTTP Error 5",
	"timed out",
	"Connection reset",
	"Connection refused",
	"Temporary failure",
	"Remote end closed",
	"IncompleteRead",
	"Unable to download webpage",
	"giving up after",
}

// classify turns yt-dlp's stderr into one short error wrapping
// domain.ErrTransient or domain.ErrPermanent.
func classify(stderr []byte, exitCode int) error {
	msg := lastErrorLine(stderr)
	if msg == "" {
		return fmt.Errorf("%w: yt-dlp exited with code %d", domain.ErrPermanent, exitCode)
	}
	for _, p := range transient {
		if strings.Contains(msg, p) {
			return fmt.Errorf("%w: %s", domain.ErrTransient, msg)
		}
	}
	return fmt.Errorf("%w: %s", domain.ErrPermanent, msg)
}

// lastErrorLine returns the last "ERROR:" line (without the prefix), or the
// last non-empty line if there is none.
func lastErrorLine(b []byte) string {
	fallback := ""
	for len(b) > 0 {
		i := bytes.LastIndexByte(b, '\n')
		line := bytes.TrimSpace(b[i+1:])
		b = b[:max(i, 0)]
		if len(line) == 0 {
			continue
		}
		if rest, ok := bytes.CutPrefix(line, []byte("ERROR:")); ok {
			return string(bytes.TrimSpace(rest))
		}
		if fallback == "" {
			fallback = string(line)
		}
		if i < 0 {
			break
		}
	}
	return fallback
}
