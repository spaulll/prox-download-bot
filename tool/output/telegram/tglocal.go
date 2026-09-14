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

// attributeGrow maps in-progress server bytes to this fetch by watching
// growth between two consecutive scans: a file counts only if it was born or
// grew since the previous scan, holds (0..total] bytes, and is the ONLY such
// file. Multiple growers = ambiguous = unknown (concurrent fetches never see
// each other's bytes; the worst case is the honest spinner, never wrong
// numbers). Pre-existing/stalled files are ignored, so this works whether or
// not the server started the temp file before this fetch began.
func attributeGrow(prev, cur map[string]tempFileStat, total int64) (int64, bool) {
	if cur == nil {
		return 0, false
	}
	var found int64 = -1
	for name, c := range cur {
		if c.size <= 0 {
			continue
		}
		if total > 0 && c.size > total {
			continue
		}
		var grew bool
		if p, ok := prev[name]; ok {
			grew = c.size > p.size
		} else {
			grew = true // born since last scan
		}
		if !grew {
			continue
		}
		if found >= 0 {
			return 0, false // ambiguous: two moving files
		}
		found = c.size
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
// downloading fileID, driving onTick for live progress while waiting.
//
// IMPORTANT: in --local mode getFile BLOCKS until the server has downloaded
// the whole file (it stores it on disk, then returns the path). So getFile
// runs in a background goroutine and completes it; meanwhile we watch the
// server's */temp/ store for byte-level progress and tick the bar.
func fetchTelegramLocalFile(ctx context.Context, bot *tgBotApi.BotAPI, gid, fileID string, knownTotal int64, onTick func()) (string, int64, error) {
	total := knownTotal
	type done struct {
		path string
		err  error
	}
	doneCh := make(chan done, 1)
	go func() {
		f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID})
		if err != nil {
			doneCh <- done{err: err}
			return
		}
		if f.FileSize > 0 {
			total = int64(f.FileSize)
			setLocalFetchTotal(gid, total)
		}
		doneCh <- done{path: f.FilePath}
	}()

	deadline := time.Now().Add(localFetchTimeout(total))
	loopStart := time.Now()
	prevScan := scanTempDir()
	if len(prevScan) == 0 {
		prevScan = nil
	}
	var lastSize int64 = -1
	var lastAt time.Time
	rate := 0.0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", total, fmt.Errorf("download cancelled")
		case d := <-doneCh:
			if d.err != nil {
				return "", total, d.err
			}
			if d.path == "" {
				return "", total, fmt.Errorf("no file path returned")
			}
			return d.path, total, nil
		default:
		}
		curScan := scanTempDir()
		// attribute server-side temp bytes to this fetch when unambiguous
		if downloaded, ok := attributeGrow(prevScan, curScan, total); ok {
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
		prevScan = curScan
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
// API server, then runs the normal organize pipeline. Runs asynchronously
// (call in a goroutine). fileSize is the size reported by the message.
//
// Progress is shown via the same Downloading list as aria2 tasks (one live
// message, auto-opened in every relevant chat), so there is no separate
// progress message to keep in sync. Registry fields (downloaded/rate) feed
// that list; no timer loop here.
func startTelegramLocalDownload(bot *tgBotApi.BotAPI, chats []int64, gid, displayName, fileID string, userID, chatID int64, fileSize int) {
	chats = dedupChats(chats)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerLocalFetch(gid, displayName, userID, chatID, int64(fileSize), cancel, nil)
	defer unregisterLocalFetch(gid)
	// open the Downloading list in every relevant chat (replaces previous
	// views per chat); delete-button flag collisions are handled by
	// startActiveRefresh. Keeps exactly one live progress message.
	for _, chat := range dedupChats(chats) {
		startActiveRefresh(chat, 0)
		time.Sleep(150 * time.Millisecond)
	}
	src, total, err := fetchTelegramLocalFile(ctx, bot, gid, fileID, int64(fileSize), func() {})
	if err != nil {
		logger.Error("telegram local fetch failed for %s: %v", displayName, err)
		if err.Error() == "download cancelled" {
			return
		}
		taskStore.SetStatus(gid, "failed")
		for _, chat := range chats {
			sendPlain(bot, chat, "⚠️ Download failed\n"+err.Error()+"\n"+displayName)
		}
		return
	}
	if _, ok := taskStore.Get(gid); !ok {
		_ = os.Remove(src)
		return
	}
	taskStore.SetSize(gid, total)
	dest := uniqueFilePath(filepath.Join(config.GetDownloadFolder(), safeOutName(displayName)))
	if err := linkOrCopy(src, dest); err != nil {
		logger.Error("telegram local stage failed for %s: %v", displayName, err)
		taskStore.SetStatus(gid, "failed")
		for _, chat := range chats {
			sendPlain(bot, chat, "⚠️ Download failed\n"+err.Error()+"\n"+displayName)
		}
		return
	}
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		logger.Error("telegram server copy cleanup failed for %s: %v", src, err)
	} else {
		logger.Info("telegram server copy cleaned: %s", src)
	}
	logger.Info("telegram local fetch completed: %s (%d bytes)", displayName, total)
	// drop the fetch row from the Downloading list before organizing takes
	// over (the deferred unregister would otherwise keep it until organize
	// finishes)
	unregisterLocalFetch(gid)
	runOrganizeDual(bot, chats, gid, dest, displayName, false)
}
