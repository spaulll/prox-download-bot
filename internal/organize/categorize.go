package organize

import (
	"os"
	"path/filepath"
	"strings"
)

// Category labels
const (
	CatMovies    = "movies"
	CatSeries    = "series"
	CatAnime     = "anime"
	CatMusic     = "music"
	CatDocuments = "documents"
	CatArchives  = "archives"
	CatOthers    = "others"
)

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
	".flv": true, ".wmv": true, ".webm": true, ".m4v": true, ".mpg": true, ".mpeg": true, ".ts": true,
}

var audioExts = map[string]bool{
	".mp3": true, ".flac": true, ".wav": true, ".ogg": true, ".m4a": true, ".wma": true, ".opus": true,
}

var docExts = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".txt": true, ".epub": true, ".mobi": true,
}

var archiveExts = map[string]bool{
	".zip": true, ".rar": true, ".7z": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
}

// Categorize maps a file extension to a category label.
func Categorize(ext string) string {
	switch {
	case videoExts[strings.ToLower(ext)]:
		return CatMovies // router refines series/anime
	case audioExts[strings.ToLower(ext)]:
		return CatMusic
	case docExts[strings.ToLower(ext)]:
		return CatDocuments
	case archiveExts[strings.ToLower(ext)]:
		return CatArchives
	default:
		return CatOthers
	}
}

// IsVideo/IsArchive helpers used by the extraction pipeline
func IsVideo(name string) bool   { return videoExts[strings.ToLower(filepath.Ext(name))] }
func IsAudio(name string) bool   { return audioExts[strings.ToLower(filepath.Ext(name))] }
func IsDoc(name string) bool     { return docExts[strings.ToLower(filepath.Ext(name))] }
func IsArchive(name string) bool { return archiveExts[strings.ToLower(filepath.Ext(name))] }

// Paths holds the resolved library directory layout.
type Paths struct {
	Movies    string
	Series    string
	Anime     string
	Music     string
	Documents string
	Archives  string
	Others    string
}

func (p Paths) dirFor(category string) string {
	switch category {
	case CatSeries:
		return p.Series
	case CatAnime:
		return p.Anime
	case CatMusic:
		return p.Music
	case CatDocuments:
		return p.Documents
	case CatArchives:
		return p.Archives
	case CatOthers:
		return p.Others
	default:
		return p.Movies
	}
}

// CategoryLabel returns the display label of a category for messages.
func CategoryLabel(category string) string {
	switch category {
	case CatMovies:
		return "Movie"
	case CatSeries:
		return "TV Series"
	case CatAnime:
		return "Anime"
	case CatMusic:
		return "Music"
	case CatDocuments:
		return "Documents"
	case CatArchives:
		return "Archive"
	default:
		return "Other"
	}
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
