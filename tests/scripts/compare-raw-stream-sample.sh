#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

SAMPLE_ID="${1:-}"
BASELINE_ROOT="${2:-}"

if [[ -z "$SAMPLE_ID" ]]; then
  echo "usage: $0 <sample-id> [baseline-root]" >&2
  exit 1
fi

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="artifacts/raw-stream-sim/compare-${SAMPLE_ID}-${RUN_ID}"
REPORT_PATH="$RUN_DIR/report.json"
mkdir -p "$RUN_DIR"

cmd=(
  node tests/tools/deepseek-sse-simulator.mjs
  --samples-root tests/raw_stream_samples
  --sample-id "$SAMPLE_ID"
  --output-root "$RUN_DIR"
  --report "$REPORT_PATH"
)

if [[ -n "$BASELINE_ROOT" ]]; then
  cmd+=(--baseline-root "$BASELINE_ROOT")
fi

"${cmd[@]}"

echo "[compare-raw-stream-sample] output: $RUN_DIR"
echo "[compare-raw-stream-sample] report: $REPORT_PATH"
