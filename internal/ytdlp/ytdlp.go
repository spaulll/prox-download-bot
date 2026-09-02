// Package ytdlp integrates yt-dlp for all supported video sites.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"DownloadBot/internal/organize"
)

// Config holds yt-dlp settings.
type Config struct {
	// BinPath is the yt-dlp binary (default "yt-dlp")
	BinPath string
	// Quality is the format selector; default 1080p + best audio
	Quality string
	// BaseYouTube is the root for YouTube downloads
	BaseYouTube string
	// BaseServices is the root for other supported sites
	BaseServices string
	// Cookies is the path to a cookies.txt file (optional)
	Cookies string
	// Proxy is an http/socks proxy (optional)
	Proxy string
	// EmbedThumbnail embeds the video thumbnail (needs ffmpeg)
	EmbedThumbnail bool
	// EmbedMetadata embeds metadata tags (needs ffmpeg)
	EmbedMetadata bool
}

// Progress is reported during download.
type Progress struct {
	Percent float64
	Stage   string // "downloading" | "processing" | "done"
	Detail  string
}

// Reporter receives progress updates.
type Reporter func(p Progress)

// Result describes a finished download.
type Result struct {
	Files     []string // final file paths
	Title     string
	Channel   string
	Service   string // e.g. "Youtube", "Pornhub", "Twitter"
	SizeBytes int64
	Duration  time.Duration
}

// KnownURLHosts are domains that should always go through yt-dlp.
var KnownURLHosts = []string{
	"youtube.com", "youtu.be", "m.youtube.com", "music.youtube.com",
	"pornhub.com", "twitter.com", "x.com", "instagram.com",
	"bilibili.com", "b23.tv", "tiktok.com", "vimeo.com",
	"dailymotion.com", "soundcloud.com", "twitch.tv", "reddit.com",
	"vk.com", "ok.ru", "vkvideo.ru", "youku.com", "iqiyi.com",
	"facebook.com", "fb.watch", "tumblr.com", "imgur.com",
}

var hostRe = regexp.MustCompile(`(?i)^(?:https?://)?([a-z0-9.-]+\.[a-z]{2,})(?:[/:?#].*)?$`)

// IsKnownSite reports whether the URL belongs to a yt-dlp-known domain.
func IsKnownSite(rawURL string) bool {
	m := hostRe.FindStringSubmatch(strings.TrimSpace(rawURL))
	if m == nil {
		return false
	}
	host := strings.ToLower(m[1])
	for _, known := range KnownURLHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// ErrNoBinary is returned when yt-dlp is not installed.
var ErrNoBinary = errors.New("yt-dlp binary not found (set ytdlpPath in config)")

type Downloader struct {
	Cfg Config
	// Logf logs decisions
	Logf func(format string, v ...interface{})
}

func (d *Downloader) log(format string, v ...interface{}) {
	if d.Logf != nil {
		d.Logf(format, v...)
	}
}

func (d *Downloader) bin() (string, error) {
	bin := d.Cfg.BinPath
	if bin == "" {
		bin = "yt-dlp"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return "", ErrNoBinary
	}
	return bin, nil
}

// probeInfo is the subset of yt-dlp JSON we need.
type probeInfo struct {
	Type          string `json:"_type"`
	Title         string `json:"title"`
	ExtractorKey  string `json:"extractor_key"`
	Channel       string `json:"channel"`
	Uploader      string `json:"uploader"`
	UploaderID    string `json:"uploader_id"`
	PlaylistTitle string `json:"playlist_title"`
	NEntries      int    `json:"n_entries"`
}

// Probe runs yt-dlp --dump-single-json to inspect the URL. It returns
// probeInfo or an error when the URL is not yt-dlp compatible.
func (d *Downloader) Probe(rawURL string) (*probeInfo, error) {
	bin, err := d.bin()
	if err != nil {
		return nil, err
	}
	args := []string{
		"--flat-playlist", "--dump-single-json", "--no-warnings", "--skip-download",
		"--socket-timeout", "15",
	}
	args = d.addOptionalArgs(args)
	args = append(args, rawURL)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp probe failed: %w", err)
	}
	var info probeInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("yt-dlp probe decode failed: %w", err)
	}
	return &info, nil
}

func (d *Downloader) addOptionalArgs(args []string) []string {
	if d.Cfg.Cookies != "" {
		args = append(args, "--cookies", d.Cfg.Cookies)
	}
	if d.Cfg.Proxy != "" {
		args = append(args, "--proxy", d.Cfg.Proxy)
	}
	return args
}

// hasFfmpeg checks whether ffmpeg is available for embedding.
func hasFfmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// Destination computes the output directory and file template for a probe
// result, following the plan's storage rules:
//
//	/YouTube/<Channel>/[<Playlist>/]<video>.mp4        (YouTube)
//	/<Service>/<Channel>/[<Playlist>/]<video>.mp4      (other sites)
func (d *Downloader) Destination(info *probeInfo) (dir string) {
	channel := firstNonEmpty(info.Channel, info.Uploader, info.UploaderID, "Unknown")
	service := firstNonEmpty(info.ExtractorKey, "Other")

	if strings.EqualFold(service, "youtube") || strings.EqualFold(service, "youtube:tab") {
		dir = filepath.Join(d.Cfg.BaseYouTube, organize.SanitizeFolderName(channel))
	} else {
		dir = filepath.Join(d.Cfg.BaseServices, organize.SanitizeFolderName(service), organize.SanitizeFolderName(channel))
	}

	// playlist subfolder: when the video comes from a playlist whose name
	// differs from the channel/uploader (channel listings stay flat)
	playlist := info.PlaylistTitle
	if playlist != "" && !strings.EqualFold(playlist, channel) {
		dir = filepath.Join(dir, organize.SanitizeFolderName(playlist))
	}
	return dir
}

// Download runs yt-dlp for the URL, placing files into the configured
// structure, reporting live progress. Returns the final files.
func (d *Downloader) Download(rawURL string, report Reporter) (*Result, error) {
	bin, err := d.bin()
	if err != nil {
		return nil, err
	}
	info, err := d.Probe(rawURL)
	if err != nil {
		return nil, err
	}
	dir := d.Destination(info)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	quality := d.Cfg.Quality
	if quality == "" {
		quality = "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"
	}
	// without ffmpeg, only combined (muxed) formats can be used - separate
	// video+audio streams would never be merged
	mergeOK := hasFfmpeg()
	if !mergeOK {
		quality = "best[height<=1080]/best"
	}

	args := []string{
		"-f", quality,
		"--merge-output-format", "mp4",
		"--newline",
		"--no-mtime",
		"--no-warnings",
		"-o", filepath.Join(dir, "%(title)s.%(ext)s"),
		"--print", "after_move:filepath",
	}
	if (d.Cfg.EmbedThumbnail || d.Cfg.EmbedMetadata) && mergeOK {
		if d.Cfg.EmbedMetadata {
			args = append(args, "--embed-metadata", "--embed-chapters")
		}
		if d.Cfg.EmbedThumbnail {
			args = append(args, "--embed-thumbnail")
		}
	}
	args = d.addOptionalArgs(args)
	args = append(args, rawURL)

	d.log("yt-dlp download started: %s -> %s", rawURL, dir)
	start := time.Now()

	cmd := exec.Command(bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var files []string
	progressRe := regexp.MustCompile(`\[download\]\s+([0-9.]+)% of\s+~?\s*([0-9.]+)([KMG]iB)`)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	errCh := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) != "" {
				d.log("yt-dlp: %s", line)
			}
		}
		errCh <- nil
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if m := progressRe.FindStringSubmatch(trimmed); m != nil {
			var pct float64
			fmt.Sscanf(m[1], "%f", &pct)
			if report != nil {
				report(Progress{Percent: pct, Stage: "downloading", Detail: trimmed})
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			// status lines like [Merger], [Metadata], [EmbedThumbnail]
			if report != nil {
				report(Progress{Percent: 100, Stage: "processing", Detail: trimmed})
			}
			continue
		}
		if filepath.IsAbs(trimmed) {
			files = append(files, trimmed)
			if report != nil {
				report(Progress{Percent: 100, Stage: "done", Detail: trimmed})
			}
		}
	}
	<-done
	<-errCh

	res := &Result{
		Files:    files,
		Title:    info.Title,
		Channel:  firstNonEmpty(info.Channel, info.Uploader, info.UploaderID),
		Service:  firstNonEmpty(info.ExtractorKey, "Other"),
		Duration: time.Since(start),
	}
	// only report files that actually exist (a failed merge prints the
	// expected final path but leaves only fragments behind)
	var verified []string
	for _, f := range files {
		if s, err := os.Stat(f); err == nil && !s.IsDir() {
			verified = append(verified, f)
			res.SizeBytes += s.Size()
		} else {
			d.log("expected file missing after download: %s", f)
		}
	}
	res.Files = verified
	return res, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
