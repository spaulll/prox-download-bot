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
// about these fetches).
type localFetch struct {
	gid    string
	name   string
	userID int64
	chatID int64
	total  int64
	start  time.Time
}

var (
	localActiveMu sync.Mutex
	localActive   = map[string]*localFetch{}
)

// registerLocalFetch announces a fetch; unregisterLocalFetch withdraws it.
func registerLocalFetch(gid, name string, userID, chatID int64) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	localActive[gid] = &localFetch{gid: gid, name: name, userID: userID, chatID: chatID, start: time.Now()}
}

func setLocalFetchTotal(gid string, total int64) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	if f, ok := localActive[gid]; ok && total > 0 {
		f.total = total
	}
}

func unregisterLocalFetch(gid string) {
	localActiveMu.Lock()
	defer localActiveMu.Unlock()
	delete(localActive, gid)
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
		fmt.Fprintf(&b, "*Filename:* `%s`\n📥 Fetching from Telegram...\n*Size:* %s *Elapsed:* %s\n*GID:* `%s`",
			name, size, formatDuration(time.Since(f.start)), f.gid)
	}
	return b.String()
}

// spinnerFrames animates the fetching indicator (byte progress is unknowable
// in local mode, but elapsed time proves the fetch is alive).
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
func fetchTelegramLocalFile(bot *tgBotApi.BotAPI, gid, fileID string, onTick func()) (string, int64, error) {
	// learn the total size first (present even before the fetch completes)
	var total int64
	if f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID}); err == nil && f.FileSize > 0 {
		total = int64(f.FileSize)
		setLocalFetchTotal(gid, total)
	}
	deadline := time.Now().Add(localFetchTimeout(total))
	var lastSize int64 = -1
	stable := 0
	for time.Now().Before(deadline) {
		f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID})
		if err != nil {
			logger.Debug("local getFile failed: %v", err)
			onTick()
			time.Sleep(time.Second)
			continue
		}
		if f.FileSize > 0 {
			total = int64(f.FileSize)
			setLocalFetchTotal(gid, total)
		}
		if f.FilePath == "" {
			onTick()
			time.Sleep(time.Second)
			continue
		}
		st, err := os.Stat(f.FilePath)
		if err != nil || st.IsDir() {
			onTick()
			time.Sleep(time.Second)
			continue
		}
		sz := st.Size()
		onTick()
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
		time.Sleep(time.Second)
	}
	return "", total, fmt.Errorf("timed out waiting for Telegram download")
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
// Runs asynchronously (call in a goroutine).
func startTelegramLocalDownload(bot *tgBotApi.BotAPI, chats []int64, gid, displayName, fileID string, userID, chatID int64) {
	chats = dedupChats(chats)
	registerLocalFetch(gid, displayName, userID, chatID)
	defer unregisterLocalFetch(gid)
	live := NewDualProgressMsg(bot, chats, "⬇️ Downloading\n"+displayName)
	start := time.Now()
	tick := 0
	var lastText string
	update := func() {
		frame := spinnerFrames[tick%len(spinnerFrames)]
		tick++
		localActiveMu.Lock()
		var total int64
		if f, ok := localActive[gid]; ok {
			total = f.total
		}
		localActiveMu.Unlock()
		size := "…"
		if total > 0 {
			size = typeTrans.Byte2Readable(float64(total))
		}
		text := "⬇️ Downloading\n\n" +
			"📥 Fetching from Telegram " + frame + "\n" +
			"📦 Size: " + size + "\n" +
			"⏱ Elapsed: " + formatDuration(time.Since(start)) + "\n\n" +
			displayName
		if text != lastText {
			live.Update(text)
			lastText = text
		}
	}
	src, total, err := fetchTelegramLocalFile(bot, gid, fileID, update)
	if err != nil {
		logger.Error("telegram local fetch failed for %s: %v", displayName, err)
		taskStore.SetStatus(gid, "failed")
		if live != nil {
			live.Update("⚠️ Download failed\n" + err.Error() + "\n" + displayName)
		}
		return
	}
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
