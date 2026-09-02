// Recovery logic for interrupted operations. If the bot (or the whole
// machine) dies mid-extraction, the staging directory leaks and the archive
// may never get processed. On startup we clean the leftovers and resume.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/internal/extract"
	"DownloadBot/tool/input"
	logger "DownloadBot/tool/zap"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// stagePrefix marks extraction staging directories.
const stagePrefix = ".proxbot_extract_"

// recoverCleanupStages runs once at startup, BEFORE aria2 connects so no
// download event can race the sweep. Every stage dir present is an orphan
// from a crashed run - the process that created it is gone.
func recoverCleanupStages() {
	cleanupStaleStages()
}

// recoverResumeDownloads runs after aria2 is connected and re-runs the
// archive pipeline for downloads that completed but were never organized
// (crash between "download complete" and "moved into the library").
func recoverResumeDownloads() {
	resumeUnprocessedArchives()
}

// cleanupStaleStages removes orphaned staging directories from crashed runs.
// At startup every stage dir is an orphan by definition - the process that
// created it is gone (cleanup runs before aria2 can deliver new events).
func cleanupStaleStages() {
	roots := stageRoots()
	// old versions staged in /tmp; tmpfs survives bot restarts
	if tmp, err := filepath.Glob(filepath.Join(os.TempDir(), "proxbot_extract_*")); err == nil {
		roots = append(roots, tmp...)
	}
	for _, root := range roots {
		// glob results are files to remove directly, dirs need a listing
		if strings.Contains(root, stagePrefix) {
			removeStageDir(root)
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), stagePrefix) {
				removeStageDir(filepath.Join(root, e.Name()))
			}
		}
	}
}

func removeStageDir(path string) {
	logger.Info("recovery: removing stale extraction stage %s", path)
	if err := os.RemoveAll(path); err != nil {
		logger.Error("recovery: failed to remove %s: %v", path, err)
	}
}

// stageRoots lists directories where staging dirs can appear: the download
// folder plus every configured library root.
func stageRoots() []string {
	seen := map[string]bool{}
	var roots []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			roots = append(roots, p)
		}
	}
	add(config.GetDownloadFolder())
	cfg := config.GetOrganizeConfig()
	add(cfg.Movies)
	add(cfg.Series)
	add(cfg.Anime)
	add(cfg.Music)
	add(cfg.Documents)
	add(cfg.Archives)
	add(cfg.Others)
	add(cfg.YouTube)
	add(cfg.Services)
	return roots
}

// resumeUnprocessedArchives finds archives in the download folder that aria2
// itself completed but that never got organized (crash right after the
// download finished), and restarts the archive pipeline for them.
func resumeUnprocessedArchives() {
	dl := config.GetDownloadFolder()
	if dl == "" {
		return
	}

	// only archives aria2 knows it completed - never touch user files
	completed := map[string]bool{}
	for _, t := range input.ToolApp.Aria2.StoppedTasks() {
		if t.Status != "complete" {
			continue
		}
		for _, f := range t.Files {
			p := f.Path
			if p == "" || filepath.Dir(p) != dl || !extract.IsArchivePath(p) {
				continue
			}
			completed[p] = true
		}
	}

	resumed := 0
	for path := range completed {
		// file gone = already organized in a previous run
		if _, err := os.Stat(path); err != nil {
			continue
		}
		// .aria2 control file present = still downloading
		if _, err := os.Stat(path+".aria2"); err == nil {
			continue
		}
		resumed++
		name := filepath.Base(path)
		logger.Info("recovery: resuming unprocessed archive %s", path)
		go func(src, displayName string) {
			org := buildOrganizer()
			runArchivePipeline(activeBot, organizeChatID(), org, src, displayName)
		}(path, name)
	}
	if resumed > 0 {
		sendPlain(activeBot, organizeChatID(),
			fmt.Sprintf("🔄 Recovery\n\nBot restarted mid-organize\n→ Resuming %d archive(s) from the previous run", resumed))
	}
}
