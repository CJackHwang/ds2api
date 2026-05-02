#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SMOKE_FILE="${1:-$ROOT_DIR/plans/stage6-manual-smoke.md}"

if [[ ! -f "$SMOKE_FILE" ]]; then
  echo "missing smoke file: $SMOKE_FILE" >&2
  exit 1
fi

extract_field() {
  local field="$1"
  local line
  line="$(grep -E "^[[:space:]]*-[[:space:]]*$field:" "$SMOKE_FILE" | head -n 1 || true)"
  if [[ -z "$line" ]]; then
    echo ""
    return
  fi
  printf '%s' "$line" | sed -E "s/^[[:space:]]*-[[:space:]]*$field:[[:space:]]*//" | sed -E 's/`//g;s/^[[:space:]]+//;s/[[:space:]]+$//'
}

date_value="$(extract_field "Date")"
tester_value="$(extract_field "Tester")"
env_value="$(extract_field "Environment")"
status_value="$(extract_field "Status")"
status_upper="$(printf '%s' "$status_value" | tr '[:lower:]' '[:upper:]')"

failed=0

if [[ -z "$date_value" ]]; then
  echo "invalid smoke file: Date is empty"
  failed=1
fi
if [[ -z "$tester_value" ]]; then
  echo "invalid smoke file: Tester is empty"
  failed=1
fi
if [[ -z "$env_value" ]]; then
  echo "invalid smoke file: Environment is empty"
  failed=1
fi
if [[ "$status_upper" != "PASS" ]]; then
  echo "invalid smoke file: Status must be PASS (got: ${status_value:-<empty>})"
  failed=1
fi

if (( failed != 0 )); then
  exit 1
fi

echo "stage6_manual_smoke=PASS file=$SMOKE_FILE"
