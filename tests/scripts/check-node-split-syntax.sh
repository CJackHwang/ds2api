#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
TARGETS_FILE="${1:-$ROOT_DIR/plans/node-syntax-gate-targets.txt}"

if [[ ! -f "$TARGETS_FILE" ]]; then
  echo "checked=0 missing=0 invalid=0"
  exit 0
fi

checked=0
missing=0
invalid=0

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ "${file:0:1}" == "#" ]] && continue

  checked=$((checked + 1))
  abs="$ROOT_DIR/$file"
  if [[ ! -f "$abs" ]]; then
    echo "MISSING $file"
    missing=$((missing + 1))
    continue
  fi

  if ! node --check "$abs"; then
    echo "INVALID $file"
    invalid=$((invalid + 1))
  fi
done < "$TARGETS_FILE"

echo "checked=$checked missing=$missing invalid=$invalid"

if (( missing > 0 || invalid > 0 )); then
  exit 1
fi
