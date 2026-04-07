# Contributing Guide

Language: [中文](CONTRIBUTING.md) | [English](CONTRIBUTING.en.md)

Thanks for your interest in contributing to DS2API!

## Development Setup

### Prerequisites

- Go 1.26+
- Node.js `20.19+` or `22.12+` (for WebUI development)
- npm (bundled with Node.js)

### Backend Development

```bash
# 1. Clone
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. Configure
cp config.example.json config.json
# Edit config.json with test accounts

# 3. Run backend
go run ./cmd/ds2api
# Local access: http://127.0.0.1:5001
# Actual bind: 0.0.0.0:5001, so LAN access is available via your private IP
```

### Frontend Development (WebUI)

```bash
# 1. Navigate to WebUI directory
cd webui

# 2. Install dependencies
npm install

# 3. Start dev server (hot reload)
npm run dev
# Default: http://localhost:5173, auto-proxies API to backend
# host: 0.0.0.0 is not configured, so LAN access is not enabled by default
```

WebUI tech stack:
- React + Vite
- Tailwind CSS
- Bilingual language packs: `webui/src/locales/zh.json` / `en.json`

### Docker Development

```bash
docker-compose -f docker-compose.dev.yml up
```

## Code Standards

| Language | Standards |
| --- | --- |
| **Go** | Run `./scripts/lint.sh` (gofmt + golangci-lint) and ensure `go test ./...` passes before committing |
| **JavaScript/React** | Follow existing project style (functional components) |
| **Commit messages** | Use semantic prefixes: `feat:`, `fix:`, `docs:`, `refactor:`, `style:`, `perf:`, `chore:` |

## Submitting a PR

1. Fork the repo
2. Create a branch (e.g. `feature/xxx` or `fix/xxx`)
3. Commit changes
4. Push your branch
5. Open a Pull Request

> 💡 If you modify files under `webui/`, no manual build is needed — CI handles it automatically.
> If you want to verify the generated `static/admin/` assets locally, you can still run `./scripts/build-webui.sh`.

## Build WebUI

Manually build WebUI to `static/admin/`:

```bash
./scripts/build-webui.sh
```

## Running Tests

```bash
# Go + Node unit tests (recommended)
./tests/scripts/run-unit-all.sh

# End-to-end live tests (real accounts)
./tests/scripts/run-live.sh
```

## Project Structure

To avoid documentation drift, directory layout and module responsibilities were moved to:

- [docs/ARCHITECTURE.en.md](./ARCHITECTURE.en.md)
- [docs/README.md](./README.md)

Before contributing, review the architecture doc sections for request flow and `internal/` module boundaries.

## Reporting Issues

Please use [GitHub Issues](https://github.com/CJackHwang/ds2api/issues) and include:

- Steps to reproduce
- Relevant log output
- Environment info (OS, Go version, deployment method)
