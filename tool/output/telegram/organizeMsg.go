package telegram

import (
	"bytes"
	"DownloadBot/internal/config"
	"DownloadBot/internal/organize"
	"DownloadBot/tool/input"
	"DownloadBot/tool/input/aria2/rpc"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// OrganizeProgressMsg is a live-updating Telegram message during organizing.
type OrganizeProgressMsg struct {
	bot       *tgBotApi.BotAPI
	chatID    int64
	messageID int
}

// NewOrganizeProgressMsg sends the initial message and returns a handle.
func NewOrganizeProgressMsg(bot *tgBotApi.BotAPI, chatID int64, text string) *OrganizeProgressMsg {
	msg := tgBotApi.NewMessage(chatID, text)
	res, err := bot.Send(msg)
	if err != nil {
		logger.Error("send organize message failed: %v", err)
		return nil
	}
	return &OrganizeProgressMsg{bot: bot, chatID: chatID, messageID: res.MessageID}
}

// Update edits the message text.
func (m *OrganizeProgressMsg) Update(text string) {
	if m == nil {
		return
	}
	edit := tgBotApi.NewEditMessageText(m.chatID, m.messageID, text)
	if _, err := m.bot.Send(edit); err != nil {
		logger.Debug("edit organize message failed: %v", err)
	}
}

// progressBar renders the mandated style: ▰▰▰▰▰▰▰▱▱▱ 70%
func progressBar(done, total int) string {
	if total <= 0 {
		return ""
	}
	pct := done * 100 / total
	const cells = 10
	filled := pct * cells / 100
	if pct > 0 && filled == 0 {
		filled = 1
	}
	if done >= total {
		filled = cells
	}
	return strings.Repeat("▰", filled) + strings.Repeat("▱", cells-filled) + fmt.Sprintf(" %d%%", pct)
}

// anilistPost is the HTTP poster used by the organizer's AniList lookups.
func anilistPost(url, body string, timeout time.Duration) (int, string) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, ""
	}
	return resp.StatusCode, string(data)
}

// buildOrganizer constructs the organizer from config.
func buildOrganizer() *organize.Organizer {
	cfg := config.GetOrganizeConfig()
	return &organize.Organizer{
		Paths: organize.Paths{
			Movies:    cfg.Movies,
			Series:    cfg.Series,
			Anime:     cfg.Anime,
			Music:     cfg.Music,
			Documents: cfg.Documents,
			Archives:  cfg.Archives,
			Others:    cfg.Others,
		},
		AniList: cfg.AniList,
		Post:    anilistPost,
		Logf:    func(format string, v ...interface{}) { logger.Info(format, v...) },
	}
}

// libraryRoots maps display labels to base dirs for tree rendering.
func libraryRoots() map[string]string {
	cfg := config.GetOrganizeConfig()
	return map[string]string{
		"Movies":    cfg.Movies,
		"Series":    cfg.Series,
		"Anime":     cfg.Anime,
		"Music":     cfg.Music,
		"Documents": cfg.Documents,
		"Archives":  cfg.Archives,
		"Others":    cfg.Others,
	}
}

// formatDuration renders "1m 42s" style durations.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%dh %dm %ds", minutes/60, minutes%60, seconds)
}

// organizeChatID returns the admin chat id for bot messages.
func organizeChatID() int64 {
	return typeTrans.Str2Int64(config.GetTelegramUserID())
}

// runOrganize executes the organize pipeline for a completed download and
// drives the live Telegram progress messages. Runs asynchronously.
func runOrganize(bot *tgBotApi.BotAPI, chatID int64, srcPath, displayName string) {
	// plan-style "Download completed" message
	sendPlain(bot, chatID, fmt.Sprintf("✅ Download completed\n\n%s", displayName))

	cfg := config.GetOrganizeConfig()
	if !cfg.Enabled {
		return
	}

	org := buildOrganizer()
	report := makeOrganizeReporter(bot, chatID, "🔍 Analyzing content")

	var (
		res *organize.Result
		err error
	)
	info, statErr := os.Stat(srcPath)
	if statErr == nil && info.IsDir() {
		res, err = org.OrganizeDirectory(srcPath, report)
	} else {
		res, err = org.OrganizeFile(srcPath, report)
	}
	if err != nil {
		logger.Error("organize failed for %s: %v", srcPath, err)
		sendPlain(bot, chatID, "⚠️ Organize failed: "+err.Error())
		return
	}
	sendOrganizeSummary(bot, chatID, displayName, res)
}

// sendPlain sends a simple message.
func sendPlain(bot *tgBotApi.BotAPI, chatID int64, text string) {
	msg := tgBotApi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		logger.Error("send message failed: %v", err)
	}
}

// makeOrganizeReporter builds a Reporter that renders the plan-style
// "Analyzing + moving" live message.
func makeOrganizeReporter(bot *tgBotApi.BotAPI, chatID int64, stepTitle string) organize.Reporter {
	var live *OrganizeProgressMsg
	var lastText string
	var lastSend time.Time
	return func(p organize.Progress) {
		if p.Total <= 0 {
			return
		}
		text := "🗂 Organizing...\n\n" +
			stepTitle + "\n" +
			p.Detail + "\n\n" +
			"📂 Moving files\n" +
			progressBar(p.Done, p.Total) + "\n" +
			fmt.Sprintf("%d/%d files", p.Done, p.Total)
		if text == lastText {
			return
		}
		if live == nil || time.Since(lastSend) >= time.Second {
			if live == nil {
				live = NewOrganizeProgressMsg(bot, chatID, text)
			} else {
				live.Update(text)
			}
			lastSend = time.Now()
			lastText = text
		}
	}
}

// sendOrganizeSummary posts the final detailed summary (plan style).
func sendOrganizeSummary(bot *tgBotApi.BotAPI, chatID int64, sourceName string, res *organize.Result) {
	if res == nil {
		return
	}
	tree := organize.Tree(res.Moved, libraryRoots())
	failed := ""
	if len(res.Failed) > 0 {
		failed = fmt.Sprintf("• %d failed\n", len(res.Failed))
	}
	files := "files"
	if len(res.Moved) == 1 {
		files = "file"
	}
	text := fmt.Sprintf(
		"✅ All done!\n\n"+
			"📦 %s processed\n%s\n\n"+
			"🗂 Result\n"+
			"• %d %s moved\n"+
			"%s\n"+
			"%s\n\n"+
			"📦 Size: %s\n"+
			"⏱ Time taken: %s",
		organize.CategoryLabel(res.Category),
		sourceName,
		len(res.Moved),
		files,
		failed,
		tree,
		typeTrans.Byte2Readable(float64(res.SizeBytes)),
		formatDuration(res.Duration),
	)
	sendPlain(bot, chatID, text)
}
// handleDownloadComplete is invoked by the notifier on download completion.
// Resolves the finished download path and starts the organize pipeline.
func handleDownloadComplete(events []rpc.Event) {
	if len(events) == 0 || activeBot == nil {
		return
	}
	gid := events[0].Gid
	srcPath, name := input.ToolApp.Aria2.DownloadedPath(gid)
	if srcPath == "" {
		return
	}
	go runOrganize(activeBot, organizeChatID(), srcPath, name)
}
