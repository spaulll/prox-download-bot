// Package organize implements post-download media organization with
// directory-structure parity to AriaFlow's organize_download.sh.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
// Derived from DownloadBot by gaowanliang (Apache-2.0).
package organize

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	extRe         = regexp.MustCompile(`\.[a-z0-9]{2,4}$`)
	yearParenRe   = regexp.MustCompile(`\(([0-9]{4})\)`)
	yearBareRe    = regexp.MustCompile(`\b[0-9]{4}\b`)
	seasonEpCut   = regexp.MustCompile(`(?i)\bs[0-9]{1,2}[._ -]?e[0-9]{1,3}\b.*`)
	seasonPackCut = regexp.MustCompile(`(?i)\bs[0-9]{1,2}\b.*`)
	seasonOnlyRe  = regexp.MustCompile(`(?i)\bs[0-9]{1,2}\b`)
	xeCut         = regexp.MustCompile(`(?i)\b[0-9]{1,2}x[0-9]{1,3}\b.*`)
	tagCut        = regexp.MustCompile(`(?i)\b(720p|1080p|2160p|480p|360p|4k|uhd|bluray|blu-ray|bdrip|bd|brrip|webrip|web-dl|webdl|web|hdtv|dvdrip|dvdscr|x264|x265|h\.?264|h\.?265|hevc|avc|aac|ac3|eac3|dts|dtshd|truehd|atmos|hdr|hdr10|dolby|vision|10bit|8bit|remux|extended|repack|proper|remastered|unrated|dual|audio|sub|subs|msubs|esubs|subbed|dubbed|hindi|korean|japanese|chinese|taiwanese|mandarin|english|RG|org|netflix|amzn|nf|dsnp|hulu|hmax|max|pc|rip)\b.*`)
	bracketRe     = regexp.MustCompile(`\[[^\]]*\]`)
	parenRe       = regexp.MustCompile(`\([^)]*\)`)
	movieYearRe   = regexp.MustCompile(`\b((?:19|20)[0-9]{2})\b`)
	spaceRe       = regexp.MustCompile(`\s+`)
	episodeRe     = regexp.MustCompile(`(?i)\b(s[0-9]{1,2}[._ -]?e[0-9]{1,3}|[0-9]{1,2}x[0-9]{1,3}|e[0-9]{1,3}(?:\b|\.[a-z0-9]{2,4}$))`)
	seRe          = regexp.MustCompile(`(?i)\bs([0-9]{1,2})[._ -]?e[0-9]{1,3}\b`)
	xRe           = regexp.MustCompile(`(?i)\b([0-9]{1,2})x[0-9]{1,3}\b`)
	eOnlyRe       = regexp.MustCompile(`(?i)(?:\b|\s)e([0-9]{1,3})\b`)
)

// SanitizeFolderName removes characters illegal in Windows/NTFS/Samba folder
// names. "Show: Subtitle" becomes "Show - Subtitle". Also strips * ? " < > |
// and leading/trailing spaces and dots.
func SanitizeFolderName(name string) string {
	name = strings.TrimSpace(name)
	name = regexp.MustCompile(`:\s*`).ReplaceAllString(name, " - ")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '*', '?', '"', '<', '>', '|', '/', '\\', '\x00':
			return -1
		}
		return r
	}, name)
	name = spaceRe.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ".")
	return name
}

// SanitizeFileName removes filesystem-illegal characters from a file name
// while preserving dots (extension separators).
func SanitizeFileName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '*', '?', '"', '<', '>', '|', ':', '/', '\\', '\x00':
			return -1
		}
		return r
	}, name)
	return strings.TrimSpace(name)
}

// NormalizeName lowercases, strips extension/punctuation/year/resolution/tags
// and collapses spaces. Used for matching both file names and folder names.
func NormalizeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = extRe.ReplaceAllString(s, "")
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(s)
	s = yearParenRe.ReplaceAllString(s, "")
	s = yearBareRe.ReplaceAllString(s, "")
	// cut everything from the episode/season tag onward
	if loc := seasonEpCut.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	} else if loc := xeCut.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	} else if loc := seasonPackCut.FindStringIndex(s); loc != nil {
		// season-only tag (season pack): cut it too, but keep text if the
		// tag is the whole name
		if strings.TrimSpace(s[:loc[0]]) != "" {
			s = s[:loc[0]]
		}
	}
	s = tagCut.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// TitleCase converts "Some Show" to "Some Show"
func TitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// IsEpisode reports whether the file name contains a clear episode tag.
// A bare year like "2019" is NOT an episode tag.
func IsEpisode(filename string) bool {
	return episodeRe.MatchString(filename)
}

// ExtractSeason returns the season number (no leading zero) from patterns
// like S04E06, S4E6, 4x06, s04.e06. Returns 0 if no season found.
func ExtractSeason(filename string) int {
	lower := strings.ToLower(filename)
	if m := seRe.FindStringSubmatch(lower); m != nil {
		return parseNum(m[1])
	}
	if m := xRe.FindStringSubmatch(lower); m != nil {
		return parseNum(m[1])
	}
	// s01 standalone (season pack folder or file without episode)
	if m := regexp.MustCompile(`(?i)\bs([0-9]{1,2})\b`).FindStringSubmatch(lower); m != nil {
		return parseNum(m[1])
	}
	return 0
}

// ExtractEpisode returns the episode number from the file name, 0 if none.
func ExtractEpisode(filename string) int {
	lower := strings.ToLower(filename)
	if m := seRe.FindStringSubmatch(lower); m != nil {
		idx := strings.Index(strings.ToLower(m[0]), "e")
		return parseNum(m[0][idx+1:])
	}
	if m := xRe.FindStringSubmatch(lower); m != nil {
		idx := strings.Index(strings.ToLower(m[0]), "x")
		return parseNum(m[0][idx+1:])
	}
	if m := eOnlyRe.FindStringSubmatch(lower); m != nil {
		return parseNum(m[1])
	}
	return 0
}

// CleanEpisodeFileName keeps only the clean short form of a release name:
// "Some.Show.S02E04.1080p.x264.Hindi...Msubs.RG.mkv"
// becomes "Some.Show.S02E04.mkv"
func CleanEpisodeFileName(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	base := filename[:len(filename)-len(ext)]

	// find the earliest episode marker and keep up to and including it
	markers := []struct {
		re *regexp.Regexp
	}{
		{regexp.MustCompile(`(?i)\bs[0-9]{1,2}[._ -]?e[0-9]{1,3}\b`)},
		{regexp.MustCompile(`(?i)\b[0-9]{1,2}x[0-9]{1,3}\b`)},
		{regexp.MustCompile(`(?i)(?:\b|\s)E[0-9]{1,3}\b`)},
	}
	bestEnd := -1
	bestMarker := ""
	for _, m := range markers {
		if loc := m.re.FindStringIndex(base); loc != nil {
			if bestEnd == -1 || loc[1] < bestEnd {
				bestEnd = loc[1]
				bestMarker = m.re.FindString(base)
			}
		}
	}
	if bestEnd == -1 {
		// no episode marker: strip tags but keep the base title
		return SanitizeFileName(base) + ext
	}
	marker := bestMarker
	norm := regexp.MustCompile(`(?i)\bs([0-9]{1,2})[._ -]?e([0-9]{1,3})\b`).FindStringSubmatch(marker)
	var cleanMarker string
	if norm != nil {
		cleanMarker = "S" + norm[1] + "E" + norm[2]
	} else if xm := regexp.MustCompile(`(?i)\b([0-9]{1,2})x([0-9]{1,3})\b`).FindStringSubmatch(marker); xm != nil {
		cleanMarker = xm[1] + "x" + xm[2]
	} else {
		cleanMarker = strings.TrimRight(marker, "._ -")
	}
	title := strings.TrimRight(strings.TrimSuffix(base[:bestEnd], marker), " ._")
	if title == "" {
		title = cleanMarker
		return SanitizeFileName(title) + ext
	}
	return SanitizeFileName(title+"."+cleanMarker) + ext
}

// CleanMovieFolderName derives a "Title (Year)" folder name from a release
// or torrent name, keeping the year (unlike CleanSeasonPackName):
// "Example Movie (2024) [1080p] [WEBRip] [5.1] [DEMO]" and
// "Example.Movie.2024.1080p.WEBRip.x264.mp4" both become "Example Movie (2024)".
// Returns "" when no usable title remains.
func CleanMovieFolderName(name string) string {
	s := strings.TrimSpace(name)
	// strip a real file extension only (tag brackets like "[GRP]" must
	// not count as one)
	if ext := filepath.Ext(s); ext != "" {
		if ok, _ := regexp.MatchString(`(?i)^\.[a-z0-9]{2,4}$`, ext); ok {
			s = strings.TrimSuffix(s, ext)
		}
	}
	year := ""
	if m := movieYearRe.FindStringSubmatch(s); m != nil {
		year = m[1]
	}
	s = bracketRe.ReplaceAllString(s, " ")
	s = parenRe.ReplaceAllString(s, " ")
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	s = tagCut.ReplaceAllString(s, "")
	s = yearBareRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "-_~ ")
	if s == "" {
		return ""
	}
	title := TitleCase(strings.ToLower(s))
	if year != "" {
		return title + " (" + year + ")"
	}
	return title
}

// CleanSeasonPackName derives a clean series folder name from a season-pack
// archive/file name: "Mousetrap.S01.480p.x264...Msubs.RG"
// becomes "Mousetrap".
func CleanSeasonPackName(name string) string {
	s := strings.NewReplacer(".", " ", "_", " ").Replace(name)
	s = regexp.MustCompile(`(?i)\bS[0-9]{1,2}(E[0-9]{1,3})?\b.*`).ReplaceAllString(s, "")
	s = tagCut.ReplaceAllString(s, "")
	s = yearParenRe.ReplaceAllString(s, "")
	s = yearBareRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	// release names often leave a dangling separator ("Anime Show")
	s = strings.TrimRight(s, "-_~ ")
	return s
}

func parseNum(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
