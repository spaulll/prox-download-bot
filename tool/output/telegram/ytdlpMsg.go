package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/internal/ytdlp"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// buildYtdlpDownloader builds the yt-dlp downloader from config.
func buildYtdlpDownloader() *ytdlp.Downloader {
	cfg := config.GetOrganizeConfig()
	// default library bases live under the download folder when unset
	youTube := cfg.YouTube
	if youTube == "" {
		youTube = filepath.Join(config.GetDownloadFolder(), "YouTube")
	}
	services := cfg.Services
	if services == "" {
		services = filepath.Join(config.GetDownloadFolder(), "Services")
	}
	return &ytdlp.Downloader{
		Cfg: ytdlp.Config{
			BinPath:        cfg.YtdlpPath,
			Quality:        cfg.YtdlpQuality,
			BaseYouTube:    youTube,
			BaseServices:   services,
			Cookies:        cfg.YtdlpCookies,
			Proxy:          cfg.YtdlpProxy,
			EmbedThumbnail: cfg.YtdlpEmbed,
			EmbedMetadata:  cfg.YtdlpEmbed,
		},
		Logf: func(f string, v ...interface{}) { logger.Info(f, v...) },
	}
}

// taskProgressBar renders the 13-segment bar with float percent, matching the
// aria2 task list style: [●●●○○○○○○○○○○] 12.34 %
func taskProgressBar(percent float64) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	const total = 13
	filled := int(percent / (100.0 / float64(total)))
	if percent > 0 && filled == 0 {
		filled = 1
	}
	if percent >= 100 {
		filled = total
	}
	return "[" + strings.Repeat("●", filled) + strings.Repeat("○", total-filled) + "] " +
		strconv.FormatFloat(percent, 'f', 2, 64) + " %"
}

// startYtdlpDownload runs the yt-dlp pipeline with a live Telegram progress
// message. Runs asynchronously (call in a goroutine).
func startYtdlpDownload(bot *tgBotApi.BotAPI, chatID int64, rawURL string) {
	dl := buildYtdlpDownloader()

	// announce
	live := NewOrganizeProgressMsg(bot, chatID, "⬇️ Downloading\n"+taskProgressBar(0)+"\n"+rawURL)
	var lastText string
	var lastUpdate time.Time
	report := func(p ytdlp.Progress) {
		if live == nil || time.Since(lastUpdate) < time.Second {
			return
		}
		lastUpdate = time.Now()
		var text string
		switch p.Stage {
		case "downloading":
			lines := []string{
				"⬇️ Downloading",
				taskProgressBar(p.Percent),
			}
			if p.TotalBytes > 0 {
				lines = append(lines, fmt.Sprintf("Downloaded: %s of %s",
					typeTrans.Byte2Readable(float64(p.DownloadedBytes)),
					typeTrans.Byte2Readable(float64(p.TotalBytes))))
			}
			if p.SpeedBytes > 0 {
				lines = append(lines, "Speed: "+typeTrans.Byte2Readable(float64(p.SpeedBytes))+"/s")
			}
			if p.ETASeconds > 0 {
				lines = append(lines, "ETA: "+formatDuration(time.Duration(p.ETASeconds)*time.Second))
			}
			text = strings.Join(lines, "\n") + "\n" + rawURL
		case "processing":
			text = "⬇️ Downloading\n" +
				taskProgressBar(100) + "\n" +
				strings.SplitN(p.Detail, "]", 2)[0][1:] + "..."
		case "done":
			return
		default:
			return
		}
		if text != lastText {
			live.Update(text)
			lastText = text
		}
	}

	res, err := dl.Download(rawURL, report)
	if err != nil {
		logger.Error("yt-dlp download failed: %v", err)
		if live != nil {
			live.Update("⚠️ Download failed\n" + err.Error() + "\n" + rawURL)
		}
		return
	}

	// final summary (plan style)
	fileList := ""
	for _, f := range res.Files {
		fileList += "• " + filepath.Base(f) + "\n"
	}
	saved := ""
	if len(res.Files) > 0 {
		saved = "📁 Saved to: " + filepath.Dir(res.Files[0]) + "\n"
	}
	what := res.Service
	if strings.EqualFold(res.Service, "youtube") {
		what = "YouTube"
	}
	text := fmt.Sprintf(
		"✅ Download completed\n\n"+
			"📺 %s\n%s\n"+
			"%s\n"+
			"%s"+
			"📦 Size: %s\n"+
			"⏱ Time taken: %s",
		what,
		res.Title,
		fileList,
		saved,
		typeTrans.Byte2Readable(float64(res.SizeBytes)),
		formatDuration(res.Duration),
	)
	if live != nil {
		live.Update(text)
		// the live message became the final summary - keep it, untrack it
		inflightRemove(live.ID())
	} else {
		sendPlain(bot, chatID, text)
	}
	logger.Info("yt-dlp download completed: %s (%d files)", res.Title, len(res.Files))
}
