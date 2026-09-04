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
	Detail    string   // e.g. "TV Series • Season 1 detected"
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

// OrganizeDirectory organizes every file inside a downloaded directory
// (e.g. a torrent root). Videos and regular files are routed by type;
// sidecar files (subtitles, artwork, nfo, release notes) are kept next to
// the video they belong to so players like Jellyfin/Plex pick them up.
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

	var videos, sidecars, regular []string
	for _, f := range files {
		switch {
		case IsVideo(filepath.Base(f)):
			videos = append(videos, f)
		case IsSidecar(filepath.Base(f)):
			sidecars = append(sidecars, f)
		default:
			regular = append(regular, f)
		}
	}

	// no episodes: keep the whole release together in one folder inside
	// its primary category (Movies/Film (Year)/...), like a movie torrent
	hasEpisode := false
	for _, v := range videos {
		if IsEpisode(filepath.Base(v)) {
			hasEpisode = true
			break
		}
	}
	if !hasEpisode {
		return o.organizeReleaseGroup(dir, files, report)
	}

	total := len(files)
	done := 0
	// videoDest remembers where each video landed for sidecar matching
	videoDest := map[string]string{}
	for _, v := range videos {
		reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: done, Total: total})
		done++
		before := len(res.Moved)
		o.routeFile(v, res)
		if len(res.Moved) > before {
			videoDest[v] = filepath.Dir(res.Moved[len(res.Moved)-1])
		}
	}
	for _, f := range regular {
		reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: done, Total: total})
		done++
		o.routeFile(f, res)
	}
	for _, s := range sidecars {
		reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: done, Total: total})
		done++
		if destDir := matchSidecarDir(s, videos, videoDest); destDir != "" {
			o.moveToDir(s, destDir, res)
		} else {
			o.routeFile(s, res)
		}
	}
	// remove now-empty source dir (never the filesystem root)
	if c := filepath.Clean(dir); c != "/" && c != "." && c != "" {
		_ = os.RemoveAll(dir)
	}

	res.Duration = time.Since(start)
	return res, nil
}

// startsOnBoundary reports whether s starts with prefix followed by a
// separator (., -, _), so "show.s01e01" never matches "show.s01e011".
func startsOnBoundary(s, prefix string) bool {
	for _, sep := range []string{".", "-", "_"} {
		if strings.HasPrefix(s, prefix+sep) {
			return true
		}
	}
	return false
}

// organizeReleaseGroup keeps a non-episodic release (movie + subs/artwork,
// music album, ...) together in one folder inside its primary category:
// Movies/Example Movie (2024)/.... Falls back to per-file routing when the
// folder cannot be determined.
func (o *Organizer) organizeReleaseGroup(dir string, files []string, report Reporter) (*Result, error) {
	start := time.Now()
	res := &Result{Started: start}

	routeEach := func() {
		total := len(files)
		for i, f := range files {
			reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: i, Total: total})
			o.routeFile(f, res)
		}
		if c := filepath.Clean(dir); c != "/" && c != "." && c != "" {
			_ = os.RemoveAll(dir)
		}
		res.Duration = time.Since(start)
	}

	if len(files) == 0 {
		routeEach()
		return res, nil
	}
	primary := ""
	for _, f := range files {
		if IsVideo(filepath.Base(f)) {
			primary = Categorize(filepath.Ext(f))
			break
		}
	}
	if primary == "" {
		primary = Categorize(filepath.Ext(files[0]))
	}
	destRoot := o.Paths.dirFor(primary)
	folder := CleanMovieFolderName(filepath.Base(dir))
	if destRoot == "" || folder == "" {
		routeEach()
		return res, nil
	}
	destDir := filepath.Join(destRoot, SanitizeFolderName(folder))
	if err := ensureDir(destDir); err != nil {
		routeEach()
		return res, nil
	}
	total := len(files)
	for i, f := range files {
		reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: i, Total: total})
		o.moveFile(f, filepath.Join(destDir, filepath.Base(f)), res)
	}
	if c := filepath.Clean(dir); c != "/" && c != "." && c != "" {
		_ = os.RemoveAll(dir)
	}
	res.Category = primary
	res.Duration = time.Since(start)
	return res, nil
}

// OrganizeTorrentFile routes a single-file torrent download into a dedicated
// folder inside its category: Movies/Example Movie (2024)/file. Episodes keep
// the series/show/season placement. report may be nil.
func (o *Organizer) OrganizeTorrentFile(srcPath, torrentName string, report Reporter) (*Result, error) {
	start := time.Now()
	res := &Result{Started: start}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return o.OrganizeDirectory(srcPath, report)
	}
	name := filepath.Base(srcPath)
	if IsEpisode(name) {
		o.routeEpisode(srcPath, res, report)
		res.Duration = time.Since(start)
		return res, nil
	}
	category := Categorize(filepath.Ext(srcPath))
	destRoot := o.Paths.dirFor(category)
	folder := CleanMovieFolderName(torrentName)
	if folder == "" {
		folder = CleanMovieFolderName(name)
	}
	if destRoot == "" || folder == "" {
		r, err := o.OrganizeFile(srcPath, report)
		return r, err
	}
	destDir := filepath.Join(destRoot, SanitizeFolderName(folder))
	if err := ensureDir(destDir); err != nil {
		r, err := o.OrganizeFile(srcPath, report)
		return r, err
	}
	reportFn(report, Progress{Step: "moving", Detail: "Analyzing content", Done: 0, Total: 1})
	o.moveFile(srcPath, filepath.Join(destDir, name), res)
	res.Category = category
	res.Duration = time.Since(start)
	return res, nil
}

// matchSidecarDir finds the destination directory of the video a sidecar
// file belongs to: same basename (movie.srt -> movie.mp4, including tagged
// variants like movie.SDH.eng.srt), or the only routed video as fallback.
// Returns "" when no video matches (caller routes the file by type).
func matchSidecarDir(sidecar string, videos []string, videoDest map[string]string) string {
	candidates := make([]string, 0, len(videos))
	for _, v := range videos {
		if d, ok := videoDest[v]; ok && d != "" {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sb := strings.ToLower(strings.TrimSuffix(filepath.Base(sidecar), filepath.Ext(sidecar)))
	best, bestLen := "", -1
	for _, v := range candidates {
		vb := strings.ToLower(strings.TrimSuffix(filepath.Base(v), filepath.Ext(filepath.Base(v))))
		// match either direction on a separator boundary: the sub often
		// drops quality tags present in the video name and vice versa
		if sb == vb || startsOnBoundary(sb, vb) || startsOnBoundary(vb, sb) {
			if len(vb) > bestLen {
				best, bestLen = v, len(vb)
			}
		}
	}
	if best != "" {
		return videoDest[best]
	}
	if len(candidates) == 1 {
		return videoDest[candidates[0]]
	}
	return ""
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

// moveToDir moves a file into an already-decided destination directory,
// keeping its original file name (used for sidecars following their video).
func (o *Organizer) moveToDir(srcPath, destDir string, res *Result) {
	if destDir == "" {
		o.moveTo(srcPath, o.Paths.Others, res)
		return
	}
	if err := ensureDir(destDir); err != nil {
		res.Failed = append(res.Failed, srcPath)
		return
	}
	o.moveFile(srcPath, filepath.Join(destDir, filepath.Base(srcPath)), res)
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

// MoveToMovies moves any file into the movies category (public helper).
func (o *Organizer) MoveToMovies(srcPath string) {
	o.moveTo(srcPath, o.Paths.Movies, &Result{})
}

// MoveToArchives moves the archive file into the archives category.
func (o *Organizer) MoveToArchives(srcPath string) {
	o.moveTo(srcPath, o.Paths.Archives, &Result{})
}
