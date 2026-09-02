package organize

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Progress is reported to Telegram during organizing.
type Progress struct {
	Step    string // "analyzing" | "moving" | "done"
	Detail  string // e.g. "TV Series • Season 1 detected"
	Done    int    // items completed
	Total   int    // total items
	Message string // rendered message body
}

// Reporter receives live progress updates.
type Reporter func(p Progress)

// ReporterFunc no-op when nil
func (r Reporter) report(p Progress) {
	if r != nil {
		r(p)
	}
}

// Result summarizes one organize run.
type Result struct {
	Category  string
	Detail    string // e.g. "TV Series • Season 1 detected"
	Moved     []string // final destination paths
	Failed    []string
	Tree      string // rendered tree for the final message
	SizeBytes int64
	Started   time.Time
	Duration  time.Duration
}

// Organizer routes completed downloads into the media library.
type Organizer struct {
	Paths Paths
	// AniList lookups (nil disables)
	AniList bool
	// Post is the HTTP POST func used for AniList (injectable for tests)
	Post func(url string, body string, timeout time.Duration) (int, string)
	// Log every decision
	Logf func(format string, v ...interface{})
}

func (o *Organizer) log(format string, v ...interface{}) {
	if o.Logf != nil {
		o.Logf(format, v...)
	}
}

// OrganizeFile routes a single downloaded file/dir into the library.
func (o *Organizer) OrganizeFile(srcPath string, report Reporter) (*Result, error) {
	start := time.Now()
	res := &Result{Started: start}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}

	// directory download (e.g. torrent with folder): organize contents
	if info.IsDir() {
		return o.OrganizeDirectory(srcPath, report)
	}

	ext := filepath.Ext(srcPath)
	category := Categorize(ext)

	switch category {
	case CatMovies:
		// refine: episode -> series/anime
		name := filepath.Base(srcPath)
		if IsEpisode(name) {
			o.routeEpisode(srcPath, res, report)
		} else {
			o.moveTo(srcPath, o.Paths.Movies, res)
		}
	case CatArchives:
		// handled by the extract pipeline (Phase 3); default keep in archives
		o.moveTo(srcPath, o.Paths.Archives, res)
	default:
		o.moveTo(srcPath, o.Paths.dirFor(category), res)
	}

	res.Category = category
	res.Duration = time.Since(start)
	return res, nil
}

// OrganizeDirectory organizes every file inside a downloaded directory.
func (o *Organizer) OrganizeDirectory(dir string, report Reporter) (*Result, error) {
	start := time.Now()
	res := &Result{Started: start}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	total := len(files)
	for i, f := range files {
		reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: i, Total: total})
		o.routeFile(f, res)
	}
	// remove now-empty source dir
	_ = os.RemoveAll(dir)

	res.Duration = time.Since(start)
	return res, nil
}

// routeFile dispatches one file by extension.
func (o *Organizer) routeFile(f string, res *Result) {
	ext := filepath.Ext(f)
	switch Categorize(ext) {
	case CatMovies:
		if IsEpisode(filepath.Base(f)) {
			o.routeEpisode(f, res, nil)
		} else {
			o.moveTo(f, o.Paths.Movies, res)
		}
	default:
		o.moveTo(f, o.Paths.dirFor(Categorize(ext)), res)
	}
}

// routeEpisode places an episode file into series/ or anime/ with Season N.
// Follows the AriaFlow decision tree:
//  1. existing series folder match -> series
//  2. existing anime folder match  -> anime
//  3. AniList decides when enabled -> anime else series
func (o *Organizer) routeEpisode(srcPath string, res *Result, report Reporter) {
	name := filepath.Base(srcPath)
	norm := NormalizeName(name)

	seriesMatcher := FolderMatcher{Root: o.Paths.Series}
	if p := seriesMatcher.Find(norm); p != "" {
		o.log("Series folder match for '%s' -> %s", name, filepath.Base(p))
		o.placeEpisodeIn(srcPath, p, res)
		return
	}

	animeMatcher := FolderMatcher{Root: o.Paths.Anime}
	if p := animeMatcher.Find(norm); p != "" {
		o.log("Anime folder match for '%s' -> %s", name, filepath.Base(p))
		o.placeAnimeIn(srcPath, p, res)
		return
	}

	if o.AniList && o.Post != nil {
		r := IsAnimeAnilist(norm, o.Post)
		if r.IsAnime {
			o.log("AniList confirmed anime '%s' -> '%s'", norm, r.Title)
			folder := r.Title
			if folder == "" {
				folder = TitleCase(norm)
			}
			animeRoot := filepath.Join(o.Paths.Anime, SanitizeFolderName(folder))
			if err := ensureDir(animeRoot); err == nil {
				o.placeAnimeIn(srcPath, animeRoot, res)
				return
			}
		} else if r.Unreachable {
			o.log("AniList unreachable - defaulting to series for '%s'", name)
		} else {
			o.log("AniList: not anime - routing to series for '%s'", name)
		}
	}

	// no match anywhere: create series/<Clean Title>/Season N
	season := ExtractSeason(name)
	cleanTitle := CleanEpisodeFileName(name)
	title := CleanSeasonPackName(norm)
	if title == "" {
		title = TitleCase(norm)
	}
	seriesRoot := filepath.Join(o.Paths.Series, SanitizeFolderName(TitleCase(strings.ToLower(title))))
	if season > 0 {
		seasonDir := filepath.Join(seriesRoot, fmt.Sprintf("Season %d", season))
		if err := ensureDir(seasonDir); err == nil {
			dst := filepath.Join(seasonDir, SanitizeFileName(cleanTitle))
			o.moveFile(srcPath, dst, res)
			return
		}
	}
	if err := ensureDir(seriesRoot); err == nil {
		o.moveFile(srcPath, filepath.Join(seriesRoot, SanitizeFileName(cleanTitle)), res)
	}
}

// placeEpisodeIn places an episode under an existing series folder.
func (o *Organizer) placeEpisodeIn(srcPath, seriesRoot string, res *Result) {
	name := filepath.Base(srcPath)
	season := ExtractSeason(name)
	if season > 0 {
		seasonDir := filepath.Join(seriesRoot, fmt.Sprintf("Season %d", season))
		if err := ensureDir(seasonDir); err == nil {
			o.moveFile(srcPath, filepath.Join(seasonDir, SanitizeFileName(CleanEpisodeFileName(name))), res)
			return
		}
	}
	// episode without clear season -> series root
	o.moveFile(srcPath, filepath.Join(seriesRoot, SanitizeFileName(CleanEpisodeFileName(name))), res)
}

// placeAnimeIn places an episode under an existing anime folder.
func (o *Organizer) placeAnimeIn(srcPath, animeRoot string, res *Result) {
	name := filepath.Base(srcPath)
	season := ExtractSeason(name)
	if season > 0 {
		seasonDir := filepath.Join(animeRoot, fmt.Sprintf("Season %d", season))
		if err := ensureDir(seasonDir); err == nil {
			o.moveFile(srcPath, filepath.Join(seasonDir, SanitizeFileName(CleanEpisodeFileName(name))), res)
			return
		}
	}
	o.moveFile(srcPath, filepath.Join(animeRoot, SanitizeFileName(CleanEpisodeFileName(name))), res)
}

// moveTo moves a file into a category root.
func (o *Organizer) moveTo(srcPath, destRoot string, res *Result) {
	if destRoot == "" {
		o.log("No destination configured for '%s' - skipped", srcPath)
		res.Failed = append(res.Failed, srcPath)
		return
	}
	if err := ensureDir(destRoot); err != nil {
		res.Failed = append(res.Failed, srcPath)
		return
	}
	o.moveFile(srcPath, filepath.Join(destRoot, filepath.Base(srcPath)), res)
}

// moveFile moves src to dst, never overwriting existing files
// (adds a numeric suffix when the destination exists).
func (o *Organizer) moveFile(src, dst string, res *Result) {
	if src == dst {
		res.Moved = append(res.Moved, dst)
		return
	}
	res.SizeBytes += fileSize(src)
	dst = uniquePath(dst)
	if err := os.Rename(src, dst); err != nil {
		// cross-device: fall back to copy+delete
		if cerr := copyFile(src, dst); cerr != nil {
			o.log("Move failed %s -> %s: %v", src, dst, err)
			res.Failed = append(res.Failed, src)
			return
		}
		_ = os.Remove(src)
	}
	o.log("Moved: %s -> %s", filepath.Base(src), dst)
	res.Moved = append(res.Moved, dst)
}

// uniquePath appends " (1)", " (2)", ... before the extension when dst exists.
func uniquePath(dst string) string {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return dst
	}
	ext := filepath.Ext(dst)
	base := dst[:len(dst)-len(ext)]
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileSize(path string) int64 {
	s, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return s.Size()
}


// reportFn safely invokes a Reporter (nil-safe).
func reportFn(r Reporter, p Progress) {
	if r != nil {
		r(p)
	}
}
