package ytdlp

import "testing"

func TestStudioURLRejectsLiveAndAcceptsStudio(t *testing.T) {
	info := songInfo{ID: "abcdefghijk", Title: "Madonna - Live to Tell (Official Audio)", Channel: "Madonna"}
	if got := studioURL(info, "Madonna", "Live to Tell"); got == "" {
		t.Fatal("studio song named Live to Tell was rejected")
	}
	info.Title += " (En Vivo)"
	if got := studioURL(info, "Madonna", "Live to Tell"); got != "" {
		t.Fatalf("live recording accepted: %s", got)
	}
	info.Title = "Madonna - Live to Tell (Official Audio)"
	info.LiveStatus = "was_live"
	if got := studioURL(info, "Madonna", "Live to Tell"); got != "" {
		t.Fatalf("live status accepted: %s", got)
	}
}

func TestBestAudioBitratePrefersAudioOnly(t *testing.T) {
	var info songInfo
	info.Formats = []songFormat{
		{ACodec: "opus", VCodec: "none", ABR: 128},
		{ACodec: "aac", VCodec: "avc1", ABR: 320},
	}
	if got := bestAudioBitrate(info); got != 128 {
		t.Fatalf("got %.0f, want 128: mimo-ytdownloader chooses bestaudio first", got)
	}
}
