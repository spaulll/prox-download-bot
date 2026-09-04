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
├── others
└── torrents
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

## Download (prebuilt binaries)

No Go toolchain needed — grab a ready-made binary from the
[**Releases**](https://github.com/spaulll/prox-download-bot/releases) page.
Each release is built automatically by CI from a `v*` tag.

| File | OS | Arch | Typical use |
|------|----|------|-------------|
| `DownloadBot-linux-amd64` | Linux | x86-64 | most servers / VPS / NAS |
| `DownloadBot-linux-arm64` | Linux | ARM 64-bit | Raspberry Pi 3/4/5 (64-bit OS), ARM VPS |
| `DownloadBot-linux-armv7` | Linux | ARM 32-bit | older Raspberry Pi (32-bit OS) |
| `DownloadBot-linux-386` | Linux | x86 32-bit | old 32-bit machines |
| `DownloadBot-windows-amd64.exe` | Windows | x86-64 | Windows server / desktop |
| `DownloadBot-darwin-amd64` | macOS | Intel | Intel Macs |
| `DownloadBot-darwin-arm64` | macOS | Apple Silicon | M1/M2/M3/M4 Macs |

> `windows/arm64` is not provided: the vendored `go-ole` dependency (via
> `gopsutil`) only supports `windows/386`+`windows/amd64`, so that target
> cannot compile.

Run it (Linux/macOS):

```bash
chmod +x DownloadBot-linux-amd64
./DownloadBot-linux-amd64 -c ./config.json
```

On Windows, run `DownloadBot-windows-amd64.exe -c .\config.json` in
PowerShell/CMD. The binary is fully static with translations embedded —
no installation, no sidecar files (an optional `./i18n/*.json` overlay can
override messages).

## Runtime dependencies

The bot binary is static, but it shells out to external tools. Install what
matches the features you use:

| Tool | Needed for | If missing |
|------|------------|------------|
| `aria2` | everything (core downloader, over RPC) | **required** — bot can't download |
| `yt-dlp` | video-page URLs (YouTube, etc.) | those fail with `yt-dlp binary not found`, rest works |
| `ffmpeg` | merging 1080p video + best audio, metadata/thumbnail embedding | yt-dlp merges/embeds fail |
| `7z` | `.7z` / `.rar` extraction (plus fallback) | those archives fail with `no extraction backend available` |
| `unrar` | `.rar` extraction fallback | rar fails unless `7z` handles it |
| `unzip` | nothing in practice | `.zip`/`.tar.*` are handled natively in Go |

### Linux (`amd64`, `arm64`, `armv7`, `386`) — Debian / Ubuntu / Raspberry Pi OS

```bash
sudo apt update
sudo apt install aria2 ffmpeg unzip p7zip-full unrar
```

- `unrar` lives in Debian `non-free` / Ubuntu `multiverse` — enable that repo
  if apt can't find it. Without it, most rar files still extract via `7z`.
- Do **not** install `yt-dlp` from apt (hopelessly outdated). Use the
  standalone build matching your arch from
  [yt-dlp releases](https://github.com/yt-dlp/yt-dlp/releases):

  | Your binary | yt-dlp asset |
  |-------------|--------------|
  | `DownloadBot-linux-amd64` | `yt-dlp_linux` |
  | `DownloadBot-linux-arm64` | `yt-dlp_linux_aarch64` |
  | `DownloadBot-linux-armv7` | `yt-dlp_linux_armv7l` |
  | `DownloadBot-linux-386` | none published — use `pip install yt-dlp` |

  ```bash
  # example: 64-bit Raspberry Pi / ARM server
  sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux_aarch64 \
    -o /usr/local/bin/yt-dlp
  sudo chmod +x /usr/local/bin/yt-dlp
  ```

  Point `ytdlpPath` in `config.json` at it if it's not on `PATH`.

### Windows (`amd64`) — winget

```powershell
winget install -e --id aria2.aria2
winget install -e --id yt-dlp.yt-dlp
winget install -e --id Gyan.FFmpeg
winget install -e --id 7zip.7zip
```

- `unzip`/`unrar` CLIs are not needed: zip is handled natively in Go and
  7-Zip covers rar extraction (grab the official `unrar` CLI from
  [rarlab.com](https://www.rarlab.com/) only for stubborn archives).
- Open a fresh shell afterwards so `PATH` picks up the new tools.

### macOS (`amd64` Intel, `arm64` Apple Silicon) — Homebrew

```bash
brew install aria2 yt-dlp ffmpeg p7zip
```

- `unzip`/`tar` ship with macOS, and zip/tar are handled natively in Go
  anyway.
- There is no `unrar` formula in homebrew-core, and the `rar` cask has been
  retired — for full rar support download the official CLI from
  [rarlab.com](https://www.rarlab.com/) and put `unrar` on your `PATH`.
  Most rar files extract via `7z` without it.

## Compile from source

Requirements: **Go 1.21+**.

```bash
git clone https://github.com/spaulll/prox-download-bot.git
cd prox-download-bot
go build -mod=vendor -trimpath -ldflags "-s -w" -o DownloadBot ./cmd/DownloadBot
./DownloadBot -c ./config.json
```

Cross-compile for another machine (examples):

```bash
# Raspberry Pi (64-bit OS)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -o DownloadBot-linux-arm64 ./cmd/DownloadBot
# Raspberry Pi (32-bit OS)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -mod=vendor -o DownloadBot-linux-armv7 ./cmd/DownloadBot
# Windows from Linux/macOS
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -o DownloadBot-windows-amd64.exe ./cmd/DownloadBot
# macOS Apple Silicon from Linux
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -o DownloadBot-darwin-arm64 ./cmd/DownloadBot
```

CI builds all supported targets automatically — see
[`.github/workflows/build.yml`](.github/workflows/build.yml).

## Setup guide

### Prerequisites

- **aria2** with RPC enabled (the actual downloader)
- A **Telegram bot token** — talk to [@BotFather](https://t.me/BotFather),
  `/newbot`, copy the token
- Your **numeric Telegram user id** (the admin) — talk to
  [@userinfobot](https://t.me/userinfobot), it replies with your id
- Install the external tools for your OS/arch — see
  [Runtime dependencies](#runtime-dependencies) (`aria2` is mandatory,
  `yt-dlp` + `ffmpeg` for video URLs, `7z`/`unrar` for archives)

### 1. Start aria2 with RPC enabled

The bot talks to aria2 over its websocket RPC interface:

```bash
aria2c --enable-rpc --rpc-listen-all --rpc-secret=YOUR_SECRET \
  --dir=/root/downloads --continue=true --max-concurrent-downloads=5
```

- `--dir` **must match** `downloadFolder` in `config.json`
- Note the RPC address (`ws://127.0.0.1:6800/jsonrpc`) and the secret —
  they go into the config next

### 2. Create the config

```bash
cp default.config.json config.json
```

Edit `config.json`:

```json
{
  "input": {
    "aria2": {
      "aria2-server": "ws://127.0.0.1:6800/jsonrpc",
      "aria2-key": "YOUR_SECRET"
    }
  },
  "output": {
    "telegram": {
      "bot-key": "123456:ABC-DEF_your_bot_token",
      "user-id": "123456789"
    }
  },
  "downloadFolder": "/root/downloads",
  "organize": {
    "enabled": true,
    "anilist": true,
    "deleteArchive": false,
    "movies": "/media/movies",
    "series": "/media/series",
    "anime": "/media/anime",
    "music": "/media/music",
    "documents": "/media/documents",
    "archives": "/media/archives",
    "others": "/media/others",
    "torrents": "/media/torrents",
    "keepTorrent": true,
    "youtube": "/media/YouTube",
    "services": "/media/Services",
    "ytdlpPath": "yt-dlp",
    "ytdlpQuality": "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"
  }
}
```

| Key | What it is |
|-----|------------|
| `input.aria2.aria2-server` | aria2 websocket RPC endpoint from step 1 |
| `input.aria2.aria2-key` | aria2 `--rpc-secret` from step 1 |
| `output.telegram.bot-key` | token from @BotFather |
| `output.telegram.user-id` | your numeric id from @userinfobot (bot admin — exactly one ID, enforced at startup) |
| `downloadFolder` | must be aria2's `--dir` |
| `organize.*` | media library roots (see Configuration reference) |

> `config.json` (and `users.json`) are git-ignored — never commit them.

### 3. Create the directories

```bash
mkdir -p /root/downloads /media/{movies,series,anime,music,documents,archives,others,YouTube,Services}
```

Adjust to whatever paths you put in `config.json`.

### 4. Run the bot

Prebuilt binary:

```bash
chmod +x DownloadBot-linux-amd64
./DownloadBot-linux-amd64 -c ./config.json
```

Or self-built:

```bash
go build -o DownloadBot ./cmd/DownloadBot
./DownloadBot -c ./config.json
```

The `-c` flag points at your config (default `./config.json`).

### 5. First use in Telegram

1. Open your bot and send `/start`.
2. If you are not the admin, the admin gets **Approve / Deny** buttons —
   approve the request, then the user sends `/start` again.
3. Send the bot an HTTP/FTP link, magnet, torrent file, or a video-page URL
   (YouTube etc. via yt-dlp).
4. For torrents/magnets you can pick files interactively; progress appears
   live, and finished downloads are sorted into the media library.

### 6. Run it as a service (systemd)

```ini
[Unit]
Description=DownloadBot (Telegram Aria2 bot)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/prox-download-bot
ExecStart=/opt/prox-download-bot/DownloadBot -c /opt/prox-download-bot/config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now downloadbot
```

> Update later by replacing the binary (or `git pull` + rebuild) and
> `sudo systemctl restart downloadbot`.

## Configuration reference

| Key | Description |
|-----|-------------|
| `downloadFolder` | Aria2 download directory |
| `organize.enabled` | Post-download organizing on/off |
| `organize.anilist` | AniList anime confirmation (default on) |
| `organize.deleteArchive` | Delete archives after successful extraction |
| `organize.movies/series/anime/music/documents/archives/others` | Library roots |
| `organize.torrents` | `.torrent` file storage (default `<downloadFolder>/torrents`) |
| `organize.keepTorrent` | Keep the `.torrent` file after its content is organized (default `true`; `false` deletes it) |
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
