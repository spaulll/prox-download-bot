package organize

import (
	"strings"
	"testing"
	"time"
)

// fakePost returns a canned AniList response with the given http code.
func fakePost(code int, body string) func(string, string, time.Duration) (int, string) {
	return func(string, string, time.Duration) (int, string) {
		return code, body
	}
}

func anilistBody(id int, romaji, english string) string {
	return `{"data":{"Media":{"id":` + itoa(id) +
		`,"title":{"romaji":"` + romaji + `","english":"` + english + `"}}}}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestAniListConfirmedAnime(t *testing.T) {
	r := IsAnimeAnilist("attack on titan", fakePost(200, anilistBody(16498, "Shingeki no Kyojin", "Attack on Titan")))
	if !r.IsAnime || r.Title != "Attack on Titan" {
		t.Errorf("expected anime confirmed with title, got %+v", r)
	}
}

func TestAniListRomajiFallback(t *testing.T) {
	r := IsAnimeAnilist("shingeki no kyojin", fakePost(200, anilistBody(16498, "Shingeki no Kyojin", "")))
	if !r.IsAnime || r.Title != "Shingeki no Kyojin" {
		t.Errorf("expected romaji fallback, got %+v", r)
	}
}

func TestAniListWrongMatchRejected(t *testing.T) {
	// AniList returns its closest guess even for wrong queries: must reject
	// when the query is not a substring of the returned title.
	r := IsAnimeAnilist("the boys", fakePost(200, anilistBody(99999, "BOYS BE...", "BOYS BE...")))
	if r.IsAnime {
		t.Errorf("expected rejection of wrong match, got %+v", r)
	}
}

func TestAniListNoMatch(t *testing.T) {
	body := `{"errors":[{"message":"Not Found."}]}`
	r := IsAnimeAnilist("totally unknown show", fakePost(200, body))
	if r.IsAnime || r.Unreachable {
		t.Errorf("expected not-anime, got %+v", r)
	}
}

func TestAniListUnreachable(t *testing.T) {
	r := IsAnimeAnilist("anything", fakePost(0, ""))
	if !r.Unreachable {
		t.Errorf("expected unreachable, got %+v", r)
	}
}

func TestAniListCaseInsensitiveMatch(t *testing.T) {
	r := IsAnimeAnilist("DEATH NOTE", fakePost(200, anilistBody(1698, "Desu Noto", "Death Note")))
	if !r.IsAnime {
		t.Errorf("expected case-insensitive confirmation, got %+v", r)
	}
	if !strings.Contains(strings.ToLower(r.Title), "death note") {
		t.Errorf("unexpected title %q", r.Title)
	}
}
