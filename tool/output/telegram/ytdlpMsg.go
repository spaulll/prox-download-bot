package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/internal/ytdlp"
	logger "DownloadBot/tool/zap"
	"DownloadBot/tool/typeTrans"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// buildYtdlpDownloader builds the yt-dlp downloader from config.
func buildYtdlpDownloader() *ytdlp.Downloader {
	cfg := config.GetOrganizeConfig()
	return &ytdlp.Downloader{
		Cfg: ytdlp.Config{
			BinPath:        cfg.YtdlpPath,
			Quality:        cfg.YtdlpQuality,
			BaseYouTube:    cfg.YouTube,
			BaseServices:   cfg.Services,
			Cookies:        cfg.YtdlpCookies,
			Proxy:          cfg.YtdlpProxy,
			EmbedThumbnail: cfg.YtdlpEmbed,
			EmbedMetadata:  cfg.YtdlpEmbed,
		},
		Logf: func(f string, v ...interface{}) { logger.Info(f, v...) },
	}
}

// startYtdlpDownload runs the yt-dlp pipeline with a live Telegram progress
// message. Runs asynchronously (call in a goroutine).
func startYtdlpDownload(bot *tgBotApi.BotAPI, chatID int64, rawURL string) {
	dl := buildYtdlpDownloader()

	// announce
	live := NewOrganizeProgressMsg(bot, chatID, "⬇️ Downloading\n▱▱▱▱▱▱▱▱▱▱ 0%\n"+rawURL)
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
			text = "⬇️ Downloading\n" +
				progressBar(int(p.Percent), 100) + "\n" +
				rawURL
		case "processing":
			text = "⬇️ Downloading\n" +
				progressBar(100, 100) + "\n" +
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
	what := res.Service
	if strings.EqualFold(res.Service, "youtube") {
		what = "YouTube"
	}
	text := fmt.Sprintf(
		"✅ Download completed\n\n"+
			"📺 %s\n%s\n"+
			"%s\n"+
			"📦 Size: %s\n"+
			"⏱ Time taken: %s",
		what,
		res.Title,
		fileList,
		typeTrans.Byte2Readable(float64(res.SizeBytes)),
		formatDuration(res.Duration),
	)
	if live != nil {
		live.Update(text)
	} else {
		sendPlain(bot, chatID, text)
	}
	logger.Info("yt-dlp download completed: %s (%d files)", res.Title, len(res.Files))
}
