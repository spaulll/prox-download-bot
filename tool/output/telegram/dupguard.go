package telegram

import (
	i18nLoc "DownloadBot/i18n"
	"DownloadBot/internal/config"
	"DownloadBot/internal/organize"
	"DownloadBot/internal/users"
	"DownloadBot/tool/input"
	logger "DownloadBot/tool/zap"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// DupHit describes an already-have-it finding.
type DupHit struct {
	Kind   string // "downloading" | "season" | "episode" | "movie"
	Detail string // human-readable, e.g. "Reacher • Season 4 (8 episodes)"
}

// dupPending holds a link awaiting duplicate-override confirmation.
type dupPending struct {
	Link     string
	UserID   int64
	ChatID   int64
	Username string
	Created  time.Time
}

const dupPendingTTL = 10 * time.Minute

var (
	dupPendMu sync.Mutex
	dupPend   = map[string]dupPending{}

	// dupOverride suppresses the deferred check for user-confirmed downloads.
	dupOverMu sync.Mutex
	dupOver   = map[string]bool{}
)

// findActiveDownload returns the in-progress task for the exact same link,
// or nil. Covers every engine (same link downloading anywhere counts).
func findActiveDownload(link string) *users.Task {
	if taskStore == nil || link == "" {
		return nil
	}
	for _, t := range taskStore.All() {
		if t.Link == link && t.Status == "downloading" {
			task := t
			return &task
		}
	}
	return nil
}

// findLibraryDuplicate checks whether displayName's content already lives in
// the media library (season pack / single episode / movie). Returns nil when
// the name is not recognizable or nothing matches — always fail-open.
func findLibraryDuplicate(displayName string) *DupHit {
	cfg := config.GetOrganizeConfig()
	return findLibraryDuplicateIn(displayName, cfg.Movies, cfg.Series, cfg.Anime)
}

// findLibraryDuplicateIn is findLibraryDuplicate with explicit roots (test seam).
func findLibraryDuplicateIn(displayName, moviesRoot, seriesRoot, animeRoot string) *DupHit {
	if displayName == "" {
		return nil
	}
	// scene archives: strip the container, match episode content
	if organize.IsArchive(displayName) {
		base := strings.TrimSuffix(displayName, filepath.Ext(displayName))
		season := organize.ExtractSeason(base)
		if !organize.IsEpisode(base) && season == 0 {
			return nil // generic archive, skip
		}
		folder := matchShowFolder(base, seriesRoot, animeRoot)
		if folder == "" {
			return nil
		}
		show := filepath.Base(folder)
		if ep := organize.ExtractEpisode(base); ep > 0 && season > 0 {
			// single-episode archive: the exact episode already owned?
			clean := organize.SanitizeFileName(organize.CleanEpisodeFileName(base + ".mkv"))
			dst := filepath.Join(folder, fmt.Sprintf("Season %d", season), clean)
			if st, err := os.Stat(dst); err == nil && !st.IsDir() {
				return &DupHit{"episode", fmt.Sprintf("%s • %s", show, clean)}
			}
			return nil
		}
		if season > 0 {
			// season pack: any episodes of that season owned?
			seasonDir := filepath.Join(folder, fmt.Sprintf("Season %d", season))
			if n := countVideos(seasonDir); n > 0 {
				return &DupHit{"season", fmt.Sprintf("%s • Season %d (%d episodes)", show, season, n)}
			}
			return nil
		}
		if n := countVideosRecursive(folder); n > 0 {
			return &DupHit{"season", fmt.Sprintf("%s (already in library)", show)}
		}
		return nil
	}
	// single video files
	if organize.IsVideo(displayName) {
		if organize.IsEpisode(displayName) {
			folder := matchShowFolder(displayName, seriesRoot, animeRoot)
			if folder == "" {
				return nil
			}
			clean := organize.SanitizeFileName(organize.CleanEpisodeFileName(displayName))
			dirs := []string{folder}
			if season := organize.ExtractSeason(displayName); season > 0 {
				dirs = []string{filepath.Join(folder, fmt.Sprintf("Season %d", season))}
			}
			for _, dir := range dirs {
				if st, err := os.Stat(filepath.Join(dir, clean)); err == nil && !st.IsDir() {
					return &DupHit{"episode", fmt.Sprintf("%s • %s", filepath.Base(folder), clean)}
				}
			}
			return nil
		}
		folderRaw := organize.CleanMovieFolderName(displayName)
		if folderRaw == "" || moviesRoot == "" {
			return nil
		}
		dir := filepath.Join(moviesRoot, organize.SanitizeFolderName(folderRaw))
		if n := countVideos(dir); n > 0 {
			return &DupHit{"movie", fmt.Sprintf("%s (already in library)", folderRaw)}
		}
		return nil
	}
	return nil
}

// matchShowFolder mirrors the organizer's routing: existing series folder
// first, then anime. Returns "" when nothing matches confidently.
func matchShowFolder(candidate, seriesRoot, animeRoot string) string {
	if seriesRoot != "" {
		if p := (organize.FolderMatcher{Root: seriesRoot}).Find(candidate); p != "" {
			return p
		}
	}
	if animeRoot != "" {
		if p := (organize.FolderMatcher{Root: animeRoot}).Find(candidate); p != "" {
			return p
		}
	}
	return ""
}

// countVideos counts video files directly inside dir (0 when unreadable).
func countVideos(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && organize.IsVideo(e.Name()) {
			n++
		}
	}
	return n
}

// countVideosRecursive counts video files anywhere under dir.
func countVideosRecursive(dir string) int {
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && organize.IsVideo(info.Name()) {
			n++
		}
		return nil
	})
	return n
}

// rememberDupPending stores a link awaiting override confirmation, returning
// its callback key. Old entries are reaped opportunistically.
func rememberDupPending(link string, userID, chatID int64, username string) string {
	key := fmt.Sprintf("dup%d", time.Now().UnixNano())
	dupPendMu.Lock()
	for k, p := range dupPend {
		if time.Since(p.Created) > dupPendingTTL {
			delete(dupPend, k)
		}
	}
	dupPend[key] = dupPending{Link: link, UserID: userID, ChatID: chatID, Username: username, Created: time.Now()}
	dupPendMu.Unlock()
	return key
}

// takeDupPending consumes a pending entry (nil when unknown or expired).
func takeDupPending(key string) *dupPending {
	dupPendMu.Lock()
	defer dupPendMu.Unlock()
	p, ok := dupPend[key]
	if !ok {
		return nil
	}
	delete(dupPend, key)
	if time.Since(p.Created) > dupPendingTTL {
		return nil
	}
	return &p
}

// dupConfirmMarkup builds the Download-anyway / Cancel prompt buttons.
func dupConfirmMarkup(key string) tgBotApi.InlineKeyboardMarkup {
	return tgBotApi.NewInlineKeyboardMarkup(
		tgBotApi.NewInlineKeyboardRow(
			tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("dupDownloadAnyway"), key+":40"),
			tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("cancel"), key+":41"),
		),
	)
}

// dupDeferredMarkup builds the Keep / Stop prompt buttons for a live gid.
func dupDeferredMarkup(gid string) tgBotApi.InlineKeyboardMarkup {
	return tgBotApi.NewInlineKeyboardMarkup(
		tgBotApi.NewInlineKeyboardRow(
			tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("dupKeepDownloading"), gid+":42"),
			tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("dupStopDelete"), gid+":43"),
		),
	)
}

// dupDeferredCheck re-checks a download once its real filename is known
// (Content-Disposition arrives after start). Runs on its own goroutine — RPC
// here is safe. Pause-first so no bandwidth is wasted while the user decides.
func dupDeferredCheck(gid string) {
	time.Sleep(15 * time.Second)
	t, ok := taskStore.Get(gid)
	if !ok || t.Engine != "aria2" || t.Status != "downloading" {
		return
	}
	dupOverMu.Lock()
	approved := dupOver[gid]
	delete(dupOver, gid)
	dupOverMu.Unlock()
	if approved {
		return
	}
	st, err := input.ToolApp.Aria2.TellStatusFull(gid)
	if err != nil {
		return
	}
	if st.Status == "complete" || st.Status == "removed" || st.Status == "error" {
		return
	}
	real := ""
	if st.BitTorrent.Info.Name != "" {
		real = st.BitTorrent.Info.Name
		if strings.HasPrefix(real, "[METADATA]") {
			return // magnet metadata pseudo-download, real torrent follows
		}
	} else if len(st.Files) > 0 && st.Files[0].Path != "" {
		parts := strings.Split(st.Files[0].Path, "/")
		real = parts[len(parts)-1]
	}
	if real == "" || real == t.Name {
		return // nothing new learned
	}
	hit := findLibraryDuplicate(real)
	if hit == nil {
		return
	}
	logger.Info("duplicate guard paused %s (%s): %s", gid, real, hit.Detail)
	input.PauseTask(gid)
	text := fmt.Sprintf(i18nLoc.LocText("dupDeferredDup"), hit.Detail)
	for _, chat := range dedupChats(taskChats(t.UserID, t.ChatID)) {
		msg := tgBotApi.NewMessage(chat, text)
		msg.ReplyMarkup = dupDeferredMarkup(gid)
		if res, err := activeBot.Send(msg); err != nil {
			logger.Error("duplicate prompt send failed: %v", err)
		} else {
			inflightAdd(chat, res.MessageID)
		}
	}
}
