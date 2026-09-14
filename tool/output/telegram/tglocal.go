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
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
// downloading fileID, reporting (downloaded, total) bytes along the way.
// Returns the absolute server-side path and the total size.
func fetchTelegramLocalFile(bot *tgBotApi.BotAPI, fileID string, onProgress func(downloaded, total int64)) (string, int64, error) {
	// learn the total size first (present even before the fetch completes)
	var total int64
	if f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID}); err == nil && f.FileSize > 0 {
		total = int64(f.FileSize)
	}
	deadline := time.Now().Add(localFetchTimeout(total))
	var lastSize int64 = -1
	stable := 0
	for time.Now().Before(deadline) {
		f, err := bot.GetFile(tgBotApi.FileConfig{FileID: fileID})
		if err != nil {
			logger.Debug("local getFile failed: %v", err)
			time.Sleep(time.Second)
			continue
		}
		if f.FileSize > 0 {
			total = int64(f.FileSize)
		}
		if f.FilePath == "" {
			onProgress(0, total)
			time.Sleep(time.Second)
			continue
		}
		st, err := os.Stat(f.FilePath)
		if err != nil || st.IsDir() {
			time.Sleep(time.Second)
			continue
		}
		sz := st.Size()
		onProgress(sz, total)
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
func startTelegramLocalDownload(bot *tgBotApi.BotAPI, chats []int64, gid, displayName, fileID string) {
	chats = dedupChats(chats)
	live := NewDualProgressMsg(bot, chats, "⬇️ Downloading\n"+taskProgressBar(0)+"\n"+displayName)
	start := time.Now()
	var lastText string
	src, total, err := fetchTelegramLocalFile(bot, fileID, func(d, t int64) {
		lines := []string{"⬇️ Downloading", taskProgressBar(percentOf(d, t))}
		if t > 0 {
			lines = append(lines, fmt.Sprintf("Downloaded: %s of %s",
				typeTrans.Byte2Readable(float64(d)),
				typeTrans.Byte2Readable(float64(t))))
		} else if d > 0 {
			lines = append(lines, "Downloaded: "+typeTrans.Byte2Readable(float64(d)))
		}
		if elapsed := time.Since(start); elapsed >= 2*time.Second && d > 0 {
			rate := float64(d) / elapsed.Seconds()
			lines = append(lines, "Speed: "+typeTrans.Byte2Readable(rate)+"/s")
			if eta := remainingETA(start, d, t); eta > 0 {
				lines = append(lines, "ETA: "+formatDuration(eta))
			}
		}
		text := ""
		for i, l := range lines {
			if i > 0 {
				text += "\n"
			}
			text += l
		}
		text += "\n" + displayName
		if text != lastText {
			live.Update(text)
			lastText = text
		}
	})
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
	logger.Info("telegram local fetch completed: %s (%d bytes)", displayName, total)
	live.Delete()
	runOrganizeDual(bot, chats, gid, dest, displayName, false)
}

// percentOf renders 0-100 safely for progress display.
func percentOf(done, total int64) float64 {
	if total <= 0 || done < 0 {
		return 0
	}
	p := float64(done) * 100.0 / float64(total)
	if p > 100 {
		return 100
	}
	return p
}
