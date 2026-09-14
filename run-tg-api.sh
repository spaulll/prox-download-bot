#!/bin/sh
# Launches the self-hosted Telegram Bot API server using credentials from the
# config.json sitting next to this script (api-id / api-hash / api-base /
# api-dir). Secrets travel via environment (same-UID readable only), never via
# argv, and stay out of the systemd unit.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

eval "$(python3 - "$DIR/config.json" <<'EOF'
import json, sys
from urllib.parse import urlparse
c = json.load(open(sys.argv[1]))
tg = c.get('output', {}).get('telegram', {})
print("API_ID=%d" % int(tg.get('api-id', 0) or 0))
print("PORT=%d" % (urlparse(tg.get('api-base', '') or '').port or 8081))
print("DATA_DIR=%s" % (tg.get('api-dir') or '/mnt/nas/.tg-bot-api'))
EOF
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
exec "$DIR/telegram-bot-api" \
  --local \
  --http-port="$PORT" \
  --dir="$DATA_DIR"
