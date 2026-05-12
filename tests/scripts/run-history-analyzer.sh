#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-artifacts/history-analyzer}"
HISTORY_PATH="${DS2API_HISTORY_ANALYZER_HISTORY:-}"
RESPONSE_HISTORY_PATH="${DS2API_HISTORY_ANALYZER_RESPONSE_HISTORY:-}"
DEVCAPTURE_PATH="${DS2API_HISTORY_ANALYZER_DEVCAPTURE:-}"
RAWSAMPLE_PATH="${DS2API_HISTORY_ANALYZER_RAWSAMPLE:-}"

args=(
  --out "${OUT_DIR}/report.md"
  --json "${OUT_DIR}/report.json"
  --fixtures "${OUT_DIR}/fixture-candidates.json"
)

if [[ -n "${HISTORY_PATH}" ]]; then
  args+=(--history "${HISTORY_PATH}")
fi
if [[ -n "${RESPONSE_HISTORY_PATH}" ]]; then
  args+=(--response-history "${RESPONSE_HISTORY_PATH}")
fi
if [[ -n "${DEVCAPTURE_PATH}" ]]; then
  args+=(--devcapture "${DEVCAPTURE_PATH}")
fi
if [[ -n "${RAWSAMPLE_PATH}" ]]; then
  args+=(--rawsample "${RAWSAMPLE_PATH}")
fi

if [[ -z "${HISTORY_PATH}${RESPONSE_HISTORY_PATH}${DEVCAPTURE_PATH}${RAWSAMPLE_PATH}" ]]; then
  cat >&2 <<'MSG'
set at least one input:
  DS2API_HISTORY_ANALYZER_HISTORY=/path/to/chat_history.json
  DS2API_HISTORY_ANALYZER_RESPONSE_HISTORY=/path/to/response_history_compatible.json
  DS2API_HISTORY_ANALYZER_DEVCAPTURE=/path/to/dev_capture.json
  DS2API_HISTORY_ANALYZER_RAWSAMPLE=/path/to/raw_samples
MSG
  exit 2
fi

go run ./cmd/history-analyzer "${args[@]}"
