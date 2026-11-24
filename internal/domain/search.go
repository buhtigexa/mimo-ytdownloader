package domain

import "strings"

// IsSearchPage reports whether url is a search-results page (YouTube or
// YouTube Music), e.g. youtube.com/results?search_query=... or
// music.youtube.com/search?q=...
//
// Rule: ytgrab downloads the URLs it is given; it never turns a search into
// downloads. yt-dlp would treat a results page as a list of videos, so these
// URLs are rejected as invalid input.
func IsSearchPage(url string) bool {
	host, path, _, ok := parseURL(url)
	if !ok || !isYouTubeHost(host) {
		return false
	}
	path = strings.TrimSuffix(path, "/")
	return path == "results" || path == "search"
}
