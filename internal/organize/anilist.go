package organize

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AniListAPI endpoint
const anilistURL = "https://graphql.anilist.co"

// AniListResult is the outcome of an AniList lookup.
type AniListResult struct {
	// True when AniList confirms the name is an anime.
	IsAnime bool
	// Canonical romaji/english title when confirmed, else "".
	Title string
	// True when the API was unreachable (caller falls back to series).
	Unreachable bool
}

type anilistResponse struct {
	Data struct {
		Media struct {
			ID    int `json:"id"`
			Title struct {
				Romaji  string `json:"romaji"`
				English string `json:"english"`
			} `json:"title"`
		} `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// IsAnimeAnilist queries the AniList GraphQL API to check whether the name is
// an anime. Mirrors AriaFlow: search result must contain the query as an
// exact substring of the returned title (AniList always returns its closest
// guess, even for wrong matches).
func IsAnimeAnilist(queryName string, doPost func(url string, body string, timeout time.Duration) (int, string)) AniListResult {
	safeName := strings.ReplaceAll(queryName, `"`, `\"`)
	query := fmt.Sprintf(`{"query":"{ Media(search:\"%s\", type:ANIME) { id title { romaji english } } }"}`, safeName)

	httpCode, body := doPost(anilistURL, query, 10*time.Second)
	if httpCode != 200 {
		return AniListResult{Unreachable: true}
	}

	var resp anilistResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return AniListResult{Unreachable: true}
	}
	if len(resp.Errors) > 0 || resp.Data.Media.ID == 0 {
		return AniListResult{IsAnime: false}
	}

	englishTitle := resp.Data.Media.Title.English
	romajiTitle := resp.Data.Media.Title.Romaji

	canonical := ""
	if englishTitle != "" && englishTitle != "null" {
		canonical = englishTitle
	} else if romajiTitle != "" && romajiTitle != "null" {
		canonical = romajiTitle
	}

	// fuzzy containment check: normalize both sides (lowercase, separators
	// to spaces) so release-name artifacts like "Anime.Show." still
	// match the clean title "Anime Show"
	if titlesMatch(queryName, englishTitle) || titlesMatch(queryName, romajiTitle) {
		return AniListResult{IsAnime: true, Title: canonical}
	}
	// AniList returned a closest guess that is not actually the show
	return AniListResult{IsAnime: false}
}

// titlesMatch reports whether a release-name-derived query refers to the
// same show as an AniList title. Normalizes both to lowercase with
// separators as spaces, then accepts contiguous containment in either
// direction (the query may carry extra markers like a trailing dash, or the
// title may carry a subtitle).
func titlesMatch(query, title string) bool {
	q, t := normalizeTitle(query), normalizeTitle(title)
	if q == "" || t == "" {
		return false
	}
	return strings.Contains(t, q) || strings.Contains(q, t)
}

// normalizeTitle lowercases and turns separators into single spaces so
// release names and clean titles become comparable strings.
func normalizeTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '_', '.', '-', ':', '\'', '!', '(', ')', '[', ']', ',':
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
