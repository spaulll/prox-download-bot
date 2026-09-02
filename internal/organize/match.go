package organize

import (
	"os"
	"path/filepath"
	"strings"
)

// MatchScore is the minimum token-overlap score required to accept a folder
// match (same threshold as AriaFlow: at least 2 matching tokens).
const MatchScore = 2

// FolderMatcher finds the best matching series/anime folder inside a library
// root, mirroring AriaFlow's find_series_folder / find_anime_folder logic.
type FolderMatcher struct {
	Root string
}

// Find returns the full path of the best matching folder under Root, or ""
// when no confident match exists. Exact normalized match wins immediately;
// otherwise the highest token-overlap score wins, ties broken by the longer
// candidate name (more specific).
func (m FolderMatcher) Find(target string) string {
	entries, err := os.ReadDir(m.Root)
	if err != nil {
		return ""
	}
	// normalize the target so raw-case queries still match
	target = NormalizeName(target)
	bestPath := ""
	bestScore := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidatePath := filepath.Join(m.Root, e.Name())
		candidateNorm := NormalizeName(e.Name())

		// exact match: return immediately
		if candidateNorm == target {
			return candidatePath
		}

		score := 0
		for _, token := range strings.Fields(target) {
			if len(token) < 2 {
				continue // skip single-char noise
			}
			for _, ct := range strings.Fields(candidateNorm) {
				if ct == token {
					score++
					break
				}
			}
		}

		if score > bestScore ||
			(score == bestScore && score > 0 && len(candidateNorm) > len(NormalizeName(filepath.Base(bestPath)))) {
			bestScore = score
			bestPath = candidatePath
		}
	}
	if bestScore >= MatchScore {
		return bestPath
	}
	return ""
}
