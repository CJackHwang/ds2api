# DS2API Deployment Guide

Language: [中文](DEPLOY.md) | [English](DEPLOY.en.md)

This guide covers all deployment methods for the current Go-based codebase.

Doc map: [Index](./README.md) | [Architecture](./ARCHITECTURE.en.md) | [API](../API.en.md) | [Testing](./TESTING.md)

---

## Table of Contents

- [Prerequisites](#0-prerequisites)
- [1. Local Run](#1-local-run)
- [2. Docker Deployment](#2-docker-deployment)
- [3. Vercel Deployment](#3-vercel-deployment)
- [4. Download Release Binaries](#4-download-release-binaries)
- [5. Reverse Proxy (Nginx)](#5-reverse-proxy-nginx)
- [6. Linux systemd Service](#6-linux-systemd-service)
- [7. Post-Deploy Checks](#7-post-deploy-checks)
- [8. Pre-Release Local Regression](#8-pre-release-local-regression)

---

## 0. Prerequisites

| Dependency | Minimum Version | Notes |
| --- | --- | --- |
| Go | 1.26+ | Build backend |
| Node.js | `20.19+` or `22.12+` | Only needed to build WebUI locally |
| npm | Bundled with Node.js | Install WebUI dependencies |

Config source (choose one):

- **File**: `config.json` (recommended for local/Docker)
- **Environment variable**: `DS2API_CONFIG_JSON` (recommended for Vercel; supports raw JSON or Base64)

Unified recommendation (best practice):

```bash
cp config.example.json config.json
# Edit config.json
```

Use `config.json` as the single source of truth:
- Local run: read `config.json` directly
- Docker / Vercel: generate `DS2API_CONFIG_JSON` (Base64) from `config.json` and inject it

---

## 1. Local Run

### 1.1 Basic Steps

```bash
# Clone
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# Copy and edit config
cp config.example.json config.json
# Open config.json and fill in:
#   - keys: your API access keys
#   - accounts: DeepSeek accounts (email or mobile + password)

# Start
go run ./cmd/ds2api
```

Default local access URL: `http://127.0.0.1:5001`; the server actually binds to `0.0.0.0:5001` (override with `PORT`).

### 1.2 WebUI Build

On first local startup, if `static/admin/` is missing, DS2API will automatically attempt to build the WebUI (requires Node.js/npm; when dependencies are missing it runs `npm ci` first, then `npm run build -- --outDir static/admin --emptyOutDir`).

Manual build:

```bash
./scripts/build-webui.sh
```

Or step by step:

```bash
cd webui
npm install
npm run build
# Output goes to static/admin/
```

Control auto-build via environment variable:

```bash
# Disable auto-build
DS2API_AUTO_BUILD_WEBUI=false go run ./cmd/ds2api

# Force enable auto-build
DS2API_AUTO_BUILD_WEBUI=true go run ./cmd/ds2api
```

### 1.3 Compile to Binary

```bash
go build -o ds2api ./cmd/ds2api
./ds2api
```

---

## 2. Docker Deployment

### 2.1 Basic Steps

```bash
# Copy env template and config file
cp .env.example .env
cp config.example.json config.json

# Edit .env and set at least:
#   DS2API_ADMIN_KEY=your-admin-key
# Optionally set the host port:
#   DS2API_HOST_PORT=6011

# Start
docker-compose up -d

# View logs
docker-compose logs -f
```

The default `docker-compose.yml` maps host port `6011` to container port `5001`. If you want `5001` exposed directly, set `DS2API_HOST_PORT=5001` (or adjust the `ports` mapping).

### 2.2 Update

```bash
docker-compose up -d --build
```

### 2.3 Docker Architecture

The `Dockerfile` now provides two image paths:

1. **Default local/dev path (`runtime-from-source`)**: a three-stage build (WebUI build + Go build + runtime).
2. **Release path (`runtime-from-dist`)**: the release workflow first creates tag-named release archives, then copies the Linux bundles to `dist/docker-input/linux_amd64.tar.gz` / `linux_arm64.tar.gz`; Docker consumes those prepared inputs directly, without rerunning `npm build`/`go build`.

The release path keeps Docker images aligned with release archives and reduces duplicate build work.

Container entry command: `/usr/local/bin/ds2api`, default exposed port: `5001`.

### 2.4 Development Mode

```bash
docker-compose -f docker-compose.dev.yml up
```

Development features:
- Source code mounted (live changes)
- `LOG_LEVEL=DEBUG`
- No auto-restart

### 2.5 Health Check

Docker Compose includes a built-in health check:

```yaml
healthcheck:
  test: ["CMD", "/usr/local/bin/busybox", "wget", "-qO-", "http://localhost:${PORT:-5001}/healthz"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

### 2.6 Docker Troubleshooting

If container logs look normal but the admin panel is unreachable, check these first:

1. **Port alignment**: when `PORT` is not `5001`, use the same port in your URL (for example `http://localhost:8080/admin`).
2. **WebUI assets in dev compose**: `docker-compose.dev.yml` runs `go run` in a dev image and does not auto-install Node.js inside the container; if `static/admin` is missing in your repo, `/admin` will return 404. Build once on host: `./scripts/build-webui.sh`.

### 2.7 Zeabur One-Click (Dockerfile)

This repo includes a `zeabur.yaml` template for one-click deployment on Zeabur:

[![Deploy on Zeabur](https://zeabur.com/button.svg)](https://zeabur.com/templates/L4CFHP)

Notes:

- **Port**: DS2API listens on `5001` by default; the template sets `PORT=5001`.
- **Persistent config**: the template mounts `/data` and sets `DS2API_CONFIG_PATH=/data/config.json`. After importing config in Admin UI, it will be written and persisted to this path.
- **Build version**: Zeabur / regular `docker build` does not require `BUILD_VERSION` by default. The image prefers that build arg when provided, and automatically falls back to the repo-root `VERSION` file when it is absent.
- **First login**: after deployment, open `/admin` and login with `DS2API_ADMIN_KEY` shown in Zeabur env/template instructions (recommended: rotate to a strong secret after first login).

---

## 3. Vercel Deployment

### 3.1 Steps

1. **Fork** the repo to your GitHub account
2. **Import** the project on Vercel
3. **Set environment variables** (minimum required: one variable):

| Variable | Description |
| --- | --- |
| `DS2API_ADMIN_KEY` | Admin key (required) |
| `DS2API_CONFIG_JSON` | Config content, raw JSON or Base64 (optional, recommended) |

4. **Deploy**

### 3.1.1 Recommended Input (avoid `DS2API_CONFIG_JSON` mistakes)

If you prefer faster one-click bootstrap, you can leave `DS2API_CONFIG_JSON` empty first, then open `/admin` after deployment, import config, and sync it back to Vercel env vars from the "Vercel Sync" page.

Recommended: in repo root, copy the template first and fill your real accounts:

```bash
cp config.example.json config.json
# Edit config.json
```

Do not hand-edit large JSON directly in Vercel. Generate Base64 locally and paste it:

```bash
# Run in repo root
DS2API_CONFIG_JSON="$(base64 < config.json | tr -d '\n')"
echo "$DS2API_CONFIG_JSON"
```

If you choose to preconfigure before first deploy, set these vars in Vercel Project Settings -> Environment Variables:

```text
DS2API_ADMIN_KEY=replace-with-a-strong-secret
DS2API_CONFIG_JSON=<the single-line Base64 output above>
```

Optional but recommended (for WebUI one-click Vercel sync):

```text
VERCEL_TOKEN=your-vercel-token
VERCEL_PROJECT_ID=prj_xxxxxxxxxxxx
VERCEL_TEAM_ID=team_xxxxxxxxxxxx   # optional for personal accounts
```

### 3.2 Optional Environment Variables

| Variable | Description | Default |
| --- | --- | --- |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | Per-account inflight limit | `2` |
| `DS2API_ACCOUNT_MAX_QUEUE` | Waiting queue limit | `recommended_concurrency` |
| `DS2API_GLOBAL_MAX_INFLIGHT` | Global inflight limit | `recommended_concurrency` |
| `DS2API_ENV_WRITEBACK` | When `DS2API_CONFIG_JSON` is present, auto-write to `DS2API_CONFIG_PATH` and switch to file-backed mode after success (`1/true/yes/on`) | Disabled |
| `DS2API_VERCEL_INTERNAL_SECRET` | Hybrid streaming internal auth | Falls back to `DS2API_ADMIN_KEY` |
| `DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS` | Stream lease TTL | `900` |
| `VERCEL_TOKEN` | Vercel sync token | — |
| `VERCEL_PROJECT_ID` | Vercel project ID | — |
| `VERCEL_TEAM_ID` | Vercel team ID | — |
| `DS2API_VERCEL_PROTECTION_BYPASS` | Deployment protection bypass for internal Node→Go calls | — |

### 3.3 Vercel Architecture

```text
Request ──────┐
              │
              ▼
         vercel.json routing
              │
        ┌─────┴─────┐
        │           │
        ▼           ▼
  api/index.go   api/chat-stream.js
  (Go Runtime)   (Node Runtime)
```

- **Go entry**: `api/index.go` (Serverless Go)
- **Stream entry**: `api/chat-stream.js` (Node Runtime for real-time SSE)
- **Routing**: `vercel.json`
- **Build command**: `npm ci --prefix webui && npm run build --prefix webui` (automatic)

#### Streaming Pipeline

Vercel Go Runtime applies platform-level response buffering, so this project uses a hybrid "**Go prepare + Node stream**" path on Vercel:

1. `api/chat-stream.js` receives `/v1/chat/completions` request
2. Node calls Go internal prepare endpoint (`?__stream_prepare=1`) for session ID, PoW, token
3. Go prepare creates a stream lease, locking the account
4. Node connects directly to DeepSeek upstream, relays SSE in real-time to client (including OpenAI chunk framing and tools anti-leak sieve)
5. After stream ends, Node calls Go release endpoint (`?__stream_release=1`) to free the account

> This adaptation is **Vercel-only**; local and Docker remain pure Go.

#### Non-Stream Fallback and Tool Call Handling

- `api/chat-stream.js` falls back to Go entry (`?__go=1`) for non-stream requests only
- Streaming requests (including requests with `tools`) stay on the Node path and use Go-aligned tool-call anti-leak handling
- WebUI non-stream test calls `?__go=1` directly to avoid Node hop timeout on long requests

#### Function Duration

`vercel.json` sets `maxDuration: 300` for both `api/chat-stream.js` and `api/index.go` (subject to your Vercel plan limits).

### 3.4 Vercel Troubleshooting

#### Go Build Failure

```text
Error: Command failed: go build -ldflags -s -w -o .../bootstrap ...
```

**Cause**: Invalid Go build flag settings in Vercel (`-ldflags` not passed as a single argument).

**Fix**:

1. Open Vercel Project Settings → Build and Development Settings
2. **Clear** custom Go Build Flags / Build Command (recommended)
3. If ldflags must be used, set `-ldflags="-s -w"` (ensure it's one argument)
4. Verify `go.mod` uses a supported version (currently `go 1.26.0`)
5. Redeploy (recommended: clear cache)

#### Internal Package Import Error

```text
use of internal package ds2api/internal/server not allowed
```

**Cause**: Vercel Go entrypoint directly imports `internal/...`.

**Fix**: This repo uses a public bridge package: `api/index.go` → `ds2api/app` → `internal/server`.

#### Output Directory Error

```text
No Output Directory named "public" found after the Build completed.
```

**Fix**: This repo uses `static` as output directory (`"outputDirectory": "static"` in `vercel.json`). If you manually changed Output Directory in Project Settings, set it to `static` or clear it.

#### Deployment Protection Blocking

If API responses return Vercel HTML `Authentication Required`:

- **Option A**: Disable Deployment Protection for that environment (recommended for public APIs)
- **Option B**: Add `x-vercel-protection-bypass` header to requests
- **Option C**: Set `VERCEL_AUTOMATION_BYPASS_SECRET` (or `DS2API_VERCEL_PROTECTION_BYPASS`) for internal Node→Go calls

### 3.5 Build Artifacts Not Committed

- `static/admin` directory is not in Git
- Vercel / Docker automatically generate WebUI assets during build

---

## 4. Download Release Binaries

Built-in GitHub Actions workflow: `.github/workflows/release-artifacts.yml`

- **Trigger**: only on Release `published` (no build on normal push)
- **Outputs**: multi-platform binary archives + `sha256sums.txt`
- **Container publishing**: GHCR only (`ghcr.io/cjackhwang/ds2api`)

| Platform | Architecture | Format |
| --- | --- | --- |
| Linux | amd64, arm64 | `.tar.gz` |
| macOS | amd64, arm64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

Each archive includes:

- `ds2api` executable (`ds2api.exe` on Windows)
- `static/admin/` (built WebUI assets)
- `config.example.json`, `.env.example`
- `README.MD`, `README.en.md`, `LICENSE`

### Usage

```bash
# 1. Download the archive for your platform
# 2. Extract
tar -xzf ds2api_<tag>_linux_amd64.tar.gz
cd ds2api_<tag>_linux_amd64

# 3. Configure
cp config.example.json config.json
# Edit config.json

# 4. Start
./ds2api
```

### Maintainer Release Flow

1. Create and publish a GitHub Release (with tag, for example `vX.Y.Z`)
2. Wait for the `Release Artifacts` workflow to complete
3. Download the matching archive from Release Assets

### Pull from GHCR (Optional)

```bash
# latest
docker pull ghcr.io/cjackhwang/ds2api:latest

# specific version (example)
docker pull ghcr.io/cjackhwang/ds2api:v3.0.0
```

---

## 5. Reverse Proxy (Nginx)

When deploying behind Nginx, **you must disable buffering** for SSE streaming to work:

```nginx
location / {
    proxy_pass http://127.0.0.1:5001;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;
    tcp_nodelay on;
}
```

For HTTPS, add SSL at the Nginx layer:

```nginx
server {
    listen 443 ssl;
    server_name api.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:5001;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
        tcp_nodelay on;
    }
}
```

---

## 6. Linux systemd Service

### 6.1 Installation

```bash
# Copy compiled binary and related files to target directory
sudo mkdir -p /opt/ds2api
sudo cp ds2api config.json /opt/ds2api/
sudo cp -r static/admin /opt/ds2api/static/admin
```

### 6.2 Create systemd Service File

```ini
# /etc/systemd/system/ds2api.service

[Unit]
Description=DS2API (Go)
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/ds2api
Environment=PORT=5001
Environment=DS2API_CONFIG_PATH=/opt/ds2api/config.json
Environment=DS2API_ADMIN_KEY=your-admin-key-here
ExecStart=/opt/ds2api/ds2api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 6.3 Common Commands

```bash
# Reload service config
sudo systemctl daemon-reload

# Enable on boot
sudo systemctl enable ds2api

# Start
sudo systemctl start ds2api

# Check status
sudo systemctl status ds2api

# View logs
sudo journalctl -u ds2api -f

# Restart
sudo systemctl restart ds2api

# Stop
sudo systemctl stop ds2api
```

---

## 7. Post-Deploy Checks

After deployment (any method), verify in order:

```bash
# 1. Liveness probe
curl -s http://127.0.0.1:5001/healthz
# Expected: {"status":"ok"}

# 2. Readiness probe
curl -s http://127.0.0.1:5001/readyz
# Expected: {"status":"ready"}

# 3. Model list
curl -s http://127.0.0.1:5001/v1/models
# Expected: {"object":"list","data":[...]}

# 4. Admin panel (if WebUI is built)
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:5001/admin
# Expected: 200

# 5. Test API call
curl http://127.0.0.1:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-chat","messages":[{"role":"user","content":"hello"}]}'
```

---

## 8. Pre-Release Local Regression

Run the full live testsuite before release (real account tests):

```bash
./tests/scripts/run-live.sh
```

With custom flags:

```bash
go run ./cmd/ds2api-tests \
  --config config.json \
  --admin-key admin \
  --out artifacts/testsuite \
  --timeout 120 \
  --retries 2
```

The testsuite automatically performs:

- ✅ Preflight checks (syntax/build/unit tests)
- ✅ Isolated config copy startup (no mutation to your original `config.json`)
- ✅ Live scenario verification (OpenAI/Claude/Admin/concurrency/toolcall/streaming)
- ✅ Full request/response artifact logging for debugging

For detailed testsuite documentation, see [TESTING.md](TESTING.md).
