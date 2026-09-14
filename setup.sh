#!/usr/bin/env bash
#
# prox-download-bot one-shot setup.
#
# Non-interactive: pass values via environment variables (see --help).
# Interactive:     ./setup.sh -i
#
set -euo pipefail

INTERACTIVE=0
ASSUME_YES=0
WITH_DEPS=ask
WITH_SYSTEMD=ask
START_NOW=ask
FORCE=0

INSTALL_DIR="${INSTALL_DIR:-/opt/prox-download-bot}"
DOWNLOAD_FOLDER="${DOWNLOAD_FOLDER:-/mnt/nas/downloads}"
LIBRARY_ROOT="${LIBRARY_ROOT:-/mnt/nas}"
ARIA2_SECRET="${ARIA2_SECRET:-}"
BOT_TOKEN="${BOT_TOKEN:-}"
ADMIN_ID="${ADMIN_ID:-}"
MAX_INDEX="${MAX_INDEX:-10}"
LANGUAGE="${LANGUAGE:-en}"
ORGANIZE_ENABLED="${ORGANIZE_ENABLED:-true}"
ANILIST="${ANILIST:-true}"
DELETE_ARCHIVE="${DELETE_ARCHIVE:-false}"
KEEP_TORRENT="${KEEP_TORRENT:-true}"
YTDLP_PATH="${YTDLP_PATH:-yt-dlp}"
YTDLP_QUALITY="${YTDLP_QUALITY:-bestvideo[height<=1080]+bestaudio/best[height<=1080]/best}"
YTDLP_EMBED="${YTDLP_EMBED:-true}"
WITH_API_SERVER="${WITH_API_SERVER:-ask}"
API_ID="${API_ID:-}"
API_HASH="${API_HASH:-}"
API_BASE="${API_BASE:-}"
API_DIR="${API_DIR:-}"
LOG_LEVEL="${LOG_LEVEL:-info}"
LOG_PATH="${LOG_PATH:-}"
ERR_PATH="${ERR_PATH:-}"

BOT_SOURCE="${BOT_SOURCE:-auto}"
API_SERVER_SOURCE="${API_SERVER_SOURCE:-auto}"
REPO_URL="${REPO_URL:-https://github.com/spaulll/prox-download-bot}"

C_RESET=$'\033[0m'; C_INFO=$'\033[36m'; C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_OK=$'\033[32m'

log()  { printf '%s%s%s\n' "$C_INFO" "$*" "$C_RESET"; }
ok()   { printf '%s%s%s\n' "$C_OK" "$*" "$C_RESET"; }
warn() { printf '%sWARN: %s%s\n' "$C_WARN" "$*" "$C_RESET" >&2; }
die()  { printf '%sERROR: %s%s\n' "$C_ERR" "$*" "$C_RESET" >&2; exit 1; }

usage() {
  cat <<'EOF'
prox-download-bot setup

Usage: sudo ./setup.sh [options]

Options:
  -i                 interactive mode (prompt for every value)
  -y                 assume "yes" for confirmations
  -f                 overwrite existing config.json / units without asking
  --install-dir DIR      install location            (default /opt/prox-download-bot)
  --download-folder DIR  aria2 download dir          (default /mnt/nas/downloads)
  --library-root DIR     media library root          (default /mnt/nas)
  --no-deps              do not install system packages
  --no-systemd           do not write/enable systemd units
  --no-start             do not start services
  --with-api-server      set up the local Bot API server (needs api-id/api-hash);
                         installs build deps and compiles telegram-bot-api if missing
  --no-api-server        never set up the local Bot API server (default)
  --bot-source SRC       auto | build | download | path:/abs/path
  --api-server-source S  auto | build | path:/abs/path
  --help                 this help

By default the local Bot API server is OFF (Telegram cloud, 20 MB file cap).
In interactive mode you get a [N/y] prompt; "y" installs the build packages and
compiles telegram-bot-api on this machine. A prebuilt binary already at
<install-dir>/telegram-bot-api is reused instead of recompiling.

Environment variables (non-interactive, same names as flags minus dashes):
  BOT_TOKEN ADMIN_ID ARIA2_SECRET INSTALL_DIR DOWNLOAD_FOLDER LIBRARY_ROOT
  MAX_INDEX LANGUAGE ORGANIZE_ENABLED ANILIST DELETE_ARCHIVE KEEP_TORRENT
  YTDLP_PATH YTDLP_QUALITY YTDLP_EMBED
  WITH_API_SERVER API_ID API_HASH API_BASE API_DIR
  LOG_LEVEL LOG_PATH ERR_PATH

Examples:
  sudo ./setup.sh -i
  sudo BOT_TOKEN=123:AA... ADMIN_ID=123456789 ARIA2_SECRET=s3cr3t ./setup.sh
  sudo BOT_TOKEN=... ADMIN_ID=... API_ID=1234 API_HASH=abc... \
       WITH_API_SERVER=yes API_DIR=/mnt/nas/.tg-bot-api ./setup.sh
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -i) INTERACTIVE=1 ;;
    -y) ASSUME_YES=1 ;;
    -f) FORCE=1 ;;
    --install-dir) INSTALL_DIR="$2"; shift ;;
    --download-folder) DOWNLOAD_FOLDER="$2"; shift ;;
    --library-root) LIBRARY_ROOT="$2"; shift ;;
    --no-deps) WITH_DEPS=no ;;
    --no-systemd) WITH_SYSTEMD=no ;;
    --no-start) START_NOW=no ;;
    --with-api-server) WITH_API_SERVER=yes ;;
    --no-api-server) WITH_API_SERVER=no ;;
    --bot-source) BOT_SOURCE="$2"; shift ;;
    --api-server-source) API_SERVER_SOURCE="$2"; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "unknown option: $1 (see --help)" ;;
  esac
  shift
done

[ "$(id -u)" = "0" ] || die "run as root (needed for packages/systemd): sudo $0 $*"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) die "unsupported architecture: $ARCH (only linux amd64/arm64 are supported)" ;;
esac
OS_ID="unknown"; [ -r /etc/os-release ] && OS_ID="$(. /etc/os-release && echo "$ID")"
log "platform: $OS_ID $(uname -m) -> linux/$GOARCH"

have() { command -v "$1" >/dev/null 2>&1; }
apt_install() {
  have apt-get || { warn "apt-get not found; install manually: $*"; return 0; }
  DEBIAN_FRONTEND=noninteractive apt-get update -qq || true
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$@"
}

ask() {
  local __var="$1" __prompt="$2" __default="${3-}" __val=""
  if [ "$INTERACTIVE" != 1 ]; then printf -v "$__var" '%s' "$__default"; return 0; fi
  if [ -n "$__default" ]; then
    read -r -p "$__prompt [$__default]: " __val || true
  else
    read -r -p "$__prompt: " __val || true
  fi
  [ -z "$__val" ] && __val="$__default"
  printf -v "$__var" '%s' "$__val"
}

ask_secret() {
  local __var="$1" __prompt="$2" __default="${3-}" __val=""
  if [ "$INTERACTIVE" != 1 ]; then printf -v "$__var" '%s' "$__default"; return 0; fi
  if [ -n "$__default" ]; then
    read -rs -p "$__prompt [hidden, Enter to keep current]: " __val || true; echo
  else
    read -rs -p "$__prompt: " __val || true; echo
  fi
  [ -z "$__val" ] && __val="$__default"
  printf -v "$__var" '%s' "$__val"
}

ask_yn() {
  local __prompt="$1" __default="$2" __val=""
  if [ "$INTERACTIVE" != 1 ] || [ "$ASSUME_YES" = 1 ]; then [ "$__default" = "yes" ]; return; fi
  local hint="y/N"; [ "$__default" = "yes" ] && hint="Y/n"
  read -r -p "$__prompt [$hint]: " __val || true
  __val="${__val,,}"
  if [ -z "$__val" ]; then [ "$__default" = "yes" ]; return; fi
  case "$__val" in y|yes) return 0 ;; *) return 1 ;; esac
}

confirm_overwrite() {
  local path="$1"
  [ -e "$path" ] || return 0
  [ "$FORCE" = 1 ] && return 0
  [ "$ASSUME_YES" = 1 ] && return 0
  if [ "$INTERACTIVE" != 1 ]; then
    warn "$path exists (keeping it; use -f to overwrite)"; return 1
  fi
  ask_yn "Overwrite $path?" no && return 0 || return 1
}

cfg_get() {
  local key="$1" file="$2" default="${3-}"
  [ -r "$file" ] || { printf '%s' "$default"; return; }
  python3 - "$file" "$key" "$default" <<'PY'
import json, sys
try:
    c = json.load(open(sys.argv[1]))
except Exception:
    print(sys.argv[3]); raise SystemExit
keys = sys.argv[2].split('.')
v = c
for k in keys:
    if isinstance(v, dict) and k in v:
        v = v[k]
    else:
        print(sys.argv[3]); raise SystemExit
if isinstance(v, bool):
    print("true" if v else "false")
else:
    print(v)
PY
}

collect_values() {
  local existing="$INSTALL_DIR/config.json"
  local existing_api=""
  if [ -r "$existing" ]; then
    BOT_TOKEN="${BOT_TOKEN:-$(cfg_get output.telegram.bot-key "$existing" "")}"
    ADMIN_ID="${ADMIN_ID:-$(cfg_get output.telegram.user-id "$existing" "")}"
    ARIA2_SECRET="${ARIA2_SECRET:-$(cfg_get input.aria2.aria2-key "$existing" "")}"
    API_ID="${API_ID:-$(cfg_get output.telegram.api-id "$existing" "")}"
    API_HASH="${API_HASH:-$(cfg_get output.telegram.api-hash "$existing" "")}"
    API_BASE="${API_BASE:-$(cfg_get output.telegram.api-base "$existing" "")}"
    API_DIR="${API_DIR:-$(cfg_get output.telegram.api-dir "$existing" "")}"
    [ -n "$API_BASE" ] && existing_api=yes
  fi

  ask INSTALL_DIR "Install directory" "$INSTALL_DIR"
  ask DOWNLOAD_FOLDER "aria2 download folder" "$DOWNLOAD_FOLDER"
  ask LIBRARY_ROOT "Media library root" "$LIBRARY_ROOT"
  ask BOT_TOKEN "Telegram bot token (from @BotFather)" "$BOT_TOKEN"
  ask ADMIN_ID "Admin numeric Telegram user id" "$ADMIN_ID"
  ask_secret ARIA2_SECRET "aria2 RPC secret" "$ARIA2_SECRET"
  ask MAX_INDEX "History/list size (max-index)" "$MAX_INDEX"
  ask LOG_LEVEL "Log level (debug|info|warn|error)" "$LOG_LEVEL"
  ask LANGUAGE "UI language" "$LANGUAGE"
  ask_yn "Enable post-download organizing?" yes && ORGANIZE_ENABLED=true || ORGANIZE_ENABLED=false
  if [ "$ORGANIZE_ENABLED" = true ]; then
    ask_yn "AniList anime confirmation?" yes && ANILIST=true || ANILIST=false
    ask_yn "Delete archives after successful extraction?" no && DELETE_ARCHIVE=true || DELETE_ARCHIVE=false
    ask_yn "Keep .torrent files after organizing?" yes && KEEP_TORRENT=true || KEEP_TORRENT=false
    ask YTDLP_PATH "yt-dlp binary (name or path)" "$YTDLP_PATH"
    ask_yn "Embed metadata/thumbnail with yt-dlp (needs ffmpeg)?" yes && YTDLP_EMBED=true || YTDLP_EMBED=false
  fi

  if [ "$WITH_API_SERVER" = ask ]; then
    if [ "$INTERACTIVE" = 1 ]; then
      ask_yn "Set up the local Bot API server (compiles telegram-bot-api, files up to 2 GB)?" "${existing_api:-no}" \
        && WITH_API_SERVER=yes || WITH_API_SERVER=no
    else
      WITH_API_SERVER=no
    fi
  fi
  if [ "$WITH_API_SERVER" = yes ]; then
    ask API_ID "Telegram api-id (my.telegram.org)" "${API_ID:-}"
    ask_secret API_HASH "Telegram api-hash (my.telegram.org)" "${API_HASH:-}"
    ask API_BASE "Bot API base URL" "${API_BASE:-http://127.0.0.1:8081}"
    [ -n "$API_DIR" ] || API_DIR="$LIBRARY_ROOT/.tg-bot-api"
    ask API_DIR "Bot API server data dir" "$API_DIR"
  else
    API_BASE=""; API_DIR=""
  fi

  [ -n "$BOT_TOKEN" ] || die "BOT_TOKEN is required"
  [ -n "$ADMIN_ID" ] || die "ADMIN_ID is required"
  case "$ADMIN_ID" in *[!0-9]*) die "ADMIN_ID must be numeric (got: $ADMIN_ID)" ;; esac
  if [ -z "$ARIA2_SECRET" ]; then
    ARIA2_SECRET="$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    log "generated aria2 secret"
  fi
  if [ "$WITH_API_SERVER" = yes ]; then
    if [ -z "$API_ID" ] || [ -z "$API_HASH" ]; then
      die "The local Bot API server needs Telegram API credentials.
  Get api-id and api-hash from https://my.telegram.org (API development tools),
  then re-run with API_ID=... API_HASH=... (or answer the prompts in -i mode)."
    fi
  fi
}

install_deps() {
  if [ "$WITH_DEPS" = ask ]; then ask_yn "Install system packages (aria2, ffmpeg, archivers)?" yes && WITH_DEPS=yes || WITH_DEPS=no; fi
  [ "$WITH_DEPS" = yes ] || { warn "skipping package installation"; return 0; }
  log "installing runtime packages..."
  case "$OS_ID" in
    debian|ubuntu|raspbian|linuxmint|pop)
      apt_install aria2 ffmpeg unzip p7zip-full ca-certificates curl || true
      apt_install unrar || warn "unrar unavailable (most rar files still extract via 7z)" ;;
    *) warn "unknown distro '$OS_ID': install aria2, ffmpeg, 7z, unrar yourself" ;;
  esac
  install_ytdlp
}

install_ytdlp() {
  have yt-dlp && { ok "yt-dlp present: $(command -v yt-dlp)"; return 0; }
  local asset="yt-dlp_linux"; [ "$GOARCH" = arm64 ] && asset="yt-dlp_linux_aarch64"
  local url="https://github.com/yt-dlp/yt-dlp/releases/latest/download/$asset"
  log "installing yt-dlp ($asset)..."
  if curl -fsSL "$url" -o /usr/local/bin/yt-dlp; then chmod +x /usr/local/bin/yt-dlp; ok "yt-dlp installed"
  else warn "could not download yt-dlp; install it manually or set YTDLP_PATH"; fi
}

build_or_fetch_bot() {
  mkdir -p "$INSTALL_DIR"
  if [ -x "$INSTALL_DIR/DownloadBot" ] && [ "${FORCE:-0}" != 1 ]; then ok "DownloadBot already present"; BOT_READY=1; return 0; fi

  local mode="$BOT_SOURCE"
  if [ "$mode" = auto ]; then
    if have go && [ -f "$(dirname "$0")/go.mod" ]; then mode=build; else mode=download; fi
  fi

  case "$mode" in
    path:*) cp -f "${mode#path:}" "$INSTALL_DIR/DownloadBot" && chmod +x "$INSTALL_DIR/DownloadBot" ;;
    build)
      have go || die "go toolchain not found (install Go 1.24+, or use --bot-source download)"
      log "building bot from source..."
      ( cd "$(dirname "$0")" && go build -mod=vendor -trimpath -ldflags "-s -w" -o "$INSTALL_DIR/DownloadBot" ./cmd/DownloadBot ) \
        || die "bot build failed" ;;
    download)
      local asset="DownloadBot-linux-$GOARCH"
      log "downloading latest release binary ($asset)..."
      curl -fL "$REPO_URL/releases/latest/download/$asset" -o "$INSTALL_DIR/DownloadBot" || die "download failed"
      chmod +x "$INSTALL_DIR/DownloadBot" ;;
    *) die "invalid --bot-source: $mode" ;;
  esac
  ok "bot ready at $INSTALL_DIR/DownloadBot"
}

write_run_tg_api() {
  cat > "$INSTALL_DIR/run-tg-api.sh" <<'SH'
#!/bin/sh
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"
eval "$(python3 - "$DIR/config.json" <<'PY'
import json, sys
from urllib.parse import urlparse
c = json.load(open(sys.argv[1]))
tg = c.get('output', {}).get('telegram', {})
print("API_ID=%d" % int(tg.get('api-id', 0) or 0))
print("PORT=%d" % (urlparse(tg.get('api-base', '') or '').port or 8081))
print("DATA_DIR=%s" % (tg.get('api-dir') or '/mnt/nas/.tg-bot-api'))
PY
)"
API_HASH="$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['output']['telegram'].get('api-hash',''))" "$DIR/config.json")"
if [ -z "$API_HASH" ] || [ "$API_ID" = "0" ]; then
  echo "api-id / api-hash missing in $DIR/config.json" >&2
  exit 1
fi
export TELEGRAM_API_ID="$API_ID"
export TELEGRAM_API_HASH="$API_HASH"
echo "starting telegram-bot-api on port $PORT (data $DATA_DIR)"
mkdir -p "$DATA_DIR"
exec "$DIR/telegram-bot-api" --local --http-port="$PORT" --dir="$DATA_DIR"
SH
  chmod +x "$INSTALL_DIR/run-tg-api.sh"
}

setup_api_server() {
  [ "$WITH_API_SERVER" = yes ] || return 0
  log "setting up local Bot API server..."
  if [ -x "$INSTALL_DIR/telegram-bot-api" ]; then
    ok "reusing existing $INSTALL_DIR/telegram-bot-api (delete it to recompile)"
    write_run_tg_api
    return 0
  fi
  case "$API_SERVER_SOURCE" in
    path:*)
      cp -f "${API_SERVER_SOURCE#path:}" "$INSTALL_DIR/telegram-bot-api"
      chmod +x "$INSTALL_DIR/telegram-bot-api" ;;
    *)
      log "installing build dependencies and compiling telegram-bot-api (this takes several minutes)..."
      apt_install git cmake g++ make gperf libssl-dev zlib1g-dev curl || die "build dependency installation failed"
      local tmp; tmp="$(mktemp -d)"
      git clone --depth 1 --recurse-submodules https://github.com/tdlib/telegram-bot-api.git "$tmp/tg-bot-api" || die "clone failed"
      ( cd "$tmp/tg-bot-api" && mkdir -p build && cd build \
        && cmake -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX:PATH=.. .. \
        && cmake --build . --target install -j"$(nproc)" ) || die "telegram-bot-api build failed"
      cp -f "$tmp/tg-bot-api/bin/telegram-bot-api" "$INSTALL_DIR/telegram-bot-api"
      chmod +x "$INSTALL_DIR/telegram-bot-api"
      rm -rf "$tmp" ;;
  esac
  write_run_tg_api
  ok "local Bot API server ready"
}

write_config() {
  local path="$INSTALL_DIR/config.json"
  if ! confirm_overwrite "$path"; then BOT_TOKEN="$(cfg_get output.telegram.bot-key "$path" "$BOT_TOKEN")"; return 0; fi
  log "writing $path"
  export CFG_DL="$DOWNLOAD_FOLDER" CFG_LIB="$LIBRARY_ROOT" \
         CFG_ARIA_KEY="$ARIA2_SECRET" CFG_BOT="$BOT_TOKEN" CFG_ADMIN="$ADMIN_ID" \
         CFG_MAX="$MAX_INDEX" CFG_LANG="$LANGUAGE" CFG_LOGLEVEL="$LOG_LEVEL" CFG_LOGPATH="$LOG_PATH" CFG_ERRPATH="$ERR_PATH" \
         CFG_ORG="$ORGANIZE_ENABLED" CFG_ANILIST="$ANILIST" CFG_DELARC="$DELETE_ARCHIVE" CFG_KEEP="$KEEP_TORRENT" \
         CFG_YTDLP="$YTDLP_PATH" CFG_YTQ="$YTDLP_QUALITY" CFG_YTEMBED="$YTDLP_EMBED" \
         CFG_APIID="$API_ID" CFG_APIHASH="$API_HASH" CFG_APIBASE="$API_BASE" CFG_APIDIR="$API_DIR" \
         CFG_SERVER="ws://127.0.0.1:6800/jsonrpc" CFG_OUT="$path"
  python3 - <<'PY'
import json, os
def b(k, d=False):
    v = os.environ.get(k, "true" if d else "false").lower()
    return v in ("1", "true", "yes", "on")
def s(k, d=""):
    return os.environ.get(k) or d
cfg = {
  "input": {"aria2": {"aria2-server": s("CFG_SERVER"), "aria2-key": s("CFG_ARIA_KEY")}},
  "output": {"telegram": {"bot-key": s("CFG_BOT"), "user-id": s("CFG_ADMIN"),
                           "api-id": int(s("CFG_APIID", "0") or 0),
                           "api-hash": s("CFG_APIHASH"),
                           "api-base": s("CFG_APIBASE"),
                           "api-dir": s("CFG_APIDIR")}},
  "max-index": int(s("CFG_MAX", "10") or 10),
  "language": s("CFG_LANG", "en"),
  "downloadFolder": s("CFG_DL"),
  "organize": {
    "enabled": b("CFG_ORG", True),
    "anilist": b("CFG_ANILIST", True),
    "deleteArchive": b("CFG_DELARC"),
    "movies": os.path.join(s("CFG_LIB"), "movies"),
    "series": os.path.join(s("CFG_LIB"), "series"),
    "anime": os.path.join(s("CFG_LIB"), "anime"),
    "music": os.path.join(s("CFG_LIB"), "music"),
    "documents": os.path.join(s("CFG_LIB"), "documents"),
    "archives": os.path.join(s("CFG_LIB"), "archives"),
    "others": os.path.join(s("CFG_LIB"), "others"),
    "torrents": os.path.join(s("CFG_LIB"), "torrents"),
    "keepTorrent": b("CFG_KEEP", True),
    "youtube": os.path.join(s("CFG_LIB"), "YouTube"),
    "services": os.path.join(s("CFG_LIB"), "Services"),
    "ytdlpPath": s("CFG_YTDLP"),
    "ytdlpQuality": s("CFG_YTQ"),
    "ytdlpEmbed": b("CFG_YTEMBED", True)
  },
  "log": {"logPath": s("CFG_LOGPATH"), "errPath": s("CFG_ERRPATH"), "level": s("CFG_LOGLEVEL", "info")}
}
path = s("CFG_OUT")
json.dump(cfg, open(path, "w"), indent=2)
os.chmod(path, 0o600)
print("config written:", path)
PY
}

make_dirs() {
  log "creating folders..."
  mkdir -p "$DOWNLOAD_FOLDER" "$INSTALL_DIR"
  if [ "$ORGANIZE_ENABLED" = true ]; then
    mkdir -p \
      "$LIBRARY_ROOT/movies" "$LIBRARY_ROOT/series" "$LIBRARY_ROOT/anime" \
      "$LIBRARY_ROOT/music" "$LIBRARY_ROOT/documents" "$LIBRARY_ROOT/archives" \
      "$LIBRARY_ROOT/others" "$LIBRARY_ROOT/torrents" "$LIBRARY_ROOT/YouTube" "$LIBRARY_ROOT/Services"
  fi
}

write_units() {
  if [ "$WITH_SYSTEMD" = ask ]; then ask_yn "Install and enable systemd services?" yes && WITH_SYSTEMD=yes || WITH_SYSTEMD=no; fi
  [ "$WITH_SYSTEMD" = yes ] || { warn "skipping systemd setup"; return 0; }
  command -v systemctl >/dev/null || { warn "systemctl not found; skipping"; WITH_SYSTEMD=no; return 0; }

  log "writing systemd units..."
  cat > /etc/systemd/system/aria2c.service <<UNIT
[Unit]
Description=Aria2 download daemon
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/bin/aria2c --enable-rpc --rpc-listen-all --rpc-listen-port=6800 --rpc-secret=$ARIA2_SECRET --dir=$DOWNLOAD_FOLDER --continue=true --max-concurrent-downloads=5 --max-connection-per-server=16 --split=16 --file-allocation=none --enable-dht=true --seed-time=0
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

  local api_after="" api_requires=""
  if [ "$WITH_API_SERVER" = yes ]; then
    cat > /etc/systemd/system/tg-bot-api.service <<UNIT
[Unit]
Description=Telegram Bot API server (local mode, prox-download-bot backend)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/run-tg-api.sh
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
    api_after=" tg-bot-api.service"; api_requires=" tg-bot-api.service"
  fi

  cat > /etc/systemd/system/downloadbot.service <<UNIT
[Unit]
Description=prox-download-bot (Telegram Aria2 bot)
After=network-online.target aria2c.service$api_after
Wants=network-online.target
Requires=aria2c.service$api_requires

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/DownloadBot -c $INSTALL_DIR/config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

  systemctl daemon-reload
  systemctl enable aria2c.service >/dev/null 2>&1 || true
  systemctl enable downloadbot.service >/dev/null 2>&1 || true
  [ "$WITH_API_SERVER" = yes ] && systemctl enable tg-bot-api.service >/dev/null 2>&1 || true
  ok "systemd units installed"
}

start_services() {
  [ "$WITH_SYSTEMD" = yes ] || return 0
  if [ "$START_NOW" = ask ]; then ask_yn "Start the services now?" yes && START_NOW=yes || START_NOW=no; fi
  [ "$START_NOW" = yes ] || { warn "not starting services (start later: systemctl start aria2c tg-bot-api downloadbot)"; return 0; }
  log "starting services..."
  systemctl restart aria2c
  [ "$WITH_API_SERVER" = yes ] && systemctl restart tg-bot-api
  sleep 2
  systemctl restart downloadbot
  sleep 4
  echo
  systemctl --no-pager --lines=0 status aria2c downloadbot 2>/dev/null | grep -E "Active:|Loaded:" || true
  [ "$WITH_API_SERVER" = yes ] && systemctl --no-pager --lines=0 status tg-bot-api 2>/dev/null | grep -E "Active:" || true

  if have curl; then
    local v
    v="$(curl -s -m 5 -X POST http://127.0.0.1:6800/jsonrpc -H 'Content-Type: application/json' \
         -d "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"method\":\"aria2.getVersion\",\"params\":[\"token:$ARIA2_SECRET\"]}" 2>/dev/null || true)"
    case "$v" in *version*) ok "aria2 RPC OK" ;; *) warn "aria2 RPC check failed" ;; esac
  fi
  echo
  log "recent bot log:"
  journalctl -u downloadbot -n 8 --no-pager 2>/dev/null || true
}

main() {
  collect_values
  install_deps
  make_dirs
  build_or_fetch_bot
  setup_api_server
  write_config
  write_units
  start_services
  echo
  ok "setup complete"
  echo "  install dir : $INSTALL_DIR"
  echo "  downloads   : $DOWNLOAD_FOLDER"
  echo "  library     : $LIBRARY_ROOT (movies/series/anime/...)"
  echo "  bot API     : $([ "$WITH_API_SERVER" = yes ] && echo "local ($API_BASE), files up to 2 GB" || echo "Telegram cloud (20 MB file cap)")"
  echo
  echo "Open Telegram, send /start to your bot. Logs: journalctl -u downloadbot -f"
}

main "$@"
