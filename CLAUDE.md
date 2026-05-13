# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DS2API transforms DeepSeek Web conversations into OpenAI, Claude, and Gemini compatible APIs. Go (1.26) backend with a React (Vite) WebUI admin panel.

## Build Commands

```bash
go build ./cmd/ds2api          # Build Go binary
go run ./cmd/ds2api            # Run server (default port 5001)
npm run build --prefix webui   # Build WebUI → static/admin/
npm run dev --prefix webui     # Vite dev server for WebUI (port 5173)
node start.mjs                 # Interactive launcher menu (dev/prod/build/webui/install/stop/status)
```

## Test Commands

```bash
./tests/scripts/run-unit-all.sh        # Go + Node unit tests (recommended)
go test ./...                           # Go unit tests only
./tests/scripts/run-unit-node.sh       # Node unit tests only
go test ./internal/<package> -count=1  # Single Go package test
```

Node tests run serially (`--test-concurrency=1`) — this is required, not optional.

## Lint

```bash
./scripts/lint.sh           # Runs golangci-lint (format + lint); auto-bootstraps if needed
gofmt -w <files>            # Format changed Go files before commit
```

## PR Gate — Must Pass All Four Before Opening/Updating a PR

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
```

## Go Style

- 140 character line limit (enforced by `lll` linter).
- Cyclomatic complexity max 15 (`gocyclo`).
- `goimports` with local prefix `ds2api`.
- Run `gofmt -w` on every changed Go file before commit.
- Do not ignore error returns from `Close`, `Flush`, `Sync`, or similar cleanup calls. Log cleanup errors that cannot be returned.

## File Size Limits (check-refactor-line-gate.sh)

- Default: 300 lines
- Frontend (under `webui/`): 500 lines
- Entry files (`api/chat-stream.js`, `internal/js/helpers/stream-tool-sieve.js`, `webui/src/App.jsx`): 120 lines
- Test files: exempt

## Architecture Rules

- **Protocol adapter boundary**: Normalize protocol-specific request shapes to the project-standard request/turn model first, run shared logic once, then render back to the target protocol at the boundary. Do not let protocol formatting own shared business behavior.
- **Documentation sync**: When business logic or user-visible behavior changes, update docs in the same change. `docs/prompt-compatibility.md` is the source of truth for API → web-chat context compatibility.

## Required Toolchain

- **Go 1.26** — newer than most default installations
- **Node 24** — required for WebUI build and Node tests

## Gotchas

- `config.json` (gitignored) contains API keys and DeepSeek credentials — never commit it.
- WebUI builds to `static/admin/` (gitignored) — must build before running if serving admin panel.
- `.env` is committed with placeholder values; real config is in `config.json`.
- Cross-compiles to 7 targets (linux/darwin/windows × amd64/arm64 + linux/armv7).
- golangci-lint auto-bootstraps v2.11.4 to `.tmp/` if system version is incompatible.

## Communication

Always explain tradeoffs when suggesting changes or refactors.
