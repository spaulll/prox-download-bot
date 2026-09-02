package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/internal/extract"
	"DownloadBot/internal/organize"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// runArchivePipeline handles a completed archive download:
// detect -> extract with live progress -> re-run organize on contents
// -> final summary. Plan-style messages throughout.
func runArchivePipeline(bot *tgBotApi.BotAPI, chatID int64, org *organize.Organizer, srcPath, displayName string) {
	start := time.Now()

	// "Archive detected" message (plan style)
	live := NewOrganizeProgressMsg(bot, chatID,
		"🗂 Organizing...\n\n📦 Archive detected\n→ Preparing extraction")
	if live == nil {
		return
	}

	cfg := config.GetOrganizeConfig()
	ext := &extract.Extractor{Logf: func(f string, v ...interface{}) { logger.Info(f, v...) }}
	if !extract.IsArchivePath(srcPath) {
		live.Update("🗂 Organizing...\n\n⚠️ Unsupported archive format\n" + filepath.Base(srcPath))
		return
	}

	// staging dir next to the archive
	stageDir := filepath.Join(os.TempDir(), fmt.Sprintf("proxbot_extract_%d", time.Now().UnixNano()))
	defer os.RemoveAll(stageDir)

	// "Extracting" live progress
	var lastUpdate time.Time
	progress := func(done, total int, current string) {
		if total <= 0 {
			return
		}
		if time.Since(lastUpdate) < time.Second {
			return
		}
		lastUpdate = time.Now()
		live.Update("🗂 Organizing...\n\n" +
			"📦 Extracting\n" +
			progressBar(done, total) + "\n" +
			fmt.Sprintf("%d/%d files", done, total))
	}

	live.Update("🗂 Organizing...\n\n📦 Extracting\n" + progressBar(0, 1) + "\n0/1 files")
	if err := ext.Extract(srcPath, stageDir, progress); err != nil {
		logger.Error("extract failed for %s: %v", srcPath, err)
		live.Update("🗂 Organizing...\n\n⚠️ Extraction failed\n" + err.Error())
		return
	}
	logger.Info("extracted %s -> %s", srcPath, stageDir)

	// inventory extracted content (AriaFlow style)
	var episodeFiles, otherVideos, otherFiles []string
	err := filepath.Walk(stageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := filepath.Base(path)
		switch {
		case organize.IsVideo(name) && organize.IsEpisode(name):
			episodeFiles = append(episodeFiles, path)
		case organize.IsVideo(name):
			otherVideos = append(otherVideos, path)
		default:
			otherFiles = append(otherFiles, path)
		}
		return nil
	})
	if err != nil {
		live.Update("🗂 Organizing...\n\n⚠️ Failed to read extracted content")
		return
	}

	var res *organize.Result
	switch {
	case len(episodeFiles) >= 2:
		// SEASON PACK
		live.Update("🗂 Organizing...\n\n🔍 Analyzing content\n→ TV Series • Season pack detected")
		res, err = org.OrganizeSeasonPack(organize.SeasonPackInput{
			SourceName:   displayName,
			EpisodeFiles: episodeFiles,
			OtherFiles:   append(otherVideos, otherFiles...),
		}, func(p organize.Progress) {
			if p.Total <= 0 || time.Since(lastUpdate) < time.Second {
				return
			}
			lastUpdate = time.Now()
			live.Update("🗂 Organizing...\n\n" +
				"🔍 Analyzing content\n" +
				p.Detail + "\n\n" +
				"📂 Moving files\n" +
				progressBar(p.Done, p.Total) + "\n" +
				fmt.Sprintf("%d/%d episodes", p.Done, p.Total))
		})
	case len(episodeFiles) == 1:
		// SINGLE EPISODE — never treated as a season pack
		live.Update("🗂 Organizing...\n\n🔍 Analyzing content\n→ Single episode detected")
		res, err = org.OrganizeSingleEpisode(episodeFiles[0], nil)
		// remaining non-episode files -> movies
		for _, v := range append(otherVideos, otherFiles...) {
			org.MoveToMovies(v)
		}
	default:
		// NO EPISODES — plain archive: keep in archives/
		dest := filepath.Join(org.Paths.Archives, strings.TrimSuffix(displayName, filepath.Ext(displayName)))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			live.Update("🗂 Organizing...\n\n⚠️ Failed to prepare archive folder")
			return
		}
		if err := os.Rename(stageDir, uniqueDest(dest)); err != nil {
			live.Update("🗂 Organizing...\n\n⚠️ Failed to move archive content")
			return
		}
		res = &organize.Result{Category: organize.CatArchives}
	}
	if err != nil {
		logger.Error("organize after extract failed: %v", err)
		live.Update("🗂 Organizing...\n\n⚠️ Organize failed: " + err.Error())
		return
	}

	// delete original archive (config) or move to archives/
	if cfg.DeleteArchive {
		if err := os.Remove(srcPath); err != nil {
			logger.Error("failed to delete archive %s: %v", srcPath, err)
		}
	} else {
		org.MoveToArchives(srcPath)
	}

	res.SizeBytes = dirTreeSize(stageDir)
	res.Duration = time.Since(start)
	sendArchiveSummary(bot, chatID, displayName, res)
}

// uniqueDest appends _1, _2 ... when the destination dir exists.
func uniqueDest(dest string) string {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s_%d", dest, i)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

func dirTreeSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// sendArchiveSummary posts the final archive summary (plan style).
func sendArchiveSummary(bot *tgBotApi.BotAPI, chatID int64, archiveName string, res *organize.Result) {
	if res == nil {
		return
	}
	tree := organize.Tree(res.Moved, libraryRoots())
	failed := ""
	if len(res.Failed) > 0 {
		failed = fmt.Sprintf("• %d failed\n", len(res.Failed))
	}
	movedN := len(res.Moved)
	what := "files moved"
	if res.Category == organize.CatSeries || res.Category == organize.CatAnime {
		what = "episodes moved"
	}
	text := fmt.Sprintf(
		"✅ All done!\n\n"+
			"📦 Archive processed\n%s\n\n"+
			"🗂 Result\n"+
			"• %d %s\n"+
			"%s\n"+
			"%s\n\n"+
			"📦 Size: %s\n"+
			"⏱ Time taken: %s",
		archiveName,
		movedN,
		what,
		failed,
		tree,
		typeTrans.Byte2Readable(float64(res.SizeBytes)),
		formatDuration(res.Duration),
	)
	sendPlain(bot, chatID, text)
}
