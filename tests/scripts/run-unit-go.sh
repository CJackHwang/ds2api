#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-${ROOT_DIR}/.tmp/go-build-cache}"
mkdir -p "$GOCACHE"

go test ./... "$@"
