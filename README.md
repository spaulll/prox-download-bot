# prox-download-bot

English-only Telegram bot that controls **Aria2** with native AriaFlow-style
media organization, accurate real-time progress bars, and **yt-dlp** support
(any site yt-dlp handles, at 1080p + best audio).

> A heavy remod / derivative of [DownloadBot](https://github.com/gaowanliang/DownloadBot)
> by **gaowanliang (Gaowan Liang)** — licensed under Apache-2.0 with full
> attribution. See [NOTICE](NOTICE) and [LICENSE](LICENSE).

## Features

- **Aria2 control** — add http/ftp/magnet/torrent links via Telegram, live
  progress with automatic pop-up when a download starts, pause / resume /
  remove tasks
- **Progress bars** — 13-segment bar with float percent everywhere:
  `[●●●●●●●○○○○○○] 42.69 %`
  - aria2 downloads: filename, downloaded / total, speed, ETA, threads, GID
  - yt-dlp downloads: same detail lines, parsed live from yt-dlp
  - extraction: byte-accurate `Extracted: 2.23 GB of 9.50 GB` + ETA
    (updates continuously, even within a single huge file)
  - moving: `N/M files` per step
- **Native media organization** — completed downloads are sorted into a
  media library (AriaFlow directory parity):
  - `movies / series / anime / music / documents / archives / others`
  - Episode detection: `S01E01`, `S1E1`, `1x01`, `s01.e01`, ...
  - Season extraction → `Season N` folders (no leading zero)
  - Single episodes are never treated as season packs
  - Smart folder matching (normalize + token overlap)
  - AniList lookup for anime confirmation (default on, configurable) with
    fuzzy release-name matching (`Anime.Show.S03-...` → `Anime Show`)
  - Clean file names: `Some.Show.S02E04.1080p.x264.Hindi...mkv`
    → `Some.Show.S02E04.mkv`
  - Duplicate files get numeric suffixes instead of being overwritten
- **Archive handling** — zip / rar / 7z / tar / gz / bz2 / xz are extracted
  (native Go first, 7z / unrar / unzip fallback), then re-run through the
  organizer; season packs are detected and moved as a unit
  - Extraction stages on real disk next to the archive (never RAM-backed
    /tmp) with a free-space pre-flight check
  - Original archive deleted after successful extraction (`deleteArchive`)
  - Optional nested / multi-volume archive support (`.partN.rar` sets)
- **yt-dlp integration** — every site yt-dlp supports (YouTube, Pornhub,
  Twitter/X, Instagram, Bilibili, ...):
  - 1080p video + best audio, metadata + thumbnail embedding
  - `YouTube/<Channel>/[<Playlist>/]<video>.mp4`
  - `<Service>/<Channel>/[<Playlist>/]<video>.mp4`
  - Live progress (percent, size, speed, ETA) with sanitized names
  - Final summary shows the exact save location
- **Crash-safe operations** — if the bot or the machine dies mid-organize:
  - orphaned extraction staging dirs are swept on startup
  - completed-but-unprocessed archives are re-organized automatically
    (`🔄 Recovery` notice)
  - in-flight progress messages left by the dead run are deleted
- **Multi-user with admin approval** — new users request access via `/start`;
  the admin gets Approve / Deny buttons. Decisions replace the request
  message (buttons hidden, meaningful confirmation shown). Denied users can
  `/start` again to re-request (fresh message, no scrolling). Regular users
  only see their own tasks; the admin sees everything.
- **History with timestamps** — `✅ Finished/Stopped` shows the newest
  completed tasks first (junk 0-byte entries filtered and purged) with a
  `Finished: 02 Sep, 11:37` timestamp, plus a `🗑 Clear history` button that
  purges aria2's history and deletes the message
- **Torrent / magnet file selection** — pick files from a torrent interactively

## Library layout (Aria2 downloads)

```
media
├── movies
├── series
│   └── Some Show
│       └── Season 2
│           └── Some.Show.S02E04.mkv
├── anime
│   └── Anime Show
│       └── Season 3
│           └── Anime.Show.S03E01.mkv
├── music
├── documents
├── archives
└── others
```

## yt-dlp layout

```
YouTube
└── Channel Name
    ├── video.mp4
    └── Playlist Name
        ├── video1.mp4
        └── video2.mp4
Services
└── ServiceName
    └── Uploader
        └── video.mp4
```

## Setup

1. Install: aria2 (with RPC), Go 1.21+, and optionally `yt-dlp` + `ffmpeg`
   (`7z` / `unrar` / `unzip` for best archive support).
2. Copy `default.config.json` to `config.json` and fill in:
   - `input.aria2.aria2-server` / `aria2-key` — Aria2 websocket RPC endpoint
   - `output.telegram.bot-key` — bot token from @BotFather
   - `output.telegram.user-id` — your Telegram numeric user id (admin)
   - `downloadFolder` — where aria2 saves downloads
   - `organize.*` — media library paths
3. Build and run:

```bash
go build -o prox-download-bot ./cmd/DownloadBot
./prox-download-bot -c ./config.json
```

4. Open the bot in Telegram and `/start`.

### systemd

```ini
[Unit]
Description=DownloadBot (Telegram Aria2 bot)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/prox-download-bot
ExecStart=/opt/prox-download-bot/prox-download-bot
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

> Rebuild after pulling: `go build -o prox-download-bot ./cmd/DownloadBot`,
> then `systemctl restart <service>`.

## Configuration reference

| Key | Description |
|-----|-------------|
| `downloadFolder` | Aria2 download directory |
| `organize.enabled` | Post-download organizing on/off |
| `organize.anilist` | AniList anime confirmation (default on) |
| `organize.deleteArchive` | Delete archives after successful extraction |
| `organize.movies/series/anime/music/documents/archives/others` | Library roots |
| `organize.youtube` | Base path for YouTube downloads (default `<downloadFolder>/YouTube`) |
| `organize.services` | Base path for other yt-dlp services (default `<downloadFolder>/Services`) |
| `organize.ytdlpPath` | yt-dlp binary path |
| `organize.ytdlpQuality` | yt-dlp format selector (default 1080p + best audio) |
| `organize.ytdlpCookies` | cookies.txt path for yt-dlp |
| `organize.ytdlpProxy` | proxy URL for yt-dlp |
| `organize.ytdlpEmbed` | embed metadata + thumbnail (needs ffmpeg) |

## Example messages

**Live download (auto-appears when a download starts):**
```
Filename: Some.Show.S01.720p.zip
[●●●○○○○○○○○○○] 23.45 %
Downloaded: 607.06 MB of 2.59 GB
Speed: 8.80 MB/s
ETA: 4 m 12 s
Threads: 16
GID: d829eeca5dc91475
```

**Extraction:**
```
🗂 Organizing...

📦 Extracting
[●●●○○○○○○○○○○] 23.49 %
Extracted: 2.23 GB of 9.50 GB
ETA: 6m 41s
3/11 files
```

**Final summary (intermediate messages are deleted automatically):**
```
✅ All done!

📦 Archive processed
Some.Show.S01.720p.zip

🗂 Result
• 10 episodes moved

Series
└── Some Show
    └── Season 1
        ├── Some.Show.S01E01.mkv
        ├── ...
        └── Some.Show.S01E10.mkv

📦 Size: 2.59 GB
⏱ Time taken: 2m 0s
```

## Credits

- **Original project:** [DownloadBot](https://github.com/gaowanliang/DownloadBot)
  by [gaowanliang (Gaowan Liang)](https://github.com/gaowanliang) — Apache-2.0
- This repository is a heavily reworked derivative with an English-only UI,
  native Go media organization, archive extraction, and yt-dlp support.
- Aria2 RPC client based on the original project's vendored implementation.

## License

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
