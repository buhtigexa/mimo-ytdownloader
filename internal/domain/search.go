package domain

import "strings"

// IsSearchPage reports whether url is a search-results page (YouTube or
// YouTube Music), e.g. youtube.com/results?search_query=... or
// music.youtube.com/search?q=...
//
// A search-results page is not a video URL. yt-dlp would treat it as a list
// of videos, so it is rejected as invalid download input.
func IsSearchPage(url string) bool {
	host, path, _, ok := parseURL(url)
	if !ok || !isYouTubeHost(host) {
		return false
	}
	path = strings.TrimSuffix(path, "/")
	return path == "results" || path == "search"
}
