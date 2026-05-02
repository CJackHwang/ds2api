#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
TARGETS_FILE="$ROOT_DIR/plans/refactor-line-gate-targets.txt"

DEFAULT_MAX=300
FRONTEND_MAX=500
ENTRY_MAX=120

is_entry_file() {
  case "$1" in
    api/chat-stream.js|\
    internal/js/helpers/stream-tool-sieve.js|\
    webui/src/App.jsx)
      return 0
      ;;
  esac
  return 1
}

is_frontend_file() {
  [[ "$1" == webui/* ]]
}

is_test_file() {
  local file="$1"
  local base
  base="$(basename "$file")"

  [[ "$file" == tests/* ]] && return 0
  [[ "$file" == */tests/* ]] && return 0
  [[ "$file" == */__tests__/* ]] && return 0
  [[ "$base" == *_test.go ]] && return 0
  [[ "$base" == *.test.js ]] && return 0
  [[ "$base" == *.test.jsx ]] && return 0
  [[ "$base" == *.test.ts ]] && return 0
  [[ "$base" == *.test.tsx ]] && return 0

  return 1
}

if [[ ! -f "$TARGETS_FILE" ]]; then
  echo "checked=0 missing=0 over_limit=0"
  exit 0
fi

missing=0
over=0
checked=0

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ "${file:0:1}" == "#" ]] && continue

  if is_test_file "$file"; then
    continue
  fi

  checked=$((checked + 1))
  abs="$ROOT_DIR/$file"
  if [[ ! -f "$abs" ]]; then
    echo "MISSING $file"
    missing=$((missing + 1))
    continue
  fi

  lines="$(wc -l < "$abs" | tr -d ' ')"
  limit="$DEFAULT_MAX"
  if is_entry_file "$file"; then
    limit="$ENTRY_MAX"
  elif is_frontend_file "$file"; then
    limit="$FRONTEND_MAX"
  fi

  if (( lines > limit )); then
    echo "OVER $file lines=$lines limit=$limit"
    over=$((over + 1))
  fi
done < "$TARGETS_FILE"

echo "checked=$checked missing=$missing over_limit=$over"

if (( missing > 0 || over > 0 )); then
  exit 1
fi
