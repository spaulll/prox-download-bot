# prox-download-bot

English-only Telegram bot that drives **aria2** and **yt-dlp**, organizes
finished downloads into a media library (AriaFlow-style), and supports
**multi-user access with admin approval**.

Send the bot an HTTP/FTP link, a magnet/torrent, a video-page URL (YouTube,
Twitter/X, Instagram, Bilibili, …) — or just **forward a Telegram file** — and
it downloads with live progress, then files everything neatly by type.

> A heavy remod / derivative of [DownloadBot](https://github.com/gaowanliang/DownloadBot)
> by **gaowanliang (Gaowan Liang)** — Apache-2.0, with full attribution.
> See [NOTICE](NOTICE) and [LICENSE](LICENSE).

---

## Table of contents

1. [Features](#features)
2. [How it works](#how-it-works)
3. [Requirements](#requirements)
4. [Quick start (prebuilt)](#quick-start-prebuilt)
5. [Full setup, step by step](#full-setup-step-by-step)
   - [1. Install runtime tools](#1-install-runtime-tools)
   - [2. Start aria2 with RPC](#2-start-aria2-with-rpc)
   - [3. Create the Telegram bot](#3-create-the-telegram-bot)
   - [4. Optional: local Bot API server (files up to 2 GB)](#4-optional-local-bot-api-server-files-up-to-2-gb)
   - [5. Create `config.json`](#5-create-configjson)
   - [6. Create the folders](#6-create-the-folders)
   - [7. Build or download the bot](#7-build-or-download-the-bot)
   - [8. Run it as systemd services](#8-run-it-as-systemd-services)
   - [9. First use in Telegram](#9-first-use-in-telegram)
6. [Configuration reference](#configuration-reference)
7. [Media library layout](#media-library-layout)
8. [yt-dlp layout](#yt-dlp-layout)
9. [Message style](#message-style)
10. [Multi-user model](#multi-user-model)
11. [Telegram file forwarding & the 20 MB limit](#telegram-file-forwarding--the-20-mb-limit)
12. [Building from source](#building-from-source)
13. [Updating](#updating)
14. [Troubleshooting](#troubleshooting)
15. [Security notes](#security-notes)
16. [Credits & license](#credits--license)

---

## Features

- **aria2 control** — add http/https/ftp/magnet/torrent links; pause, resume
  and remove tasks; live progress that pops up automatically when a download
  starts.
- **Accurate progress bars** — the same 13-segment bar everywhere:
  `[●●●●●●●○○○○○○] 42.69 %`
  - aria2: name, downloaded/total, speed, ETA, threads, GID.
  - yt-dlp: percent, size, speed, ETA parsed live.
  - Telegram-file fetch: real percent + speed + ETA (via the local Bot API
    server's temp store; see [below](#telegram-file-forwarding--the-20-mb-limit)).
  - Archive extraction: bytes done/total + ETA; moving: `N/M files`.
- **Native media organization** (Go, no shell scripts):
  - sorts into `movies / series / anime / music / documents / archives / others`;
  - episode detection (`S01E01`, `S1E1`, `1x01`, `s01.e01`, …);
  - `Season N` folders (no leading zero); single episodes never treated as
    season packs; smart folder matching (normalized + token overlap);
  - AniList lookup for anime confirmation (default on);
  - clean names (`Some.Show.S02E04.1080p.x264.Hindi.mkv` → `Some.Show.S02E04.mkv`);
  - duplicates get numeric suffixes instead of being overwritten.
- **Archive handling** — zip/rar/7z/tar/gz/bz2/xz are extracted (native Go
  first; 7z/unrar/unzip fallback), then re-run through the organizer. Stages
  on real disk with a free-space pre-flight check; optional delete-after-extract.
- **yt-dlp** — any site yt-dlp supports, at 1080p + best audio, metadata and
  thumbnail embedded, into `YouTube/<Channel>/[<Playlist>/]<video>.mp4` or
  `<Service>/<Uploader>/…`.
- **Multi-user** — users request access via `/start`; the admin approves or
  denies. Regular users only ever see their own tasks; the admin sees
  everyone's. The admin has a **👥 Users** button to list approved users and
  revoke access.
- **Telegram file forwarding** — forward any document/video/audio/photo to the
  bot and it downloads it (through the local server, up to **2 GB**), then
  runs it through the same organizer.
- **Crash-safe** — orphaned extraction stages are swept on startup;
  completed-but-unorganized archives are resumed; stale progress messages are
  cleaned up.
- **Resilient** — Telegram API errors (including rate-limit `429`) never take
  the bot down; progress views refresh every 5 s.

---

## How it works

```
Telegram  ──►  DownloadBot  ──►  aria2c (RPC)  ──►  downloads/  ──►  organizer ──►  media library
                   │                                                                    ▲
                   ├────────►  yt-dlp ───────────────────────────────────────────────────┤
                   │                                                                    │
                   └────────►  local Bot API server (optional) ──► temp store ──────────┘
```

- The bot talks to aria2 over its **websocket RPC** (`aria2-server` +
  `aria2-key`).
- yt-dlp is shelled out for supported video sites.
- For files forwarded **to** the bot, an optional **self-hosted Bot API
  server** removes Telegram's 20 MB bot download cap and enables up to 2 GB.

---

## Requirements

| Component | Why | Mandatory |
|-----------|-----|-----------|
| `aria2c` | the actual downloader (RPC) | **yes** |
| Telegram bot token | bot login | **yes** |
| Numeric Telegram user id | the bot admin | **yes** |
| `yt-dlp` | video-page URLs (YouTube, …) | no |
| `ffmpeg` | yt-dlp merge/embed | no (needed for yt-dlp) |
| `7z` / `unrar` / `unzip` | archive extraction fallbacks | no |
| `telegram-bot-api` + api-id/api-hash | forwarded files > 20 MB | no |

Supported platforms: **Linux `amd64` and `arm64`** (prebuilt binaries and CI).

---

## Quick start (prebuilt)

1. Grab the binary for your arch from the
   [**Releases**](https://github.com/spaulll/prox-download-bot/releases):

   | File | Platform |
   |------|----------|
   | `DownloadBot-linux-amd64` | Linux x86-64 (most VPS/NAS) |
   | `DownloadBot-linux-arm64` | Linux ARM 64-bit (Raspberry Pi 4/5 64-bit OS, ARM VPS) |

2. Install runtime tools, start aria2, write `config.json` — follow
   [Full setup](#full-setup-step-by-step).
3. Run:

   ```bash
   chmod +x DownloadBot-linux-amd64
   ./DownloadBot-linux-amd64 -c ./config.json
   ```

The binary embeds its English translations — no sidecar files required.

---

## Full setup, step by step

### 1. Install runtime tools

**Debian / Ubuntu / Raspberry Pi OS:**

```bash
sudo apt update
sudo apt install -y aria2 ffmpeg unzip p7zip-full
# unrar is in non-free (Debian) / multiverse (Ubuntu):
sudo apt install -y unrar || true
```

Install **yt-dlp** from its standalone release (the apt package is outdated):

```bash
# amd64
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux \
  -o /usr/local/bin/yt-dlp
# arm64
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux_aarch64 \
  -o /usr/local/bin/yt-dlp
sudo chmod +x /usr/local/bin/yt-dlp
```

### 2. Start aria2 with RPC

`aria2c` is the downloader. It must expose the RPC interface the bot uses:

```bash
aria2c \
  --enable-rpc --rpc-listen-all --rpc-listen-port=6800 \
  --rpc-secret=CHANGE_ME_SECRET \
  --dir=/mnt/nas/downloads \
  --continue=true --max-concurrent-downloads=5 \
  --max-connection-per-server=16 --split=16 \
  --file-allocation=none
```

- `--dir` **must equal** `downloadFolder` in `config.json`.
- Remember `--rpc-secret` → it becomes `aria2-key`.

**systemd unit** (`/etc/systemd/system/aria2c.service`):

```ini
[Unit]
Description=Aria2 download daemon
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/bin/aria2c --enable-rpc --rpc-listen-all --rpc-listen-port=6800 \
  --rpc-secret=CHANGE_ME_SECRET --dir=/mnt/nas/downloads \
  --continue=true --max-concurrent-downloads=5 \
  --max-connection-per-server=16 --split=16 \
  --file-allocation=none --enable-dht=true --seed-time=0
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now aria2c
curl -s -X POST http://127.0.0.1:6800/jsonrpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":["token:CHANGE_ME_SECRET"]}'
```

### 3. Create the Telegram bot

1. Talk to [@BotFather](https://t.me/BotFather) → `/newbot` → copy the **token**
   (`123456789:AA…`).
2. Get your **numeric user id** from [@userinfobot](https://t.me/userinfobot).
3. That id is the **admin** (`output.telegram.user-id`) — exactly one id,
   enforced at startup.

### 4. Optional: local Bot API server (files up to 2 GB)

Telegram's cloud Bot API only lets a bot download files **up to 20 MB**.
Running a **local Bot API server** removes that cap (up to 2 GB) and is
required for forwarding big files to the bot.

**You need** `api-id` + `api-hash` from <https://my.telegram.org> → *API
development tools*. They identify your **application** — they do **not** have
to come from the account that created the bot; any valid app credentials work.

Build `telegram-bot-api` (on a machine with a toolchain):

```bash
sudo apt install -y cmake g++ make libssl-dev zlib1g-dev gperf
git clone --recursive https://github.com/tdlib/telegram-bot-api.git
cd telegram-bot-api
mkdir build && cd build
cmake -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX:PATH=.. ..
cmake --build . --target install -j"$(nproc)"
# binary lands at ../bin/telegram-bot-api
```

Put the binary next to the bot as `telegram-bot-api`, and use the bundled
launcher `run-tg-api.sh` (reads credentials from `config.json`, passes them
via environment, runs in `--local` mode):

```bash
install -m755 telegram-bot-api /opt/prox-download-bot/telegram-bot-api
install -m755 run-tg-api.sh     /opt/prox-download-bot/run-tg-api.sh
```

Add to `config.json` (see next step): `api-id`, `api-hash`,
`api-base: "http://127.0.0.1:8081"`, `api-dir: "/mnt/nas/.tg-bot-api"`.

> **Important:** each running server needs its **own** `api-dir`. If two
> machines share the same storage, give each a distinct directory
> (`/mnt/nas/.tg-bot-api` vs `/mnt/nas/.tg-bot-api-prod`), otherwise the
> server aborts with *“Can't lock file … tqueue.binlog”*.
>
> Tip: use a directory on the **same filesystem as `downloadFolder`** so the
> bot can hardlink fetched files (instant, no extra disk).

### 5. Create `config.json`

```bash
cp default.config.json config.json
```

Minimal config (cloud Bot API, ≤ 20 MB forwarded files):

```json
{
  "input": {
    "aria2": {
      "aria2-server": "ws://127.0.0.1:6800/jsonrpc",
      "aria2-key": "CHANGE_ME_SECRET"
    }
  },
  "output": {
    "telegram": {
      "bot-key": "123456789:AA-your-bot-token",
      "user-id": "123456789"
    }
  },
  "max-index": 10,
  "language": "en",
  "downloadFolder": "/mnt/nas/downloads",
  "organize": {
    "enabled": true,
    "anilist": true,
    "deleteArchive": false,
    "movies": "/mnt/nas/movies",
    "series": "/mnt/nas/series",
    "anime": "/mnt/nas/anime",
    "music": "/mnt/nas/music",
    "documents": "/mnt/nas/documents",
    "archives": "/mnt/nas/archives",
    "others": "/mnt/nas/others",
    "torrents": "/mnt/nas/torrents",
    "keepTorrent": true,
    "youtube": "/mnt/nas/YouTube",
    "services": "/mnt/nas/Services",
    "ytdlpPath": "yt-dlp",
    "ytdlpQuality": "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best",
    "ytdlpEmbed": true
  },
  "log": { "logPath": "", "errPath": "", "level": "info" }
}
```

With the local Bot API server enabled (files up to 2 GB), add:

```json
      "api-id": 1234567,
      "api-hash": "0123456789abcdef0123456789abcdef",
      "api-base": "http://127.0.0.1:8081",
      "api-dir": "/mnt/nas/.tg-bot-api"
```

- `api-id`/`api-hash` — from my.telegram.org (see step 4).
- `api-base` — leave empty to use Telegram cloud (20 MB cap).
- `api-dir` — the local server's data directory; **required for byte-level
  progress** while fetching forwarded files.

> `config.json`, `users.json`, `tasks.json` and the binaries are git-ignored —
> **never commit them** (they contain secrets).

### 6. Create the folders

```bash
sudo mkdir -p /mnt/nas/downloads \
  /mnt/nas/{movies,series,anime,music,documents,archives,others,torrents,YouTube,Services}
```

Adjust to the paths used in `config.json`.

### 7. Build or download the bot

Use a release binary (`DownloadBot-linux-amd64` / `-arm64`) or build from
source — see [Building from source](#building-from-source).

Install it (example layout `/opt/prox-download-bot`):

```bash
sudo install -m755 DownloadBot-linux-amd64 /opt/prox-download-bot/DownloadBot
sudo cp config.json /opt/prox-download-bot/config.json
```

### 8. Run it as systemd services

**Bot API server** (`/etc/systemd/system/tg-bot-api.service`) — only if using
the local server from step 4:

```ini
[Unit]
Description=Telegram Bot API server (local mode, prox-download-bot backend)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/prox-download-bot
ExecStart=/opt/prox-download-bot/run-tg-api.sh
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

**The bot** (`/etc/systemd/system/downloadbot.service`):

```ini
[Unit]
Description=prox-download-bot (Telegram Aria2 bot)
After=network-online.target aria2c.service tg-bot-api.service
Wants=network-online.target
Requires=aria2c.service tg-bot-api.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/prox-download-bot
ExecStart=/opt/prox-download-bot/DownloadBot -c /opt/prox-download-bot/config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Drop the `tg-bot-api.service` entries from `After=`/`Requires=` if you didn't
set up the local server. Then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now aria2c tg-bot-api downloadbot   # omit tg-bot-api if unused
systemctl is-active aria2c downloadbot
journalctl -u downloadbot -n 20 --no-pager
```

Healthy log:

```
Configuration information loading completed!
using local Bot API server at http://127.0.0.1:8081     # only with api-base
Connecting to ws://127.0.0.1:6800/jsonrpc
Aria2 websocket connection established! Aria2 version 1.37.0
Login to authorized account YourBot
```

### 9. First use in Telegram

1. Open your bot, send `/start`.
2. **Admin:** the control keyboard appears (see below).
3. **Others:** they get a “request sent” message and the admin receives
   **✅ Approve / ⛔ Deny** buttons. After approval they `/start` again.
4. Send a link, a magnet/torrent, a video-page URL, or forward a file.

**Keyboard:**
```
⬇️ Downloading   ⌛️ Waiting   ✅ Finished/Stopped
⏸️ Pause task     ▶️ Resume task   ❌ Remove task
👥 Users                                            (admin only)
```

- **⬇️ Downloading** opens a live list of *your* active tasks (admin: all).
- **❌ Remove task** lists only **active** items (aria2 + in-progress Telegram
  fetches); finished items live in **✅ Finished/Stopped**.
- **👥 Users** (admin) lists approved users with a **❌ Remove** button that
  revokes access.

---

## Configuration reference

| Key | Description |
|-----|-------------|
| `input.aria2.aria2-server` | aria2 websocket RPC endpoint, e.g. `ws://127.0.0.1:6800/jsonrpc` |
| `input.aria2.aria2-key` | aria2 `--rpc-secret` |
| `output.telegram.bot-key` | bot token from @BotFather |
| `output.telegram.user-id` | admin's numeric Telegram id (exactly one) |
| `output.telegram.api-id` | my.telegram.org app id (local Bot API server; `0` = cloud) |
| `output.telegram.api-hash` | my.telegram.org app hash |
| `output.telegram.api-base` | local server base URL, e.g. `http://127.0.0.1:8081`; empty = cloud (20 MB cap) |
| `output.telegram.api-dir` | local server data dir (enables byte-level progress for forwarded files) |
| `max-index` | max items shown in waiting/finished lists |
| `language` | UI language (`en`) |
| `downloadFolder` | aria2 download directory (must match `--dir`) |
| `organize.enabled` | enable post-download organizing |
| `organize.anilist` | AniList anime confirmation (default on) |
| `organize.deleteArchive` | delete archives after successful extraction |
| `organize.movies` … `organize.others` | media library roots |
| `organize.torrents` | `.torrent` storage (default `<downloadFolder>/torrents`) |
| `organize.keepTorrent` | keep `.torrent` after its content is organized (default `true`) |
| `organize.youtube` | base for YouTube downloads (default `<downloadFolder>/YouTube`) |
| `organize.services` | base for other yt-dlp services (default `<downloadFolder>/Services`) |
| `organize.ytdlpPath` | yt-dlp binary path/name |
| `organize.ytdlpQuality` | yt-dlp format selector |
| `organize.ytdlpCookies` | cookies.txt path for yt-dlp |
| `organize.ytdlpProxy` | proxy URL for yt-dlp |
| `organize.ytdlpEmbed` | embed metadata + thumbnail (needs ffmpeg) |
| `log.logPath` / `log.errPath` | log files (empty = stdout only) |
| `log.level` | `debug` \| `info` \| `warn` \| `error` |

---

## Media library layout

```
movies
├── Some Movie (2024).mkv
└── Title (Year)/
    └── Title (Year).mkv          ← torrent/release folder grouping

series
└── Some Show
    └── Season 2
        └── Some.Show.S02E04.mkv

anime
└── Anime Show
    └── Season 3
        └── Anime.Show.S03E01.mkv

music / documents / archives / others / torrents
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

---

## Message style

All progress uses the same 13-segment bar and refreshes every 5 seconds.

**aria2 download (auto-appears / via ⬇️ Downloading):**
```
⬇️ Downloading
[●●●●●●●○○○○○○] 42.69 %
Downloaded: 607.06 MB of 1.39 GB
Speed: 8.80 MB/s
ETA: 4m 12s
Threads: 16
GID: d829eeca5dc91475
```

**Forwarded Telegram file:**
```
⬇️ Downloading

Filename: Some.Movie.2024.1080p.mkv
[●●○○○○○○○○○○○] 18.40 %
Downloaded: 229.50 MB of 1.22 GB
Speed: 5.85 MB/s ETA: 3m 22s
GID: tg1789384326596837985
```

**Organize / extraction:**
```
🗂 Organizing...

📦 Extracting
[●●●○○○○○○○○○○] 23.49 %
Extracted: 2.23 GB of 9.50 GB
ETA: 6m 41s
3/11 files
```

**Final summary:**
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
        └── Some.Show.S01E10.mkv

📦 Size: 2.59 GB
⏱ Time taken: 2m 0s
```

---

## Multi-user model

- `output.telegram.user-id` is the **admin**.
- New user → `/start` → admin gets **Approve / Deny** buttons.
- Approved users can download; denied users may `/start` again to re-request.
- **Visibility:** regular users see only their own tasks and progress;
  the admin sees all users' tasks.
- **Control:** users can pause/resume/remove only their own tasks; the admin
  can control everything. Bulk pause/resume affects only your own tasks
  (admin: all).
- **Users management:** admin **👥 Users** → lists approved users → **❌ Remove**
  revokes access (they can re-request later).

State lives in `users.json` (roles) and `tasks.json` (ownership), both
git-ignored.

---

## Telegram file forwarding & the 20 MB limit

Forward/send any **document, video, audio, GIF, voice note, video note or
photo** to the bot and it downloads through the same pipeline as links.

| Setup | Max file size |
|-------|---------------|
| Telegram **cloud** Bot API (no `api-base`) | **20 MB** (Telegram's bot limit) |
| **Local Bot API server** (`api-base` set) | **up to 2 GB** |

With the local server, the bot:
1. asks the server to fetch the file (progress is read from the server's
   `*/temp/` store — real percent, speed and ETA);
2. hardlinks the finished file into `downloadFolder` (instant, no extra disk);
3. deletes the server-side cache copy;
4. runs the normal organizer.

**Notes**
- Repeatedly forwarding the *same* file is instant (server cache).
- Byte-level progress requires `api-dir` to point at the server's data dir;
  without it you get an indeterminate “Fetching…” spinner, never wrong numbers.
- Cancelling a fetch stops the bot's processing; the server may still finish
  fetching in the background.

---

## Building from source

Requirements: **Go 1.24+**, vendored deps (`-mod=vendor`).

```bash
git clone https://github.com/spaulll/prox-download-bot.git
cd prox-download-bot
go build -mod=vendor -trimpath -ldflags "-s -w" -o DownloadBot ./cmd/DownloadBot
./DownloadBot -c ./config.json
```

Cross-compile (supported targets):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -ldflags "-s -w" -o DownloadBot-linux-amd64 ./cmd/DownloadBot
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -trimpath -ldflags "-s -w" -o DownloadBot-linux-arm64 ./cmd/DownloadBot
```

Tests:

```bash
go test ./internal/... ./tool/output/...
```

Releases are produced by CI on `v*` tags — see
[`.github/workflows/build.yml`](.github/workflows/build.yml) (only
`linux/amd64` and `linux/arm64` are built).

---

## Updating

```bash
# from source
git pull
go build -mod=vendor -trimpath -ldflags "-s -w" -o DownloadBot ./cmd/DownloadBot
sudo systemctl restart downloadbot      # add tg-bot-api if you self-host the API server
```

If the config or an aria2-side setting changed, restart the affected units:

```bash
sudo systemctl restart aria2c downloadbot
```

---

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| Bot exits with `Aria2 RPC connection failed` | aria2 not running / wrong `aria2-server` or `aria2-key` |
| `output.telegram.user-id must be a single numeric Telegram ID` | set exactly one numeric id |
| Telegram errors, bot restarts | now non-fatal; check `journalctl -u downloadbot` for details |
| `429 Too Many Requests` | rate limit; the bot backs off automatically (updates every 5 s) |
| Forwarded file > 20 MB fails | set up the [local Bot API server](#4-optional-local-bot-api-server-files-up-to-2-gb) |
| Server log: `Can't lock file … tqueue.binlog` | another instance uses the same `api-dir`; give each its own |
| Forwarded-file progress stuck on “Fetching…” | `api-dir` wrong/missing — must point at the server's data dir |
| yt-dlp failures | install `yt-dlp` + `ffmpeg`, or set `organize.ytdlpPath` |
| Archives fail | install `7z`/`unrar`; zip/tar are native |
| Nothing organizes | `organize.enabled` false, or library paths not writable |
| Torrent picker / magnet silent | dead magnet or no metadata; check `journalctl -u downloadbot` |

Logs:

```bash
journalctl -u downloadbot -n 50 --no-pager
journalctl -u tg-bot-api -n 50 --no-pager    # if self-hosting the API server
journalctl -u aria2c   -n 50 --no-pager
```

---

## Security notes

- `config.json` holds the bot token and API credentials — keep it `0600`,
  never commit it.
- `users.json` / `tasks.json` contain user ids — also git-ignored.
- If a token ever leaks (logs, screenshots, chat), revoke it via @BotFather
  and update `bot-key`, then `systemctl restart downloadbot`.
- The local Bot API server runs as root by default in the sample units; use a
  dedicated user with access to the download/library directories if you prefer.

---

## Credits & license

- **Original project:** [DownloadBot](https://github.com/gaowanliang/DownloadBot)
  by [gaowanliang (Gaowan Liang)](https://github.com/gaowanliang) — Apache-2.0.
- This repository is a heavily reworked derivative: English-only UI, native Go
  media organization, archive extraction, yt-dlp support, multi-user
  management, Telegram-file forwarding and a self-hosted Bot API integration.
- Optional self-hosted API server uses
  [tdlib/telegram-bot-api](https://github.com/tdlib/telegram-bot-api).
- Licensed under **Apache-2.0** — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
