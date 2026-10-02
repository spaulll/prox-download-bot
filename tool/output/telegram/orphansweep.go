package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/tool/input"
	logger "DownloadBot/tool/zap"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// orphanSweepInterval is how often the download folder is scanned.
	orphanSweepInterval = time.Hour
	// orphanGracePeriod shields fresh entries: task registration and aria2
	// session restore can lag behind file creation.
	orphanGracePeriod = 30 * time.Minute
	// orphanStartupDelay lets aria2 restore its session (and recovery run)
	// before the first sweep.
	orphanStartupDelay = 2 * time.Minute
)

// startOrphanSweeper launches the periodic orphan sweep (log-only, no
// Telegram spam). Safe to call once at startup.
func startOrphanSweeper() {
	go func() {
		time.Sleep(orphanStartupDelay)
		sweepOrphans()
		ticker := time.NewTicker(orphanSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			sweepOrphans()
		}
	}()
}

// sweepOrphans deletes stale download debris. Aborts (fail-closed) when aria2
// status is unavailable rather than risk live downloads.
func sweepOrphans() {
	dl := config.GetDownloadFolder()
	if dl == "" {
		return
	}
	referenced, ok := input.ToolApp.Aria2.ReferencedTops()
	if !ok {
		logger.Error("orphan sweep aborted: aria2 status unavailable")
		return
	}
	for _, d := range sweepOrphansIn(dl, referenced, orphanGracePeriod) {
		logger.Info("orphan sweep deleted %s", d)
	}
}

// sweepOrphansIn scans dir (non-recursive) and deletes stale debris,
// returning what it removed. Only entries anchored by an aria2 control
// file (*.aria2) are ever candidates:
//   - stale "base + base.aria2" pairs with no referencing task (interrupted
//     downloads, error leftovers),
//   - stale lone ".aria2" files whose base is gone (organize moved the data
//     file via rename and left the control file behind).
//
// Never touched: directories, lone data files without a control file
// (completed downloads pending recovery have none), fresh entries within
// minAge, and anything a live task references.
func sweepOrphansIn(dir string, referenced map[string]bool, minAge time.Duration) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Error("orphan sweep read failed for %s: %v", dir, err)
		return nil
	}
	now := time.Now()
	stale := func(info os.FileInfo) bool { return now.Sub(info.ModTime()) > minAge }
	var deleted []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".aria2") {
			continue
		}
		base := strings.TrimSuffix(name, ".aria2")
		if base == "" {
			continue
		}
		if referenced[base] {
			logger.Debug("orphan sweep keep %s: referenced by live task", name)
			continue
		}
		ctrlInfo, err := e.Info()
		if err != nil {
			continue
		}
		if !stale(ctrlInfo) {
			logger.Debug("orphan sweep keep %s: within grace period", name)
			continue
		}
		basePath := filepath.Join(dir, base)
		baseInfo, err := os.Stat(basePath)
		if err == nil {
			if baseInfo.IsDir() {
				logger.Debug("orphan sweep keep %s: base is a directory", name)
				continue
			}
			if !stale(baseInfo) {
				logger.Debug("orphan sweep keep %s: base within grace period", name)
				continue
			}
			if err := os.Remove(basePath); err != nil {
				logger.Error("orphan sweep failed to delete %s: %v", basePath, err)
				continue
			}
			deleted = append(deleted, basePath)
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			logger.Error("orphan sweep failed to delete %s: %v", name, err)
			// base already removed: still report it
			continue
		}
		deleted = append(deleted, filepath.Join(dir, name))
	}
	return deleted
}
