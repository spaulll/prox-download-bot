# Prox Download Bot – Remod Plan

**Original project:** https://github.com/gaowanliang/DownloadBot  
**New upstream:** https://github.com/spaulll/prox-download-bot  
**Git user:** spaulll  

**License:** Apache-2.0  
→ Must retain original copyright notices and give proper credit to **gaowanliang (Gaowan Liang)**

**Goal:** Clean English-only Telegram bot that controls Aria2, with native AriaFlow-style media organization + accurate progress bars + yt-dlp (1080p + best audio).

Work in phases. Update todos after every task. Short keyword-style commits.

---

## Phase 0 – Repo Reset & Clean
**Goal:** Fresh history, zero Chinese/Taiwanese content, no old docs/imgs.

### Todo
- [x] Clone / use existing DownloadBot source
- [x] `rm -rf .git`
- [x] Delete: `README.md`, `docs/`, `img/`, unnecessary files
- [x] Strip **all** Chinese / Mandarin / Traditional Chinese comments, strings, and i18n packs (keep English only)
- [x] Remove cloud-upload related code (OneDrive, Google Drive, Mega, etc.)
- [x] Keep `LICENSE` (Apache-2.0) and add proper attribution
- [x] `git init`
- [x] `git config user.name "spaulll"`
- [x] `git remote add origin https://github.com/spaulll/prox-download-bot.git`
- [x] First commit: `init: clean slate from DownloadBot`

**Commit keywords:** `init:`, `chore: remove chinese`, `chore: drop cloud upload`, `chore: drop docs imgs`

---

## Phase 1 – Core Stability (English-only base)
**Goal:** Project builds and runs cleanly with English UI/logs only.

### Todo
- [x] Fix imports / build after cleanup
- [x] Modernize / simplify config (`default.config.json`)
- [x] Verify Aria2 websocket control still works
- [x] Ensure all Telegram messages are English
- [x] Smoke test: start bot → add download → see status
- [x] Commit: `fix: english only base`

---

## Phase 2 – Native File Management (exact AriaFlow parity)
**Goal:** Post-download organization identical to AriaFlow + accurate real-time progress bars.

**Reference:** https://github.com/spaulll/ariaflow (`organize_download.sh`)

### Features (native Go)
- Sort by type → `movies / series / anime / music / documents / archives / others`
- Episode detection: `S01E01`, `S1E1`, `1x01`, `s01.e01`, etc.
- Season extraction + create `Season N` folders
- Single-episode handling (never treat as full season pack)
- Smart folder matching (normalize + token overlap)
- AniList lookup for anime confirmation (default **on**, configurable off)
- Exact same directory structure as AriaFlow
- Accurate progress bar updates to Telegram during organize

### Todo
- [ ] Create `internal/organize/` package
- [ ] Port normalize / sanitize / extract_season / is_episode
- [ ] Port find_series_folder / find_anime_folder
- [ ] Category router
- [ ] Hook into Aria2 download-complete event
- [ ] Configurable library base path
- [ ] Real-time progress reporting (percent + current step)
- [ ] Logging of every decision / move
- [ ] Unit tests for name parsing & matching
- [ ] Commits:  
  `feat: organize core`  
  `feat: series season detect`  
  `feat: anime anilist`  
  `feat: progress bar organize`

---

## Phase 3 – Archive Extraction + Re-analysis
**Goal:** Extract archives, then re-run organize logic. Handle slow extractions with progress.

**Supported:** zip, rar, 7z, tar, gz, bz2, xz

### Todo
- [ ] Safe extraction (prefer external tools or solid Go libs)
- [ ] Detect archive on download complete
- [ ] Extract to temp with live progress updates to Telegram
- [ ] After extract → full organize pipeline on contents
- [ ] Optional: delete original archive (config)
- [ ] Handle nested / multi-volume archives
- [ ] Timeout + error handling for large/slow extracts
- [ ] Commits: `feat: archive extract` / `fix: slow extract progress`

---

## Phase 4 – yt-dlp Integration
**Goal:** Support **all** sites that yt-dlp supports (not only YouTube). Download at 1080p + best audio with metadata, then organize into a dedicated structure.

### Storage rules for yt-dlp downloads

**YouTube:**
```
/YouTube
└── Channel Name
    ├── video.mp4                          ← single video
    └── Playlist Name                      ← if from a playlist
        ├── video1.mp4
        └── video2.mp4
```

**Other yt-dlp supported sites** (Pornhub, Twitter, Instagram, Bilibili, etc.):
```
/ServiceName                               ← e.g. PornHub, Twitter, Instagram
└── Channel / Uploader / User
    └── video.mp4
```
(or with playlist subfolder when applicable)

- Direct download links (http/ftp/magnet/torrent) → still handled by Aria2 + normal organize pipeline
- Anything yt-dlp can handle → use the structure above

### Additional requirements
- Must safely handle **any special characters** in filenames, channel names, playlist names, etc. (sanitize for filesystem)
- Default quality: 1080p video + best audio + embed metadata/thumbnail
- Live progress bar while downloading
- After download → place into the structure above (do **not** run through movies/series/anime organizer)

### Todo
- [ ] Integrate yt-dlp (binary or Go wrapper)
- [ ] Detect yt-dlp-compatible links vs Aria2 links
- [ ] Handler for all yt-dlp supported sites
- [ ] Default: 1080p + best audio + embed metadata/thumbnail
- [ ] Live progress bar from yt-dlp → Telegram
- [ ] Build correct folder structure (YouTube vs other services)
- [ ] Sanitize all special characters in names
- [ ] Config: quality, embed, cookies, proxy, base paths, etc.
- [ ] Commits: `feat: ytdlp` / `feat: ytdlp progress` / `feat: ytdlp structure` / `feat: ytdlp sanitize`

---

## Phase 5 – Polish & Credits
**Goal:** Clean, attributed, production-ready.

### Todo
- [ ] New English-only `README.md`
- [ ] Proper credits section (Apache-2.0 compliant):
  - Original author: gaowanliang (Gaowan Liang)
  - Original repo link
  - Note that this is a heavy remod / derivative
- [ ] Final config cleanup (AniList toggle, library paths, ytdlp defaults, etc.)
- [ ] Progress bar consistency across all operations
- [ ] Final commit: `docs: readme + credits` / `chore: release ready`

---

## Phase 6 – Multi-User Support
**Goal:** Proper multi-user system with admin approval and visibility control.

### Rules
- The `user-id` in `config.json` is the **Admin**.
- When a new user starts the bot (`/start`):
  - Admin receives a message containing:
    - User ID
    - Username
    - Full name
  - Inline buttons: **Approve** / **Deny**
- Only approved users can use the bot.
- When any approved user adds a link/task:
  - The user sees normal progress for their own task
  - Admin also receives a notification, e.g.:
    ```
    👤 @username added a task
    🔗 https://...
    ```
- Visibility:
  - Regular users → can only see **their own** tasks and progress
  - Admin → can see **all** users’ tasks and history

### Todo
- [ ] Store approved users (file or simple DB)
- [ ] On `/start` → notify admin with Approve / Deny buttons
- [ ] Handle Approve / Deny callback
- [ ] Block unapproved users
- [ ] When user adds a task → notify admin
- [ ] Scope task lists / progress / history by user (admin sees everything)
- [ ] Commits: `feat: multiuser` / `feat: admin approve` / `feat: task visibility`

---

## Bot UI / Message Style (MANDATORY)

The agent **must** follow these exact message patterns.  
All messages must look clean and properly aligned on Telegram.

### Progress bar style (required)
```
▰▰▰▰▰▰▰▱▱▱ 70%
```

### Tree style (required)
```
├── 
└── 
```

### Filename cleaning rule (MANDATORY)

For **movies / series / anime**:

1. **Show / Movie folder name** → Clean Title Case with spaces  
   Example: `Some.Show` → `Some Show`

2. **Season folder** → `Season 2` (no leading zero)

3. **Episode / Movie file name** → Keep only clean short form  
   Remove: resolution, codec, language, group, site tags, etc.

   Example:
   ```
   Original:  Some.Show.S02E04.1080p.x264.Hindi.Korean.English.Msubs.RG.mkv
   Stored as: Some.Show.S02E04.mkv
   ```

   Final path example:
   ```
   Series
   └── Some Show
       └── Season 2
           └── Some.Show.S02E04.mkv
   ```

This rule applies to both extracted files and single unhandled episode files.

### Message flow examples (must match this style)

**Download completed**
```
✅ Download completed

Mousetrap.S01.480p.x264.Hindi.Korean.English.Msubs.RG.zip
```

**Archive detected**
```
🗂 Organizing...

📦 Archive detected
→ Preparing extraction
```

**Extraction progress (live)**
```
🗂 Organizing...

📦 Extracting
▰▰▰▰▰▰▰▱▱▱ 70%
7/10 files
```

**Analyzing + moving (live)**
```
🗂 Organizing...

🔍 Analyzing content
→ TV Series • Season 1 detected

📂 Moving files
▰▰▰▰▰▰▰▰▱▱ 80%
4/5 episodes
```

**Final detailed summary (required style)**
```
✅ All done!

📦 Archive processed
Mousetrap.S01.480p.x264.Hindi.Korean.English.Msubs.RG.zip

🗂 Result
• 5 episodes moved
• 0 failed

Series
└── Mousetrap
    └── Season 1
        ├── Mousetrap.S01E01.mkv
        ├── Mousetrap.S01E02.mkv
        ├── Mousetrap.S01E03.mkv
        ├── Mousetrap.S01E04.mkv
        └── Mousetrap.S01E05.mkv

📦 Size: 2.4 GB
⏱ Time taken: 1m 42s
```

**Another example (long release name):**
```
Original file: Some.Show.S02E04.1080p.x264.Hindi.Korean.English.Msubs.RG.mkv

Final path:
Series
└── Some Show
    └── Season 2
        └── Some.Show.S02E04.mkv
```

### UI Rules
- Keep messages clean and well-aligned on Telegram
- Use the exact progress bar and tree styles above
- Always include size + time taken on the final message
- Use emojis consistently as shown
- Prefer short, readable lines
- Never show full long release names for media files after cleaning

---

## Working Rules
- Update this PLAN.md todos after every completed task
- Short commits with clear keywords (`feat:`, `fix:`, `chore:`, `refactor:`)
- Prefer pure Go when reasonable
- Progress bars must be accurate and update in real time
- Zero non-English text left in code/comments/UI
- Always keep Apache-2.0 LICENSE + original attribution notices
- **All bot messages must strictly follow the UI style defined above**
- **Must safely handle any special characters** in all filenames, folder names, channel names, playlist names, etc. (sanitize for the filesystem)

---

## Locked Decisions
| Topic              | Decision                                                                 |
|--------------------|--------------------------------------------------------------------------|
| Cloud uploads      | Dropped                                                                  |
| Organize logic     | Exact AriaFlow parity                                                    |
| Folder structure   | Exact AriaFlow (`movies/series/anime/...`) for Aria2 downloads           |
| AniList            | Default **on**, config can disable                                       |
| yt-dlp quality     | 1080p + best audio + metadata                                            |
| yt-dlp structure   | `/YouTube/Channel/[Playlist/]video` or `/ServiceName/...` for others     |
| Special characters | Must be sanitized safely in all names                                    |
| Multi-user         | Admin approval required, users see only own tasks, admin sees all        |
| Credits            | Full proper attribution to gaowanliang                                   |
| Bot UI             | Exact style defined in this plan                                         |

---

Update this file as you complete each phase.
```
