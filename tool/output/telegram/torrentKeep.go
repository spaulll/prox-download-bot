package telegram

import (
	"DownloadBot/internal/config"
	"DownloadBot/tool/input"
	logger "DownloadBot/tool/zap"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Torrent-file handling: every .torrent sent to the bot is stored in the
// torrents folder, then its content is downloaded. keepTorrent controls
// whether the .torrent file survives after the content is organized.

// torrentFileOf maps a torrent content GID -> stored .torrent file path.
// Filled when the .torrent file is organized (via followedBy) or uploaded,
// consumed when the content organize finishes.
var (
	torrentFileMu sync.Mutex
	torrentFileOf = map[string]string{}
)

func rememberTorrentFile(childGID, torrentPath string) {
	if childGID == "" || torrentPath == "" {
		return
	}
	torrentFileMu.Lock()
	torrentFileOf[childGID] = torrentPath
	torrentFileMu.Unlock()
}

func takeTorrentFile(gid string) (string, bool) {
	torrentFileMu.Lock()
	defer torrentFileMu.Unlock()
	p, ok := torrentFileOf[gid]
	if ok {
		delete(torrentFileOf, gid)
	}
	return p, ok
}

// torrentsDir resolves the .torrent storage folder.
func torrentsDir() string {
	cfg := config.GetOrganizeConfig()
	if cfg.Torrents != "" {
		return cfg.Torrents
	}
	return filepath.Join(config.GetDownloadFolder(), "torrents")
}

// keepTorrentFile reports whether .torrent files are kept after organizing.
// Defaults to true when the key is omitted from config.json.
func keepTorrentFile() bool {
	if config.GetOrganizeConfig().KeepTorrent == nil {
		return true
	}
	return *config.GetOrganizeConfig().KeepTorrent
}

// followedChildren returns GIDs aria2 auto-generated from gid
// (follow-torrent / follow-metalink). Retried briefly: the child may appear
// a moment after the parent completes.
func followedChildren(gid string) []string {
	for i := 0; i < 6; i++ {
		info, err := input.ToolApp.Aria2.TellStatusFull(gid)
		if err != nil {
			logger.Error("followedChildren tellStatus failed for %s: %v", gid, err)
			return nil
		}
		if len(info.FollowedBy) > 0 {
			return info.FollowedBy
		}
		time.Sleep(time.Second)
	}
	return nil
}

// maybeDropTorrentFile deletes the stored .torrent file for gid when the
// keep toggle is off. Called at every organize-completion point.
func maybeDropTorrentFile(gid string) {
	path, ok := takeTorrentFile(gid)
	if !ok || keepTorrentFile() {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Error("failed to remove .torrent file %s: %v", path, err)
		return
	}
	logger.Info("Removed .torrent file %s (keepTorrent off)", path)
}

// rememberUploadedTorrent stores a copy of an uploaded temp.torrent in the
// torrents folder and links it to its content GID for the keep toggle.
func rememberUploadedTorrent(gid string) {
	name := input.ToolApp.Aria2.TellName(gid)
	if name == "" {
		name = gid
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	dir := torrentsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Error("failed to prepare torrents dir %s: %v", dir, err)
		return
	}
	dest := uniqueFilePath(filepath.Join(dir, name))
	src, err := os.Open("temp.torrent")
	if err != nil {
		logger.Error("failed to read uploaded torrent: %v", err)
		return
	}
	defer src.Close()
	out, err := os.Create(dest)
	if err != nil {
		logger.Error("failed to store uploaded torrent %s: %v", dest, err)
		return
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		logger.Error("failed to store uploaded torrent %s: %v", dest, err)
		return
	}
	out.Close()
	logger.Info("Stored uploaded torrent %s", dest)
	rememberTorrentFile(gid, dest)
}

// uniqueFilePath appends " (1)", " (2)", ... before the extension when dst
// exists (mirrors organize.uniquePath for the telegram layer).
func uniqueFilePath(dst string) string {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return dst
	}
	ext := filepath.Ext(dst)
	base := dst[:len(dst)-len(ext)]
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}
