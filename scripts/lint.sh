#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

LINT_BIN="${GOLANGCI_LINT_BIN:-golangci-lint}"
BOOTSTRAP_VERSION="${GOLANGCI_LINT_VERSION:-v2.11.4}"
BOOTSTRAP_BIN="${ROOT_DIR}/.tmp/golangci-lint-${BOOTSTRAP_VERSION}"
BOOTSTRAP_TARGET=""

detect_bootstrap_target() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    linux|darwin|windows) ;;
    mingw*|msys*|cygwin*) os="windows" ;;
    *)
      echo "unsupported OS for auto-bootstrap: ${os}" >&2
      return 1
      ;;
  esac

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "unsupported architecture for auto-bootstrap: ${arch}" >&2
      return 1
      ;;
  esac

  BOOTSTRAP_TARGET="${os}-${arch}"
}

should_bootstrap() {
  local output="$1"
  [[ "$output" == *"unknown command \"fmt\" for \"golangci-lint\""* ]] ||
    [[ "$output" == *"unknown command \"run\" for \"golangci-lint\""* ]] ||
    [[ "$output" == *"is no longer supported"* ]] ||
    [[ "$output" == *"unknown flag:"* ]] ||
    [[ "$output" == *"command not found"* ]] ||
    [[ "$output" == *"No such file or directory"* ]]
}

bootstrap_golangci_lint() {
  local version_no_v archive_url tmp_dir
  version_no_v="${BOOTSTRAP_VERSION#v}"
  detect_bootstrap_target
  archive_url="https://github.com/golangci/golangci-lint/releases/download/${BOOTSTRAP_VERSION}/golangci-lint-${version_no_v}-${BOOTSTRAP_TARGET}.tar.gz"

  mkdir -p "${ROOT_DIR}/.tmp"
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN

  curl -sSfL "${archive_url}" -o "${tmp_dir}/golangci-lint.tar.gz"
  tar -xzf "${tmp_dir}/golangci-lint.tar.gz" -C "${tmp_dir}"
  cp "${tmp_dir}/golangci-lint-${version_no_v}-${BOOTSTRAP_TARGET}/golangci-lint" "${BOOTSTRAP_BIN}"
  chmod +x "${BOOTSTRAP_BIN}"

  echo "bootstrapped golangci-lint ${BOOTSTRAP_VERSION} to ${BOOTSTRAP_BIN}" >&2
}

run_lint() {
  local bin="$1"
  if [[ "$bin" == *" "* ]]; then
    eval "$bin fmt --diff -c .golangci.yml" && eval "$bin run -c .golangci.yml"
  else
    "$bin" fmt --diff -c .golangci.yml && "$bin" run -c .golangci.yml
  fi
}

# v2 separates formatters from linters; enforce both in one entrypoint.
if lint_output="$(run_lint "$LINT_BIN" 2>&1)"; then
  [[ -n "$lint_output" ]] && echo "$lint_output"
  exit 0
fi

if [[ -n "${GOLANGCI_LINT_BIN:-}" ]]; then
  echo "$lint_output" >&2
  echo "lint failed with explicit GOLANGCI_LINT_BIN=${GOLANGCI_LINT_BIN}; skip auto-bootstrap." >&2
  exit 1
fi

if ! should_bootstrap "$lint_output"; then
  echo "$lint_output" >&2
  exit 1
fi

echo "default golangci-lint is incompatible; bootstrapping ${BOOTSTRAP_VERSION}..." >&2
if [[ ! -x "${BOOTSTRAP_BIN}" ]]; then
  bootstrap_golangci_lint
fi

run_lint "${BOOTSTRAP_BIN}"
