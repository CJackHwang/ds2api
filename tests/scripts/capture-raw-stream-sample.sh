#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

CONFIG_PATH="${1:-config.json}"
SAMPLE_ID="${2:-capture-$(date -u +%Y%m%dT%H%M%SZ)}"
QUESTION="${3:-广州天气}"
MODEL="${4:-deepseek-v4-pro-search}"
API_KEY="${5:-}"
ADMIN_KEY="${DS2API_ADMIN_KEY:-admin}"

if [[ -z "$API_KEY" ]]; then
  API_KEY="$(python3 - <<'PY' "$CONFIG_PATH"
import json,sys
cfg=json.load(open(sys.argv[1]))
keys=cfg.get('keys') or []
print(keys[0] if keys else '')
PY
)"
fi

if [[ -z "$API_KEY" ]]; then
  echo "[capture] missing API key (pass as arg5 or set config.keys[0])" >&2
  exit 1
fi

HDR_FILE="$(mktemp)"
BODY_FILE="$(mktemp)"

cleanup() {
  rm -f "$HDR_FILE" "$BODY_FILE"
  pkill -f "cmd/ds2api" >/dev/null 2>&1 || true
}
trap cleanup EXIT

DS2API_CONFIG_PATH="$CONFIG_PATH" \
DS2API_ADMIN_KEY="$ADMIN_KEY" \
DS2API_DEV_PACKET_CAPTURE=1 \
DS2API_DEV_PACKET_CAPTURE_LIMIT=20 \
  go run ./cmd/ds2api >/tmp/ds2api_capture_server.log 2>&1 &

for _ in $(seq 1 120); do
  if curl -sSf http://127.0.0.1:5001/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

REQUEST_BODY="$(python3 - <<'PY' "$SAMPLE_ID" "$MODEL" "$QUESTION" "$API_KEY"
import json,sys
sample_id,model,question,api_key=sys.argv[1:5]
payload={
  'sample_id': sample_id,
  'api_key': api_key,
  'model': model,
  'stream': True,
  'messages': [{'role': 'user', 'content': question}],
}
print(json.dumps(payload, ensure_ascii=False))
PY
)"

curl -sS \
  -D "$HDR_FILE" \
  http://127.0.0.1:5001/admin/dev/raw-samples/capture \
  -H "Authorization: Bearer ${ADMIN_KEY}" \
  -H 'Content-Type: application/json' \
  --data-binary "${REQUEST_BODY}" \
  >"$BODY_FILE"

SAMPLE_DIR="$(python3 - <<'PY' "$HDR_FILE"
import sys,pathlib
headers=pathlib.Path(sys.argv[1]).read_text().splitlines()
for line in headers:
  if line.lower().startswith('x-ds2-sample-dir:'):
    print(line.split(':',1)[1].strip())
    raise SystemExit
print('')
PY
)"

SAMPLE_ID_HEADER="$(python3 - <<'PY' "$HDR_FILE"
import sys,pathlib
headers=pathlib.Path(sys.argv[1]).read_text().splitlines()
for line in headers:
  if line.lower().startswith('x-ds2-sample-id:'):
    print(line.split(':',1)[1].strip())
    raise SystemExit
print('')
PY
)"

echo "[capture] sample_id=${SAMPLE_ID_HEADER:-$SAMPLE_ID}"
echo "[capture] sample_dir=${SAMPLE_DIR:-tests/raw_stream_samples/$SAMPLE_ID}"
cat "$BODY_FILE"
