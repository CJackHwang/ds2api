#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="artifacts/raw-stream-sim/run-${RUN_ID}"
REPORT_PATH="$RUN_DIR/report.json"
mkdir -p "$RUN_DIR"

node tests/tools/deepseek-sse-simulator.mjs \
  --samples-root tests/raw_stream_samples \
  --output-root "$RUN_DIR" \
  --report "$REPORT_PATH" \
  "$@"

echo "[run-raw-stream-sim] output: $RUN_DIR"
echo "[run-raw-stream-sim] report: $REPORT_PATH"
