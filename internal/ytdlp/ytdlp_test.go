package ytdlp

import (
	"path/filepath"
	"testing"
)

func TestIsKnownSite(t *testing.T) {
	yes := []string{
		"https://www.youtube.com/watch?v=abc",
		"https://youtu.be/abc",
		"https://m.youtube.com/watch?v=abc",
		"https://www.pornhub.com/view_video.php?viewkey=xyz",
		"https://twitter.com/user/status/123",
		"https://x.com/user/status/123",
		"https://www.instagram.com/p/abc/",
		"https://www.bilibili.com/video/BV1xx",
		"https://www.tiktok.com/@user/video/123",
		"youtube.com/watch?v=abc",
	}
	no := []string{
		"https://example.com/file.zip",
		"https://cdn.example.org/video.mkv",
		"magnet:?xt=urn:btih:abc",
		"ftp://host/file.iso",
		"not a url",
	}
	for _, u := range yes {
		if !IsKnownSite(u) {
			t.Errorf("IsKnownSite(%q) = false, want true", u)
		}
	}
	for _, u := range no {
		if IsKnownSite(u) {
			t.Errorf("IsKnownSite(%q) = true, want false", u)
		}
	}
}

func TestDestination(t *testing.T) {
	d := &Downloader{Cfg: Config{
		BaseYouTube:  "/media/YouTube",
		BaseServices: "/media/Services",
	}}

	// YouTube single video
	dir := d.Destination(&probeInfo{
		ExtractorKey: "Youtube", Channel: "Linus Tech Tips", Title: "My Video",
	})
	if dir != filepath.Join("/media/YouTube", "Linus Tech Tips") {
		t.Errorf("youtube destination = %q", dir)
	}

	// YouTube playlist video
	dir = d.Destination(&probeInfo{
		ExtractorKey: "Youtube", Channel: "Linus Tech Tips",
		PlaylistTitle: "Linux Tips", Title: "Episode 1",
	})
	if dir != filepath.Join("/media/YouTube", "Linus Tech Tips", "Linux Tips") {
		t.Errorf("youtube playlist destination = %q", dir)
	}

	// channel listing: playlist_title == channel -> flat
	dir = d.Destination(&probeInfo{
		ExtractorKey: "Youtube:Tab", Channel: "Linus Tech Tips",
		PlaylistTitle: "Linus Tech Tips", Title: "Video",
	})
	if dir != filepath.Join("/media/YouTube", "Linus Tech Tips") {
		t.Errorf("channel listing destination = %q", dir)
	}

	// other service
	dir = d.Destination(&probeInfo{
		ExtractorKey: "Pornhub", Uploader: "SomeUser", Title: "Video",
	})
	if dir != filepath.Join("/media/Services", "Pornhub", "SomeUser") {
		t.Errorf("service destination = %q", dir)
	}

	// special characters sanitized
	dir = d.Destination(&probeInfo{
		ExtractorKey: "Youtube", Channel: `Bad: Name? *|<>`, Title: "Video",
	})
	want := filepath.Join("/media/YouTube", "Bad - Name")
	if dir != want {
		t.Errorf("sanitize destination = %q, want %q", dir, want)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "", "x") != "x" {
		t.Error("firstNonEmpty failed")
	}
	if firstNonEmpty() != "" {
		t.Error("firstNonEmpty empty failed")
	}
}
