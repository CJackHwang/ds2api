#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

./tests/scripts/check-node-split-syntax.sh

# Keep Node's file-level test scheduling serial to avoid intermittent cross-file
# interference when multiple suites import mutable module singletons.
NODE_TEST_LOG="$(mktemp)"
cleanup() {
  rm -f "$NODE_TEST_LOG"
}
trap cleanup EXIT

if ! node --test --test-concurrency=1 tests/node/stream-tool-sieve.test.js tests/node/chat-stream.test.js tests/node/js_compat_test.js "$@" 2>&1 | tee "$NODE_TEST_LOG"; then
  echo
  echo "[run-unit-node] Node tests failed. 失败摘要如下："
  if command -v rg >/dev/null 2>&1; then
    rg -n "^(not ok|# fail)|ERR_TEST_FAILURE" "$NODE_TEST_LOG" || true
  else
    grep -nE "^(not ok|# fail)|ERR_TEST_FAILURE" "$NODE_TEST_LOG" || true
  fi
  exit 1
fi
