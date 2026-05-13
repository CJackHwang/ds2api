# DS2API Deployment Guide

Language: [中文](DEPLOY.md) | [English](DEPLOY.en.md)

This guide covers all deployment methods for the current Go-based codebase.

Doc map: [Index](./README.md) | [Architecture](./ARCHITECTURE.en.md) | [API](../API.en.md) | [Testing](./TESTING.md)

---

## Table of Contents

- [Recommended deployment priority](#recommended-deployment-priority)
- [Prerequisites](#0-prerequisites)
- [1. Download Release Binaries](#1-download-release-binaries)
- [2. Docker / GHCR Deployment](#2-docker--ghcr-deployment)
- [3. Vercel Deployment](#3-vercel-deployment)
- [4. Local Run from Source](#4-local-run-from-source)
- [5. Reverse Proxy (Nginx)](#5-reverse-proxy-nginx)
- [6. Linux systemd Service](#6-linux-systemd-service)
- [7. Post-Deploy Checks](#7-post-deploy-checks)
- [8. Pre-Release Local Regression](#8-pre-release-local-regression)

---

## Recommended deployment priority

Recommended order when choosing a deployment method:

1. **Download and run release binaries**: the easiest path for most users because the artifacts are already built.
2. **Docker / GHCR image deployment**: suitable for containerized, orchestrated, or cloud environments.
3. **Vercel deployment**: suitable if you already use Vercel and accept its platform constraints.
4. **Run from source / build locally**: suitable for development, debugging, or when you need to modify the code yourself.

---

## 0. Prerequisites

| Dependency | Minimum Version | Notes |
| --- | --- | --- |
| Go | 1.26+ | Build backend |
| Node.js | `20.19+` or `22.12+` (CI / Docker builds use Node 24) | Only needed to build WebUI locally |
| npm | Bundled with Node.js; 10+ recommended | Install WebUI dependencies |

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

## 1. Download Release Binaries

Built-in GitHub Actions workflow: `.github/workflows/release-artifacts.yml`

- **Trigger**: by default only on Release `published`; you can also run it manually via `workflow_dispatch` and pass `release_tag` to rerun / backfill
- **Outputs**: multi-platform binary archives, Linux Docker image export tarballs, and `sha256sums.txt`
- **Container publishing**: GHCR only (`ghcr.io/cjackhwang/ds2api`)

| Platform | Architecture | Format |
| --- | --- | --- |
| Linux | amd64, arm64, armv7 | `.tar.gz` |
| macOS | amd64, arm64 | `.tar.gz` |
| Windows | amd64, arm64 | `.zip` |

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

---

## 2. Docker / GHCR Deployment

### 2.1 Basic Steps

```bash
# Pull prebuilt image
docker pull ghcr.io/cjackhwang/ds2api:latest

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

The default `docker-compose.yml` directly uses `ghcr.io/cjackhwang/ds2api:latest` and maps host port `6011` to container port `5001`. If you want `5001` exposed directly, set `DS2API_HOST_PORT=5001` (or adjust the `ports` mapping).
The compose template also defaults to `DS2API_CONFIG_PATH=/data/config.json` with `./config.json:/data/config.json` mounted, so deployments avoid read-only `/app` persistence issues by default.
The image pre-creates `/data` and grants it to the non-root `ds2api` user. If you bind-mount a single host file, make sure `config.json` is readable/writable by the container user, for example with `chmod 644 config.json`; otherwise Linux UID/GID mismatches can still cause `open /data/config.json: permission denied`.
Compatibility note: when `DS2API_CONFIG_PATH` is unset and runtime base dir is `/app`, newer versions prefer `/data/config.json`; if that file is missing but legacy `/app/config.json` exists, DS2API automatically falls back to the legacy path to avoid post-upgrade config loss.

If you want a pinned version instead of `latest`, you can also pull a specific tag directly:

```bash
docker pull ghcr.io/cjackhwang/ds2api:v3.0.0
```

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
- **Persistent config**: the template mounts `/data` and sets `DS2API_CONFIG_PATH=/data/config.json`. On a fresh volume, DS2API starts with an empty file-backed config; after importing config in Admin UI, it will be written and persisted to this path.
- **`open /app/config.json: permission denied`**: this means the instance is trying to persist runtime tokens to a read-only path (commonly `/app` inside the image).  
  Recommended handling:
  1. Set a writable path explicitly: `DS2API_CONFIG_PATH=/data/config.json` (and mount a persistent volume at `/data`);
  2. If you bootstrap with `DS2API_CONFIG_JSON` and do not need runtime writeback, keep env-backed mode (`DS2API_ENV_WRITEBACK` disabled);
  3. In current versions, login/session tests continue even if persistence fails; Admin API returns a warning that token persistence failed and token is memory-only until restart.
- **Build version**: Zeabur / regular `docker build` does not require `BUILD_VERSION` by default. The image prefers that build arg when provided, and automatically falls back to the repo-root `VERSION` file when it is absent.
- **First login**: after deployment, open `/admin` and login with `DS2API_ADMIN_KEY` shown in Zeabur env/template instructions (recommended: rotate to a strong secret after first login).

#### Manual Deployment Without The Template

If you do not want to use the `zeabur.yaml` one-click template, deploy directly from the repo root with Zeabur's GitHub integration:

1. Fork this repo, or push the code to your own GitHub repository.
2. In Zeabur Dashboard, create a Project, add a Service, then choose a GitHub/Git repository source.
3. Select the repository and branch. Keep Root Directory as `/`.
4. Use the Dockerfile build path. Zeabur auto-detects the repo-root `Dockerfile`; do not set `ZBPACK_IGNORE_DOCKERFILE=true`. If the UI asks for a Dockerfile name, enter `Dockerfile`.
5. Add a persistent volume in the Service settings and mount it at `/data`.
6. Configure environment variables:

| Variable | Recommended value | Description |
| --- | --- | --- |
| `PORT` | `5001` | Service listen port; keep it aligned with the exposed Zeabur HTTP port. |
| `DS2API_ADMIN_KEY` | Strong random string | Required admin login key. |
| `DS2API_CONFIG_PATH` | `/data/config.json` | Recommended persistent config path. |
| `LOG_LEVEL` | `INFO` | Optional log level. |
| `DS2API_CONFIG_JSON` | Raw JSON or Base64 JSON | Optional config bootstrap from env. |
| `DS2API_ENV_WRITEBACK` | `1` | Optional; enable only when using `DS2API_CONFIG_JSON` and you want the initial config written to `/data/config.json`. |

7. Expose HTTP port `5001`. The health check path can be `/healthz`.
8. After deployment, open `/admin`, login with `DS2API_ADMIN_KEY`, then import or edit config in Admin UI. A fresh volume does not need `/data/config.json` up front; the service boots first and creates the file on the first save.

Troubleshooting:

- **Startup log says `open /data/config.json: no such file or directory`**: make sure you deployed a version that includes the fresh-volume bootstrap fix, then redeploy the latest code.
- **`open /app/config.json: permission denied`**: the config path still points at the read-only image directory; mount `/data` and set `DS2API_CONFIG_PATH=/data/config.json`.
- **Config disappears after restart**: check that the `/data` persistent volume is mounted on this service. If you use `DS2API_CONFIG_JSON` but want Admin UI saves persisted, enable `DS2API_ENV_WRITEBACK=1`.

References: Zeabur's official [GitHub/Git integration](https://zeabur.com/docs/en-US/deploy/github), [Dockerfile deployment](https://zeabur.com/docs/en-US/deploy/dockerfile), and [Volumes](https://zeabur.com/docs/data-management/volumes) docs.

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
| `DS2API_RAW_STREAM_SAMPLE_ROOT` | Raw stream sample root for saving/reading samples | `tests/raw_stream_samples` |
| `DS2API_STATIC_ADMIN_DIR` | WebUI static asset directory | `static/admin` |
| `DS2API_AUTO_BUILD_WEBUI` | Whether local startup auto-builds missing WebUI assets (`1/true/yes/on` or `0/false/no/off`) | Enabled outside Vercel |
| `VERCEL_TOKEN` | Vercel sync token | — |
| `VERCEL_PROJECT_ID` | Vercel project ID | — |
| `VERCEL_TEAM_ID` | Vercel team ID | — |
| `DS2API_CHAT_HISTORY_PATH` | Chat history storage path (must be set to `/tmp/chat_history.json` on Vercel, otherwise unavailable due to read-only filesystem) | `data/chat_history.json` |
| `DS2API_VERCEL_PROTECTION_BYPASS` | Deployment protection bypass for internal Node→Go calls | — |

### 3.4 Vercel Architecture

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
- **Stream entry**: `api/chat-stream.js` (Node Runtime for real-time SSE; `vercel.json` rewrites only the canonical `/v1/chat/completions` path here, while the root shortcut `/chat/completions` stays on the Go entry)
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
- The Node stream path also mirrors Go finalization semantics: empty visible output returns the same shaped error SSE, and empty `content_filter` returns a `content_filter` error
- WebUI non-stream test calls `?__go=1` directly to avoid Node hop timeout on long requests

#### Function Duration

`vercel.json` sets `maxDuration: 300` for both `api/chat-stream.js` and `api/index.go` (subject to your Vercel plan limits).

### 3.5 Vercel Troubleshooting

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

#### Chat History Unavailable (read-only file system)

```text
create chat history dir: mkdir /var/task/data: read-only file system
```

**Cause**: Vercel Serverless functions have a read-only filesystem (`/var/task`). Chat history fails because it cannot create directories there.

**Fix**: Add the following in Vercel Project Settings → Environment Variables:

```text
DS2API_CHAT_HISTORY_PATH=/tmp/chat_history.json
```

`/tmp` is the only writable directory in Vercel Serverless. Data is ephemeral (not persisted across cold starts), but the feature works within a single instance lifetime.

### 3.6 Build Artifacts Not Committed

- `static/admin` directory is not in Git
- Vercel / Docker automatically generate WebUI assets during build

---

## 4. Local Run from Source

### 4.1 Basic Steps

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

### 4.2 WebUI Build

On first local startup, if the WebUI static directory is missing, DS2API automatically attempts to build it (requires Node.js/npm; when dependencies are missing it runs `npm ci --prefix webui`, then `npm run build --prefix webui -- --outDir <static-dir> --emptyOutDir`). The default static directory is `static/admin/`, and `DS2API_STATIC_ADMIN_DIR` can override it.

Manual build:

```bash
./scripts/build-webui.sh
```

Or step by step:

```bash
cd webui
npm ci
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

### 4.3 Compile to Binary

```bash
go build -o ds2api ./cmd/ds2api
./ds2api
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
# Expected: {"object":"list","data":[...]} (including `*-nothinking` variants)

# 4. Admin panel (if WebUI is built)
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:5001/admin
# Expected: 200

# 5. Test API call
curl http://127.0.0.1:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hello"}]}'
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

For detailed testsuite documentation, see [TESTING.md](TESTING.md). The fixed local PR gates are listed in [TESTING.md](TESTING.md#pr-门禁--pr-gates).
