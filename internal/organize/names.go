// Package organize implements post-download media organization with
// directory-structure parity to AriaFlow's organize_download.sh.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
// Derived from DownloadBot by gaowanliang (Apache-2.0).
package organize

import (
	"net/url"
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
	tagCut        = regexp.MustCompile(`(?i)\b(720p|1080p|2160p|480p|360p|4k|uhd|bluray|blu-ray|bdrip|bd|brrip|webrip|web-dl|webdl|web|hdtv|dvdrip|dvdscr|x264|x265|h\.?264|h\.?265|hevc|avc|aac|ac3|eac3|dts|dtshd|truehd|atmos|hdr|hdr10|dolby|vision|10bit|8bit|remux|extended|repack|proper|remastered|unrated|dual|audio|sub|subs|msubs|esubs|subbed|dubbed|hindi|korean|japanese|chinese|taiwanese|mandarin|english|RG|org|netflix|amzn|nf|dsnp|hulu|hmax|max|pc|rip)\b.*`)
	bracketRe     = regexp.MustCompile(`\[[^\]]*\]`)
	parenRe       = regexp.MustCompile(`\([^)]*\)`)
	movieYearRe   = regexp.MustCompile(`\b((?:19|20)[0-9]{2})\b`)
	// editionCut matches "edition" phrases (IMAX, director's cut,
	// theatrical/final/ultimate cuts, special editions, uncut, Criterion,
	// open matte, HFR, ...). Applied to post-year text only, so genuine
	// title words like "The Final Cut (2004)" are never stripped.
	editionCut = regexp.MustCompile(`(?i)\b(imax|uncut|criterion|open[\s._-]*matte|hfr|[46]0fps|120fps|3d|directors?(?:['’]s)?[\s._-]*cut|theatrical(?:[\s._-]*cut)?|final[\s._-]*cut|ultimate(?:[\s._-]*cut|[\s._-]*edition)?|special[\s._-]*edition|extended[\s._-]*edition)\b.*`)
	spaceRe       = regexp.MustCompile(`\s+`)
	seRe          = regexp.MustCompile(`(?i)\bs([0-9]{1,2})[._ -]?e[0-9]{1,3}\b`)
	eOnlyRe       = regexp.MustCompile(`(?i)(?:\b|\s)e([0-9]{1,3})\b`)
	// xFullRe captures season + episode of the NxM form so codec tags
	// like x264/x265 can be excluded (see isCodecXMatch).
	xFullRe = regexp.MustCompile(`(?i)\b([0-9]{1,2})x([0-9]{1,3})\b`)
	standaloneSRe = regexp.MustCompile(`(?i)\bs([0-9]{1,2})\b`)
)

// decodeName URL-decodes percent-encoded names ("%20" -> " "). Download
// backends sometimes leave HTTP-encoded names on disk
// ("Sample%20Film%202025...%20x264..."), where the trailing "20" of
// "%20" glues to the codec ("20x264") and fakes an episode tag.
// PathUnescape fails on stray "%" (e.g. "100% Wolf"), so fall back to a
// minimal %20 -> space replacement that never destroys a literal "%".
func decodeName(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	s = strings.ReplaceAll(s, "%20", " ")
	// hex escapes with letters (case-insensitive) for common separators
	s = strings.ReplaceAll(s, "%2e", ".")
	s = strings.ReplaceAll(s, "%2E", ".")
	s = strings.ReplaceAll(s, "%2d", "-")
	s = strings.ReplaceAll(s, "%2D", "-")
	s = strings.ReplaceAll(s, "%5b", "[")
	s = strings.ReplaceAll(s, "%5B", "[")
	s = strings.ReplaceAll(s, "%5d", "]")
	s = strings.ReplaceAll(s, "%5D", "]")
	return s
}

// isCodecXMatch reports whether an NxM match is really a video codec tag
// (x264/x265). A glued "%20x264" decodes to " x264" and never reaches here,
// but "Movie.1x264" style names must still not count as season 1 episode 264.
func isCodecXMatch(season, episode string) bool {
	if len(season) == 0 || len(episode) == 0 {
		return false
	}
	return episode == "264" || episode == "265"
}

// firstNonCodecX returns the start index of the first non-codec NxM marker,
// or -1 when there is none.
func firstNonCodecX(s string) int {
	locs := xFullRe.FindAllStringSubmatchIndex(s, -1)
	for _, loc := range locs {
		season := s[loc[2]:loc[3]]
		episode := s[loc[4]:loc[5]]
		if !isCodecXMatch(season, episode) {
			return loc[0]
		}
	}
	return -1
}

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
	name = decodeName(name)
	s := strings.ToLower(strings.TrimSpace(name))
	s = extRe.ReplaceAllString(s, "")
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(s)
	s = yearParenRe.ReplaceAllString(s, "")
	s = yearBareRe.ReplaceAllString(s, "")
	// cut everything from the episode/season tag onward
	if loc := seasonEpCut.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	} else if idx := firstNonCodecX(s); idx != -1 {
		// NxM marker that is not a glued codec tag (x264/x265 excluded
		// inside firstNonCodecX); cut from the marker onward like xeCut.
		s = s[:idx]
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
// A bare year like "2019" is NOT an episode tag. Codec tags x264/x265 are
// NOT episode tags (1x264 would otherwise fake S01E264), and percent-encoded
// names are decoded first so "%20x264" never fakes S20E264.
func IsEpisode(filename string) bool {
	f := decodeName(filename)
	if seRe.MatchString(f) {
		return true
	}
	for _, m := range xFullRe.FindAllStringSubmatch(f, -1) {
		if !isCodecXMatch(m[1], m[2]) {
			return true
		}
	}
	return eOnlyRe.MatchString(f)
}

// ExtractSeason returns the season number (no leading zero) from patterns
// like S04E06, S4E6, 4x06, s04.e06. Returns 0 if no season found.
// x264/x265 codec tags never count as a season.
func ExtractSeason(filename string) int {
	lower := strings.ToLower(decodeName(filename))
	if m := seRe.FindStringSubmatch(lower); m != nil {
		return parseNum(m[1])
	}
	for _, m := range xFullRe.FindAllStringSubmatch(lower, -1) {
		if isCodecXMatch(m[1], m[2]) {
			continue
		}
		return parseNum(m[1])
	}
	// s01 standalone (season pack folder or file without episode)
	if m := standaloneSRe.FindStringSubmatch(lower); m != nil {
		return parseNum(m[1])
	}
	return 0
}

// ExtractEpisode returns the episode number from the file name, 0 if none.
// x264/x265 codec tags never count as an episode.
func ExtractEpisode(filename string) int {
	lower := strings.ToLower(decodeName(filename))
	if m := seRe.FindStringSubmatch(lower); m != nil {
		idx := strings.Index(strings.ToLower(m[0]), "e")
		return parseNum(m[0][idx+1:])
	}
	for _, m := range xFullRe.FindAllStringSubmatch(lower, -1) {
		if isCodecXMatch(m[1], m[2]) {
			continue
		}
		return parseNum(m[2])
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
	filename = decodeName(filename)
	ext := strings.ToLower(filepath.Ext(filename))
	base := filename[:len(filename)-len(ext)]

	// find the earliest episode marker and keep up to and including it;
	// x264/x265 codec tags are skipped so they never fake an NxM marker.
	bestEnd := -1
	bestMarker := ""
	if loc := regexp.MustCompile(`(?i)\bs[0-9]{1,2}[._ -]?e[0-9]{1,3}\b`).FindStringIndex(base); loc != nil {
		bestEnd = loc[1]
		bestMarker = base[loc[0]:loc[1]]
	}
	for _, loc := range xFullRe.FindAllStringIndex(base, -1) {
		m := xFullRe.FindStringSubmatch(base[loc[0]:loc[1]])
		if m != nil && isCodecXMatch(m[1], m[2]) {
			continue
		}
		if bestEnd == -1 || loc[1] < bestEnd {
			bestEnd = loc[1]
			bestMarker = base[loc[0]:loc[1]]
		}
		break // x markers are left-to-right; first non-codec is earliest
	}
	if loc := regexp.MustCompile(`(?i)(?:\b|\s)E[0-9]{1,3}\b`).FindStringIndex(base); loc != nil {
		if bestEnd == -1 || loc[1] < bestEnd {
			bestEnd = loc[1]
			bestMarker = base[loc[0]:loc[1]]
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
	s := strings.TrimSpace(decodeName(name))
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
	// strip edition tags (IMAX, director's cut, ...) from the post-year
	// segment only: title words before the year are left intact
	if loc := movieYearRe.FindStringIndex(s); loc != nil {
		pre, post := s[:loc[0]], s[loc[0]:]
		s = pre + editionCut.ReplaceAllString(post, "")
	}
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

// CleanMovieFileName derives a clean "Title (Year).ext" file name from a
// release file name, keeping the year (unlike CleanEpisodeFileName):
// "Example.Movie.2024.1080p.WEBRip.x264.mp4" becomes
// "Example Movie (2024).mp4". Falls back to the sanitized original name
// when no usable title remains.
func CleanMovieFileName(filename string) string {
	ext := filepath.Ext(filename)
	folderRaw := CleanMovieFolderName(filename)
	if folderRaw == "" {
		return SanitizeFileName(filename)
	}
	folder := SanitizeFolderName(folderRaw)
	return SanitizeFileName(folder + ext)
}

// CleanSeasonPackName derives a clean series folder name from a season-pack
// archive/file name: "Mousetrap.S01.480p.x264...Msubs.RG"
// becomes "Mousetrap".
func CleanSeasonPackName(name string) string {
	name = decodeName(name)
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
