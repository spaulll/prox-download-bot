package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/internal/organize"
	"DownloadBot/tool/input"
	"DownloadBot/tool/input/aria2/rpc"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	// track for cleanup if the process dies mid-run
	inflightAdd(chatID, res.MessageID)
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

// Delete removes the progress message (cleanup after task success).
func (m *OrganizeProgressMsg) Delete() {
	if m == nil {
		return
	}
	if _, err := m.bot.Request(tgBotApi.NewDeleteMessage(m.chatID, m.messageID)); err != nil {
		logger.Debug("delete organize message failed: %v", err)
	}
	inflightRemove(m.messageID)
}

// ID returns the underlying Telegram message ID.
func (m *OrganizeProgressMsg) ID() int {
	if m == nil {
		return 0
	}
	return m.messageID
}

// segmentBar renders the 13-segment float bar used across all bot messages:
// [●●●○○○○○○○○○○] 12.34 %
func segmentBar(percent float64) string {
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

// segmentBarRatio renders segmentBar from a done/total pair.
func segmentBarRatio(done, total int) string {
	if total <= 0 {
		return segmentBar(0)
	}
	return segmentBar(float64(done) / float64(total) * 100)
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
			Torrents:  torrentsDir(),
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
		"Torrents":  torrentsDir(),
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

// remainingETA estimates time left from average throughput so far.
// Returns 0 when there is nothing meaningful to estimate yet.
func remainingETA(start time.Time, done, total int64) time.Duration {
	if done <= 0 || total <= 0 || done >= total {
		return 0
	}
	elapsed := time.Since(start)
	if elapsed < 2*time.Second {
		return 0
	}
	rate := float64(done) / elapsed.Seconds()
	if rate <= 0 {
		return 0
	}
	return time.Duration(float64(total-done)/rate) * time.Second
}

// organizeChatID returns the admin chat id for bot messages.
func organizeChatID() int64 {
	return typeTrans.Str2Int64(config.GetTelegramUserID())
}

// runOrganize executes the organize pipeline for a completed download and
// drives the live Telegram progress messages. Runs asynchronously.
func runOrganize(bot *tgBotApi.BotAPI, chatID int64, gid, srcPath, displayName string) {
	// plan-style "Download completed" message (deleted once organizing ends)
	completedMsg := sendPlain(bot, chatID, fmt.Sprintf("✅ Download completed\n\n%s", displayName))
	inflightAdd(chatID, completedMsg)
	notifyUserTaskDone(gid, displayName)

	cfg := config.GetOrganizeConfig()
	if !cfg.Enabled {
		return
	}

	// safety: never organize the download root itself (a mis-resolved
	// torrent path here would walk the entire library staging area)
	if samePath(srcPath, config.GetDownloadFolder()) {
		logger.Error("refusing to organize download root for gid %s", gid)
		return
	}

	org := buildOrganizer()

	// archive -> extraction pipeline (extract, then re-run organize)
	if organize.IsArchive(srcPath) {
		go runArchivePipeline(bot, chatID, gid, org, srcPath, displayName, completedMsg)
		return
	}

	report, cleanup := makeOrganizeReporter(bot, chatID, "🔍 Analyzing content")

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
	// a .torrent file was stored: link its follow-torrent children to the
	// stored path so the keep toggle can drop it after the content lands
	if organize.IsTorrent(srcPath) && len(res.Moved) > 0 {
		stored := res.Moved[len(res.Moved)-1]
		for _, child := range followedChildren(gid) {
			rememberTorrentFile(child, stored)
		}
	}
	maybeDropTorrentFile(gid)
	// success: wipe intermediates, keep only the final summary
	cleanup()
	deleteMessages(bot, chatID, completedMsg)
	sendOrganizeSummary(bot, chatID, displayName, res)
}

// samePath reports whether two paths resolve to the same directory.
func samePath(a, b string) bool {
	ca, errA := filepath.Abs(filepath.Clean(a))
	cb, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ca == cb
}

// sendPlain sends a simple message, returning its message ID (0 on failure).
func sendPlain(bot *tgBotApi.BotAPI, chatID int64, text string) int {
	msg := tgBotApi.NewMessage(chatID, text)
	res, err := bot.Send(msg)
	if err != nil {
		logger.Error("send message failed: %v", err)
		return 0
	}
	return res.MessageID
}

// deleteMessages removes messages by ID (0 IDs are skipped).
func deleteMessages(bot *tgBotApi.BotAPI, chatID int64, ids ...int) {
	for _, id := range ids {
		if id == 0 {
			continue
		}
		// deleteMessage answers with plain true - use Request, not Send
		if _, err := bot.Request(tgBotApi.NewDeleteMessage(chatID, id)); err != nil {
			logger.Debug("delete message failed: %v", err)
		}
		inflightRemove(id)
	}
}

// makeOrganizeReporter builds a Reporter that renders the plan-style
// "Analyzing + moving" live message, plus a cleanup func that deletes it.
func makeOrganizeReporter(bot *tgBotApi.BotAPI, chatID int64, stepTitle string) (organize.Reporter, func()) {
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
			segmentBarRatio(p.Done, p.Total) + "\n" +
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
	}, func() { live.Delete() }
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
	go func() {
		// resolve path with retries: the RPC connection may be busy right
		// after the completion notification
		var srcPath, name string
		for attempt := 0; attempt < 5; attempt++ {
			srcPath, name = input.ToolApp.Aria2.DownloadedPath(gid)
			if srcPath != "" {
				break
			}
			time.Sleep(3 * time.Second)
		}
		if srcPath == "" {
			logger.Error("could not resolve download path for gid %s", gid)
			return
		}
		runOrganize(activeBot, organizeChatID(), gid, srcPath, name)
	}()
}
