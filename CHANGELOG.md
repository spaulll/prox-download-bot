# Changelog

All notable changes to this project are documented in this file.

## v0.4.0 - 2026-09-14

### Added
- **Forward/send Telegram files** to the bot (documents, video, audio, GIF,
  voice notes, video notes, photos). They ride the exact same pipeline as
  links: owner + admin progress, then automatic organizing by file type.
- **Local Bot API server integration** — `output.telegram.api-id`,
  `api-hash`, `api-base`, `api-dir`, a portable `run-tg-api.sh` launcher and
  a `tg-bot-api.service` unit. This raises the forwarded-file limit from
  Telegram's **20 MB** bot cap to **2 GB**.
- **Real byte-level progress** for forwarded-file fetches (percent, speed,
  ETA), read from the API server's temp store. Attribution is strict — a
  fetch only shows numbers when exactly one file is growing — so concurrent
  downloads never display each other's bytes (worst case: an honest spinner).
- **One-shot setup script** `setup.sh` with an interactive mode (`-i`):
  installs dependencies, writes `config.json`, creates folders, installs and
  enables systemd units, and can build/install the Bot API server. The local
  API server is **off by default** and enabled via a `[N/y]` prompt.

### Changed
- Progress views now refresh every **5 seconds** (was every second).
- **❌ Remove task** lists only *active* (removable) tasks; finished downloads
  remain in **✅ Finished/Stopped** history.
- Telegram API errors (including rate-limit `429`) are **non-fatal** — the bot
  backs off using the server's retry hint instead of exiting.
- Downloaded copies of forwarded files are removed from the API server cache
  after staging, so large files no longer occupy disk twice.
- `output.telegram.user-id` must be a single numeric id (enforced at startup).

### Fixed
- Forwarded files showed a stuck `0 %` bar, or no progress at all.
- `getFile` blocks until the local server finishes downloading a file; the
  call now runs in the background so live progress can drive the bar.
- Bot could crash with `429 Too Many Requests` during heavy progress updates.
- Duplicate/parallel live progress messages for a single download.
- Markdown escaping: usernames containing `_` and filenames inside code spans
  are handled correctly (no more rejected messages).
- Stale temp copies left behind by interrupted fetches.
- Stored admin roles from older configs are demoted to approved at startup, so
  the **👥 Users** list shows everyone and revoke works.

### Removed
- **All prebuilt targets except `linux/amd64` and `linux/arm64`.**
  Dropped: `linux/armv7`, `linux/386`, `windows/amd64`, `darwin/amd64`,
  `darwin/arm64`.
  **Why:** this bot is built for Linux servers and NAS boxes — it drives
  `aria2` over RPC, runs under `systemd`, and (for files over 20 MB) runs the
  Linux `tdlib` Bot API server alongside it. The other targets were compiled
  by CI but never supported or tested, so shipping them was misleading. CI now
  builds only the two targets the project actually targets. (`windows/arm64`
  was already impossible: the vendored `go-ole` via `gopsutil` supports only
  `windows/386` and `windows/amd64`.)

## v0.3.0 - 2026-09-14

### Added
- Live download/organize progress is mirrored to both the task owner and the
  admin (previously admin-only)
- Torrent/magnet file picker is shown to the owner plus all admins
- Admin **👥 Users** button: lists approved users with a per-user Remove
  button that revokes access (re-request via `/start` still works)
- Personal live view: the **⬇️ Downloading** button shows regular users only
  their own tasks, admins see everything
- Pause-all / resume-all for regular users now affects only their own tasks

### Fixed
- Users adding a link saw no feedback at all (every reply went to the admin
  chat); start/pause/error notices now reach the task owner too
- Crash-safe in-flight message tracking now supports multiple chats
- Uploaded `.torrent` files use per-user temp files and record ownership, so
  concurrent uploads no longer clash

## v0.2.1 - 2026-09-04

### Removed
- **Delete files from the download folder** button
- **Move files in the download folder** button (and all unused file-control plumbing)

### Changed
- Releases now include this changelog

## v0.2.0 - 2026-09-04

### Added
- Torrent releases are grouped into a dedicated folder inside their category
  (`Movies/Title (Year)/` for movies, existing series/anime layout for shows)
- `.torrent` files sent to the bot are stored in a dedicated torrents folder
- `organize.torrents` config key (default `<downloadFolder>/torrents`)
- `organize.keepTorrent` config key: keep (default) or delete the `.torrent`
  file after its content is organized
- Sidecar files (subtitles, `.nfo`, artwork, release notes) now follow their
  video instead of being sorted away
- Release-file support: no-year names produce `Title` folders

### Fixed
- Torrent file-picker no longer crashes on single-file torrents, empty
  metadata or stale buttons
- Magnet metadata pseudo-downloads no longer spam "Download completed" +
  "Organize failed" messages
- Torrent picker auto-pause no longer sends a "paused" notification
- `.torrent` files generated by magnet links no longer linger in the download
  folder root
- Per-user task tracking works for magnet / .torrent-URL spawned downloads

## v0.1.1

- Initial tagged release of the English-only remod
- Aria2 control, media organization, archive extraction, yt-dlp support,
  multi-user admin approval, history with timestamps
