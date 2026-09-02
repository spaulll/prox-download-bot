package organize

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// SeasonPackInput describes a batch of extracted episode files.
type SeasonPackInput struct {
	// SourceName is the original archive/file name (used for series + season detection)
	SourceName string
	// EpisodeFiles are the extracted video files with episode tags
	EpisodeFiles []string
	// OtherFiles are extracted files without episode tags (extras, trailers...)
	OtherFiles []string
}

// OrganizeSeasonPack routes a season pack (2+ episodes) into
// <series|anime>/<Clean Title>/Season N/<clean names>.
func (o *Organizer) OrganizeSeasonPack(in SeasonPackInput, report Reporter) (*Result, error) {
	start := timeNow()
	res := &Result{Started: start}

	nameNoExt := strings.TrimSuffix(in.SourceName, filepath.Ext(in.SourceName))
	cleanSeries := CleanSeasonPackName(nameNoExt)
	if cleanSeries == "" {
		// fall back to first episode name
		if len(in.EpisodeFiles) > 0 {
			cleanSeries = CleanSeasonPackName(NormalizeName(filepath.Base(in.EpisodeFiles[0])))
		}
	}
	o.log("Season pack detected: %d episodes in %s | clean series: '%s'",
		len(in.EpisodeFiles), in.SourceName, cleanSeries)

	season := ExtractSeason(nameNoExt)
	if season == 0 && len(in.EpisodeFiles) > 0 {
		season = ExtractSeason(filepath.Base(in.EpisodeFiles[0]))
	}
	if season == 0 {
		season = 1
	}

	isAnime := false
	if o.AniList && o.Post != nil {
		r := IsAnimeAnilist(cleanSeries, o.Post)
		isAnime = r.IsAnime
		if r.IsAnime && r.Title != "" {
			cleanSeries = r.Title
		}
	}

	var root string
	sanitized := SanitizeFolderName(TitleCase(strings.ToLower(cleanSeries)))
	if isAnime {
		m := FolderMatcher{Root: o.Paths.Anime}
		root = m.Find(NormalizeName(cleanSeries))
		if root == "" {
			root = filepath.Join(o.Paths.Anime, sanitized)
			if err := ensureDir(root); err != nil {
				return res, err
			}
			o.log("Created new anime folder: %s", root)
		}
		res.Category = CatAnime
	} else {
		m := FolderMatcher{Root: o.Paths.Series}
		root = m.Find(NormalizeName(cleanSeries))
		if root == "" {
			root = filepath.Join(o.Paths.Series, sanitized)
			if err := ensureDir(root); err != nil {
				return res, err
			}
			o.log("Created new series folder: %s", root)
		}
		res.Category = CatSeries
	}

	seasonDir := filepath.Join(root, fmt.Sprintf("Season %d", season))
	if err := ensureDir(seasonDir); err != nil {
		return res, err
	}
	res.Detail = fmt.Sprintf("%s • Season %d detected", CategoryLabel(res.Category), season)

	total := len(in.EpisodeFiles)
	for i, ep := range in.EpisodeFiles {
		reportFn(report, Progress{Step: "moving", Detail: res.Detail, Done: i, Total: total})
		dst := filepath.Join(seasonDir, SanitizeFileName(CleanEpisodeFileName(filepath.Base(ep))))
		o.moveFile(ep, dst, res)
	}
	reportFn(report, Progress{Step: "moving", Detail: res.Detail, Done: total, Total: total})

	// extras -> movies
	for _, other := range in.OtherFiles {
		o.log("Non-episode file in pack: %s - moving to movies/", filepath.Base(other))
		o.moveTo(other, o.Paths.Movies, res)
	}

	res.Duration = timeSince(start)
	return res, nil
}

// OrganizeSingleEpisode routes one extracted episode file using the
// single-episode decision tree (never treated as a season pack).
func (o *Organizer) OrganizeSingleEpisode(srcPath string, report Reporter) (*Result, error) {
	start := timeNow()
	res := &Result{Started: start}
	o.routeEpisode(srcPath, res, report)
	res.Duration = timeSince(start)
	return res, nil
}

var timeNow = time.Now
var timeSince = time.Since
