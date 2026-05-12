#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-artifacts/release-readiness}"

go run ./cmd/release-readiness \
  --branch current \
  --out "${OUT_DIR}/report.md" \
  --json "${OUT_DIR}/report.json" \
  --lint-result "${DS2API_READINESS_LINT_RESULT:-unknown}" \
  --refactor-result "${DS2API_READINESS_REFACTOR_RESULT:-unknown}" \
  --unit-result "${DS2API_READINESS_UNIT_RESULT:-unknown}" \
  --webui-build-result "${DS2API_READINESS_WEBUI_BUILD_RESULT:-unknown}" \
  --live-result "${DS2API_READINESS_LIVE_RESULT:-skip}" \
  --live-skip-reason "${DS2API_READINESS_LIVE_SKIP_REASON:-not required unless high-risk live path changes}"
