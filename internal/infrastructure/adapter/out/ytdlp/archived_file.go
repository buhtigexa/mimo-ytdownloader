package ytdlp

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
)

// archivedFile resolves an existing file from the video's ID in the default
// filename template. The directory is indexed once, only if a skip occurs.
func (d *Downloader) archivedFile(ctx context.Context, id string) domain.Fetched {
	path := d.archivedPath(id)
	return domain.Fetched{Kind: domain.AlreadyPresent, Path: path, Name: readMP3Name(ctx, d.ffprobe, path)}
}

func (d *Downloader) archivedPath(id string) string {
	d.pathsOnce.Do(func() {
		d.paths = make(map[string]string)
		filepath.WalkDir(d.outputDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !d.matchesMode(path) {
				return nil
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			open := strings.LastIndexByte(name, '[')
			if open < 0 {
				return nil
			}
			close := strings.IndexByte(name[open+1:], ']')
			if close != 11 {
				return nil
			}
			videoID := name[open+1 : open+12]
			if _, exists := d.paths[videoID]; !exists {
				absolute, err := filepath.Abs(path)
				if err == nil {
					d.paths[videoID] = absolute
				}
			}
			return nil
		})
	})
	return d.paths[id]
}

func (d *Downloader) matchesMode(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if d.video {
		return ext == ".mp4" || ext == ".mkv" || ext == ".webm" || ext == ".mov"
	}
	if d.format == MP3 {
		return ext == ".mp3"
	}
	return ext == ".m4a" || ext == ".opus" || ext == ".webm" || ext == ".flac" || ext == ".wav"
}
