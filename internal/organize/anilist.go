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
	query := fmt.Sprintf(`{"query":"{ Media(search:\\\"%s\\\", type:ANIME) { id title { romaji english } } }"}`, safeName)

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

	// exact substring verification
	searchLower := strings.ToLower(queryName)
	if strings.Contains(strings.ToLower(englishTitle), searchLower) ||
		strings.Contains(strings.ToLower(romajiTitle), searchLower) {
		return AniListResult{IsAnime: true, Title: canonical}
	}
	// AniList returned a closest guess that is not actually the show
	return AniListResult{IsAnime: false}
}
