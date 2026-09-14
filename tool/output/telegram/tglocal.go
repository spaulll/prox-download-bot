// Local Bot API file fetching. In --local mode the API server has no /file/
// HTTP route at all: getFile returns an absolute on-disk path and the file
// must be consumed from the shared filesystem. So Telegram files are fetched
// by polling getFile until the server finishes downloading, then hardlinked
// (or copied) into the download folder and handed to the normal organize
// pipeline - no aria2 HTTP fetch involved.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// localFetch tracks one in-progress server-side Telegram fetch so the
// "Downloading" list can show it next to aria2 tasks (which know nothing
// about these fetches). Cancel stops the wait; live lets removal drop the
// progress messages immediately. downloaded < 0 means unknown byte progress;
// rate <= 0 means unknown speed.
type localFetch struct {
	gid        string
	name       string
	userID     int64
	chatID     int64
	total      int64
	downloaded int64
	rate       float64
	start      time.Time
	cancel     context.CancelFunc
	live       *DualProgressMsg
}

var (
	localActiveMu sync.Mutex
	localActive   = map[string]*localFetch{}
)

// registerLocalFetch announces a fetch; unregisterLocalFetch withdraws it.
// downloaded starts unknown (-1) until temp watching attributes bytes.
func registerLocalFetch(gid, name string, userID, chatID, total int64, cancel context.CancelFunc, live *DualProgressMsg) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	localActive[gid] = &localFetch{gid: gid, name: name, userID: userID, chatID: chatID, total: total, downloaded: -1, start: time.Now(), cancel: cancel, live: live}
}

func setLocalFetchTotal(gid string, total int64) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	if f, ok := localActive[gid]; ok && total > 0 {
		f.total = total
	}
}

// setLocalFetchProgress records attributed bytes and smoothed rate.
// downloaded < 0 clears back to unknown.
func setLocalFetchProgress(gid string, downloaded int64, rate float64) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	if f, ok := localActive[gid]; ok {
		f.downloaded = downloaded
		f.rate = rate
	}
}

func unregisterLocalFetch(gid string) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	delete(localActive, gid)
}

// cancelTelegramFetch aborts an in-progress fetch: the wait loop exits,
// progress messages are dropped and the task record is forgotten. Reports
// whether a live fetch was actually cancelled (false = already gone, the
// caller should forget a history entry instead).
func cancelTelegramFetch(gid string) bool {
	localActiveMu.Lock()
	f, ok := localActive[gid]
	if !ok {
		localActiveMu.Unlock()
		return false
	}
	cancel, live := f.cancel, f.live
	localActiveMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if live != nil {
		live.Delete()
	}
	unregisterLocalFetch(gid)
	taskStore.Remove(gid)
	return true
}

// formatLocalActive renders visible in-progress fetches for the "Downloading"
// list. Admins see all; regular users only their own (owner or origin chat).
func formatLocalActive(requester int64) string {
	localActiveMu.Lock()
	fetches := make([]*localFetch, 0, len(localActive))
	for _, f := range localActive {
		fetches = append(fetches, f)
	}
	localActiveMu.Unlock()
	if len(fetches) == 0 {
		return ""
	}
	admin := isAdminID(requester)
	visible := fetches[:0]
	for _, f := range fetches {
		if admin || f.userID == requester || f.chatID == requester {
			visible = append(visible, f)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].gid < visible[j].gid })
	var b strings.Builder
	for i, f := range visible {
		if i > 0 {
			b.WriteString("\n\n")
		}
		size := "…"
		if f.total > 0 {
			size = typeTrans.Byte2Readable(float64(f.total))
		}
		// NOTE: no escapeMarkdown here - the name sits inside backticks
		// (a literal code span) where escapes would show up raw; only
		// backticks themselves must go.
		name := strings.ReplaceAll(f.name, "`", "")
		head := fmt.Sprintf("*Filename:* `%s`\n", name)
		if f.downloaded >= 0 && f.total > 0 {
			// real byte progress attributed from the server temp store
			pct := float64(f.downloaded) * 100.0 / float64(f.total)
			if pct > 100 {
				pct = 100
			}
			head += taskProgressBar(pct) + "\n" +
				fmt.Sprintf("*Downloaded:* %s *of* %s",
					typeTrans.Byte2Readable(float64(f.downloaded)), size)
			if f.rate > 0 {
				head += fmt.Sprintf("\n*Speed:* %s/s", typeTrans.Byte2Readable(f.rate))
				left := f.total - f.downloaded
				if left > 0 {
					head += fmt.Sprintf(" *ETA:* %s", formatDuration(time.Duration(float64(left)/f.rate)*time.Second))
				}
			}
		} else {
			head += "📥 Fetching from Telegram...\n" +
				fmt.Sprintf("*Size:* %s *Elapsed:* %s", size, formatDuration(time.Since(f.start)))
		}
		head += fmt.Sprintf("\n*GID:* `%s`", f.gid)
		b.WriteString(head)
	}
	return b.String()
}

// tempFileStat is a size/mtime snapshot of one server temp file.
type tempFileStat struct {
	size  int64
	mtime time.Time
}

// scanTempDir snapshots every */temp/ file anywhere under the API data dir
// (the server creates one temp/ per bot subdirectory; nesting varies by
// version so scan recursively instead of assuming a fixed location).
func scanTempDir() map[string]tempFileStat {
	root := config.GetTelegramApiDir()
	if root == "" {
		return nil
	}
	out := make(map[string]tempFileStat, 0)
	var scan func(dir string)
	scan = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if e.Name() == "temp" {
					tfEntries, err := os.ReadDir(p)
					if err != nil {
						continue
					}
					for _, tf := range tfEntries {
						if tf.IsDir() {
							continue
						}
						info, err := tf.Info()
						if err != nil {
							continue
						}
						out[tf.Name()] = tempFileStat{size: info.Size(), mtime: info.ModTime()}
					}
				} else {
					scan(p)
				}
			}
		}
	}
	scan(root)
	return out
}

// attributeTemp maps in-progress server bytes to one fetch. Candidates are
// temp files unseen at fetch start, modified since, holding (0, total]
// bytes. Exactly one candidate = attributable; anything else = unknown.
// This strict rule keeps concurrent fetches from ever showing each other's
// bytes (worst case is the honest spinner, never wrong numbers).
func attributeTemp(baseline, current map[string]tempFileStat, start time.Time, total int64) (int64, bool) {
	if current == nil {
		return 0, false
	}
	var found int64 = -1
	for name, cur := range current {
		if _, ok := baseline[name]; ok {
			continue // predates our fetch: not provably ours
		}
		if cur.size <= 0 {
			continue
		}
		if total > 0 && cur.size > total {
			continue
		}
		if cur.mtime.Before(start.Add(-2 * time.Second)) {
			continue
		}
		if found >= 0 {
			return 0, false // ambiguous: two candidates
		}
		found = cur.size
	}
	if found < 0 {
		return 0, false
	}
	return found, true
}

// spinnerFrames animates the fetching indicator while byte progress is
// unattributed (no fetch yet visible, or several concurrent fetches).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// localFetchTimeout bounds the server-side fetch wait. Assumes at least
// ~50KB/s, clamped to [10min, 3h] (30min when the size is unknown).
func localFetchTimeout(total int64) time.Duration {
	if total <= 0 {
		return 30 * time.Minute
	}
	secs := total / (50 * 1024)
	if secs < 600 {
		secs = 600
	}
	if secs > 3*3600 {
		secs = 3 * 3600
	}
	return time.Duration(secs) * time.Second
}

// fetchTelegramLocalFile waits for the local Bot API server to finish
// downloading fileID, ticking roughly once a second so the caller can
// animate liveness. Returns the absolute server-side path and total size.
//
// NOTE: getFile only reveals file_path once the fetch COMPLETES, so true
// byte progress is unknowable here - callers must not render percentages.
func fetchTelegramLocalFile(ctx context.Context, bot *tgBotApi.BotAPI, gid, fileID string, onTick func()) (string, int64, error) {
	// learn the total size first (present even before the fetch completes)
	var total int64
	if f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID}); err == nil && f.FileSize > 0 {
		total = int64(f.FileSize)
		setLocalFetchTotal(gid, total)
	}
	deadline := time.Now().Add(localFetchTimeout(total))
	loopStart := time.Now()
	baseline := scanTempDir()
	var lastSize int64 = -1
	var lastAt time.Time
	rate := 0.0
	stable := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", total, fmt.Errorf("download cancelled")
		default:
		}
		// attribute server-side temp bytes to this fetch when unambiguous
		if downloaded, ok := attributeTemp(baseline, scanTempDir(), loopStart, total); ok {
			now := time.Now()
			if lastAt.IsZero() {
				lastAt = loopStart
			}
			if dt := now.Sub(lastAt).Seconds(); dt > 0 && downloaded >= lastSize && lastSize >= 0 {
				inst := float64(downloaded-lastSize) / dt
				if rate <= 0 {
					rate = inst
				} else {
					rate = 0.7*rate + 0.3*inst
				}
			}
			lastSize, lastAt = downloaded, now
			setLocalFetchProgress(gid, downloaded, rate)
			onTick()
		} else {
			setLocalFetchProgress(gid, -1, 0)
			onTick()
		}
		f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID})
		if err != nil {
			logger.Debug("local getFile failed: %v", err)
			if !sleepCtx(ctx, time.Second) {
				return "", total, fmt.Errorf("download cancelled")
			}
			continue
		}
		if f.FileSize > 0 {
			total = int64(f.FileSize)
			setLocalFetchTotal(gid, total)
		}
		if f.FilePath == "" {
			if !sleepCtx(ctx, time.Second) {
				return "", total, fmt.Errorf("download cancelled")
			}
			continue
		}
		st, err := os.Stat(f.FilePath)
		if err != nil || st.IsDir() {
			if !sleepCtx(ctx, time.Second) {
				return "", total, fmt.Errorf("download cancelled")
			}
			continue
		}
		sz := st.Size()
		if total > 0 && sz >= total {
			return f.FilePath, total, nil
		}
		if total == 0 && sz > 0 {
			if sz == lastSize {
				if stable++; stable >= 3 {
					return f.FilePath, sz, nil
				}
			} else {
				stable = 0
			}
			lastSize = sz
		}
		if !sleepCtx(ctx, time.Second) {
			return "", total, fmt.Errorf("download cancelled")
		}
	}
	return "", total, fmt.Errorf("timed out waiting for Telegram download")
}

// sleepCtx sleeps d, reporting false early when ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// linkOrCopy places src at dst via hardlink when possible (same filesystem:
// instant, no extra space), falling back to a full copy.
func linkOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// startTelegramLocalDownload fetches a Telegram file through the local Bot
// API server with live progress, then runs the normal organize pipeline.
// Runs asynchronously (call in a goroutine). fileSize is the size reported
// by the Telegram message itself (getFile may not know it yet).
func startTelegramLocalDownload(bot *tgBotApi.BotAPI, chats []int64, gid, displayName, fileID string, userID, chatID int64, fileSize int) {
	chats = dedupChats(chats)
	ctx, cancel := context.WithCancel(context.Background())
	live := NewDualProgressMsg(bot, chats, "⬇️ Downloading\n"+displayName)
	registerLocalFetch(gid, displayName, userID, chatID, int64(fileSize), cancel, live)
	defer func() {
		live.Delete()
		unregisterLocalFetch(gid)
	}()
	start := time.Now()
	tick := 0
	var lastText string
	update := func() {
		localActiveMu.Lock()
		f, ok := localActive[gid]
		var total, downloaded int64
		var rate float64
		if ok {
			total, downloaded, rate = f.total, f.downloaded, f.rate
		}
		localActiveMu.Unlock()
		size := "…"
		if total > 0 {
			size = typeTrans.Byte2Readable(float64(total))
		}
		var text string
		if ok && downloaded >= 0 && total > 0 {
			// attributed server bytes: real bar, downloaded, speed, ETA
			pct := float64(downloaded) * 100.0 / float64(total)
			if pct > 100 {
				pct = 100
			}
			text = "⬇️ Downloading\n" +
				taskProgressBar(pct) + "\n" +
				fmt.Sprintf("Downloaded: %s of %s",
					typeTrans.Byte2Readable(float64(downloaded)), size)
			if rate > 0 {
				text += "\nSpeed: " + typeTrans.Byte2Readable(rate) + "/s"
				if left := total - downloaded; left > 0 {
					text += "\nETA: " + formatDuration(time.Duration(float64(left)/rate)*time.Second)
				}
			}
			text += "\n" + displayName
		} else {
			frame := spinnerFrames[tick%len(spinnerFrames)]
			tick++
			text = "⬇️ Downloading\n\n" +
				"📥 Fetching from Telegram " + frame + "\n" +
				"📦 Size: " + size + "\n" +
				"⏱ Elapsed: " + formatDuration(time.Since(start)) + "\n\n" +
				displayName
		}
		if text != lastText {
			live.Update(text)
			lastText = text
		}
	}
	src, total, err := fetchTelegramLocalFile(ctx, bot, gid, fileID, update)
	if err != nil {
		logger.Error("telegram local fetch failed for %s: %v", displayName, err)
		if err.Error() == "download cancelled" {
			// removal already handled by the canceller; just go quiet
			return
		}
		taskStore.SetStatus(gid, "failed")
		if live != nil {
			live.Update("⚠️ Download failed\n" + err.Error() + "\n" + displayName)
		}
		return
	}
	// cancelled while staging/organizing was queued: discard instead of
	// delivering a surprise result
	if _, ok := taskStore.Get(gid); !ok {
		_ = os.Remove(src)
		return
	}
	taskStore.SetSize(gid, total)
	dest := uniqueFilePath(filepath.Join(config.GetDownloadFolder(), safeOutName(displayName)))
	if err := linkOrCopy(src, dest); err != nil {
		logger.Error("telegram local stage failed for %s: %v", displayName, err)
		taskStore.SetStatus(gid, "failed")
		if live != nil {
			live.Update("⚠️ Download failed\n" + err.Error() + "\n" + displayName)
		}
		return
	}
	// the staged copy now owns the data (hardlink shares the inode, plain
	// copy duplicates it): drop the server-side cache copy so 2 GB files do
	// not linger twice on disk. The server re-fetches on demand if needed.
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		logger.Error("telegram server copy cleanup failed for %s: %v", src, err)
	} else {
		logger.Info("telegram server copy cleaned: %s", src)
	}
	logger.Info("telegram local fetch completed: %s (%d bytes)", displayName, total)
	live.Delete()
	runOrganizeDual(bot, chats, gid, dest, displayName, false)
}
