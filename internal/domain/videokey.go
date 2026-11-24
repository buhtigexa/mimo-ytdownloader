package domain

import "strings"

// YouTubeID returns the 11-character id of a YouTube video URL. It accepts
//
//	youtube.com/watch?v=ID, youtu.be/ID, /shorts/ID, /embed/ID, /live/ID, /v/ID
//
// on youtube.com (with or without www., m. or music.) and youtube-nocookie.com.
// It does not allocate.
func YouTubeID(url string) (string, bool) {
	host, path, query, ok := parseURL(url)
	if !ok {
		return "", false
	}
	var id string
	switch {
	case strings.EqualFold(host, "youtu.be"):
		id = firstSegment(path)
	case isYouTubeHost(host):
		id = youtubePathID(path, query)
	default:
		return "", false
	}
	if !isVideoID(id) {
		return "", false
	}
	return id, true
}

// parseURL splits an http(s) URL into its host (port and www./m./music.
// prefix removed), path (without leading slash) and query (fragment removed).
func parseURL(url string) (host, path, query string, ok bool) {
	rest, ok := strings.CutPrefix(url, "https://")
	if !ok {
		if rest, ok = strings.CutPrefix(url, "http://"); !ok {
			return "", "", "", false
		}
	}
	authority, tail, ok := strings.Cut(rest, "/")
	if !ok {
		return "", "", "", false
	}
	host, _, _ = strings.Cut(authority, ":")
	tail, _, _ = strings.Cut(tail, "#")
	path, query, _ = strings.Cut(tail, "?")
	return trimHostPrefix(host), path, query, true
}

func isYouTubeHost(host string) bool {
	return strings.EqualFold(host, "youtube.com") || strings.EqualFold(host, "youtube-nocookie.com")
}

// trimHostPrefix drops one www./m./music. prefix, case-insensitively.
func trimHostPrefix(host string) string {
	for _, p := range [...]string{"www.", "m.", "music."} {
		if len(host) > len(p) && strings.EqualFold(host[:len(p)], p) {
			return host[len(p):]
		}
	}
	return host
}

func youtubePathID(path, query string) string {
	if path == "watch" || path == "watch/" {
		return queryParam(query, "v")
	}
	for _, p := range [...]string{"shorts/", "embed/", "live/", "v/"} {
		if rest, ok := strings.CutPrefix(path, p); ok {
			return firstSegment(rest)
		}
	}
	return ""
}

func firstSegment(path string) string {
	seg, _, _ := strings.Cut(path, "/")
	return seg
}

func queryParam(query, name string) string {
	for query != "" {
		var pair string
		pair, query, _ = strings.Cut(query, "&")
		if k, v, ok := strings.Cut(pair, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// isVideoID: YouTube ids are exactly 11 chars of [A-Za-z0-9_-].
func isVideoID(s string) bool {
	if len(s) != 11 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
