# prox-download-bot

English-only Telegram bot that controls **Aria2** with native AriaFlow-style
media organization, accurate real-time progress bars, and **yt-dlp** support
(any site yt-dlp handles, at 1080p + best audio).

> A heavy remod / derivative of [DownloadBot](https://github.com/gaowanliang/DownloadBot)
> by **gaowanliang (Gaowan Liang)** — licensed under Apache-2.0 with full
> attribution. See [NOTICE](NOTICE) and [LICENSE](LICENSE).

## Features

- **Aria2 control** — add http/ftp/magnet/torrent links via Telegram, live
  progress (`▰▰▰▰▰▰▰▱▱▱ 70%`), pause / resume / remove tasks
- **Native media organization** — completed downloads are sorted into a
  media library (AriaFlow directory parity):
  - `movies / series / anime / music / documents / archives / others`
  - Episode detection: `S01E01`, `S1E1`, `1x01`, `s01.e01`, ...
  - Season extraction → `Season N` folders (no leading zero)
  - Smart folder matching (normalize + token overlap)
  - AniList lookup for anime confirmation (default on, configurable)
  - Clean file names: `Some.Show.S02E04.1080p.x264.Hindi...mkv`
    → `Some.Show.S02E04.mkv`
- **Archive handling** — zip / rar / 7z / tar / gz / bz2 / xz are extracted
  (7z, unrar, unzip or native Go), then re-run through the organizer; season
  packs are detected and moved as a unit
- **yt-dlp integration** — every site yt-dlp supports (YouTube, Pornhub,
  Twitter/X, Instagram, Bilibili, ...):
  - 1080p video + best audio, metadata + thumbnail embedding
  - `YouTube/<Channel>/[<Playlist>/]<video>.mp4`
  - `<Service>/<Channel>/[<Playlist>/]<video>.mp4`
  - Live progress to Telegram
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
│   └── Attack on Titan
│       └── Season 4
│           └── Attack.on.Titan.S04E05.mkv
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

## Configuration reference

| Key | Description |
|-----|-------------|
| `downloadFolder` | Aria2 download directory |
| `organize.enabled` | Post-download organizing on/off |
| `organize.anilist` | AniList anime confirmation (default on) |
| `organize.deleteArchive` | Delete archives after successful extraction |
| `organize.movies/series/anime/music/documents/archives/others` | Library roots |
| `organize.youtube` | Base path for YouTube downloads |
| `organize.services` | Base path for other yt-dlp services |
| `organize.ytdlpPath` | yt-dlp binary path |
| `organize.ytdlpQuality` | yt-dlp format selector (default 1080p + best audio) |
| `organize.ytdlpCookies` | cookies.txt path for yt-dlp |
| `organize.ytdlpProxy` | proxy URL for yt-dlp |
| `organize.ytdlpEmbed` | embed metadata + thumbnail (needs ffmpeg) |

## Credits

- **Original project:** [DownloadBot](https://github.com/gaowanliang/DownloadBot)
  by [gaowanliang (Gaowan Liang)](https://github.com/gaowanliang) — Apache-2.0
- This repository is a heavily reworked derivative with an English-only UI,
  native Go media organization, archive extraction, and yt-dlp support.
- Aria2 RPC client based on the original project's vendored implementation.

## License

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
