package telegram

import "testing"

func TestShortDisplayName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://example.com/files/Reacher.S04.1080P.zip", "Reacher.S04.1080P.zip"},
		{"https://example.com/files/Reacher.S04.1080P.zip?token=abc#frag", "Reacher.S04.1080P.zip"},
		{"https://worker.dev/abc123::def456/Reacher.S04.1080P.zip", "Reacher.S04.1080P.zip"},
		{"magnet:?xt=urn:btih:ABCDEF123456&dn=Some.Show.S01", "Some.Show.S01"},
		{"magnet:?xt=urn:btih:ABCDEF123456", "magnet"},
		{"show.torrent", "show.torrent"},
	}
	for _, c := range cases {
		if got := shortDisplayName(c.in); got != c.want {
			t.Errorf("shortDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
