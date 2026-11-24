package domain

import "strings"

// IsPlaylist reports whether url is a YouTube playlist page that must be
// expanded into one job per video:
//
//	youtube.com/playlist?list=ID, music.youtube.com/playlist?list=ID
//
// A video watched inside a playlist (watch?v=X&list=Y) is a video, not a
// playlist: only that video is downloaded. This also keeps endless "radio"
// mixes (list=RD...) from exploding a batch.
func IsPlaylist(url string) bool {
	host, path, query, ok := parseURL(url)
	if !ok || !isYouTubeHost(host) {
		return false
	}
	path = strings.TrimSuffix(path, "/")
	return path == "playlist" && queryParam(query, "list") != ""
}
