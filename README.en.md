<p align="center">
  <img src="webui/public/ds2api-favicon.svg" width="128" height="128" alt="DS2API icon" />
</p>

# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Release](https://img.shields.io/github/v/release/CJackHwang/ds2api?display_name=tag)](https://github.com/CJackHwang/ds2api/releases)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](docs/DEPLOY.en.md)
[![Deploy on Zeabur](https://zeabur.com/button.svg)](https://zeabur.com/templates/L4CFHP)
[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https://github.com/CJackHwang/ds2api)

Language: [中文](README.MD) | [English](README.en.md)

DS2API converts DeepSeek Web chat capability into OpenAI-compatible, Claude-compatible, and Gemini-compatible APIs. The backend is a **pure Go implementation**, with a React WebUI admin panel (source in `webui/`, build output auto-generated to `static/admin` during deployment).

Documentation entry: [Docs Index](docs/README.md) / [Architecture](docs/ARCHITECTURE.en.md) / [API Reference](API.en.md)

> **Important Disclaimer**
>
> This repository is provided for learning, research, personal experimentation, and internal validation only. It does not grant any commercial authorization and comes with no warranty of fitness, stability, or results.
>
> The author and repository maintainers are not responsible for any direct or indirect loss, account suspension, data loss, legal risk, or third-party claims arising from use, modification, distribution, deployment, or reliance on this project.
>
> Do not use this project in ways that violate service terms, agreements, laws, or platform rules. Before any commercial use, review the `LICENSE`, the relevant terms, and confirm that you have the author's written permission.

## Architecture Overview (Summary)

```mermaid
flowchart LR
    Client["🖥️ Clients / SDKs\n(OpenAI / Claude / Gemini)"]
    Upstream["☁️ DeepSeek API"]

    subgraph DS2API["DS2API 3.x (Unified OpenAI Core)"]
        Router["chi Router + Middleware\n(RequestID / RealIP / Logger / Recoverer / CORS)"]

        subgraph Adapters["Protocol Adapters"]
            OA["OpenAI\n/v1/*"]
            CA["Claude\n/anthropic/* + /v1/messages"]
            GA["Gemini\n/v1beta/models/* + /v1/models/*"]
            Admin["Admin API\n/admin/*"]
            WebUI["WebUI\n/admin (static hosting)"]
        end

        subgraph Runtime["Runtime + Core Capabilities"]
            Bridge["CLIProxy Bridge\n(multi-protocol <-> OpenAI)"]
            OAEngine["OpenAI ChatCompletions\n(unified tools + stream semantics)"]
            Auth["Auth Resolver\n(API key / bearer / x-goog-api-key)"]
            Pool["Account Pool + Queue\n(in-flight slots + wait queue)"]
            DSClient["DeepSeek Client\n(session / auth / HTTP)"]
            Pow["PoW Solver\n(Pure Go ms-level)"]
            Tool["Tool Sieve\n(Go/Node semantic parity)"]
        end
    end

    Client --> Router
    Router --> OA & CA & GA
    Router --> Admin
    Router --> WebUI

    OA --> OAEngine
    CA & GA --> Bridge
    Bridge --> OAEngine
    OAEngine --> Auth
    OAEngine -.account rotation.-> Pool
    OAEngine -.tool-call parsing.-> Tool
    OAEngine -.PoW solving.-> Pow
    Auth --> DSClient
    DSClient --> Upstream
    Upstream --> DSClient
    OAEngine --> Bridge
    Bridge --> Client
```

For the full module-by-module architecture and directory responsibilities, see [docs/ARCHITECTURE.en.md](docs/ARCHITECTURE.en.md).

- **Backend**: Go (`cmd/ds2api/`, `api/`, `internal/`), no Python runtime
- **Frontend**: React admin panel (`webui/`), served as static build at runtime
- **Deployment**: local run, Docker, Vercel serverless, Linux systemd

### 3.X Architecture Changes (vs older releases)

- **Unified routing core**: all protocol entries are now centralized through `internal/server/router.go`, with OpenAI / Claude / Gemini / Admin / WebUI routes registered in one tree to avoid multi-entry drift.
- **Unified execution chain**: Claude/Gemini entries are translated by `internal/translatorcliproxy`, then executed through `openai.ChatCompletions` for shared tool-calling and stream semantics, then translated back to the client protocol.
- **Cleaner adapter boundaries**: `internal/adapter/{claude,gemini}` handles protocol wrappers, while `internal/adapter/openai` remains the execution core; upstream DeepSeek calls are retained only in the OpenAI core.
- **Tool-calling parity across runtimes**: Go (`internal/toolcall`) and Vercel Node (`internal/js/helpers/stream-tool-sieve`) share aligned parsing/anti-leak semantics, now centered on XML/Markup-family payloads (`<tool_call>` / `<function_call>` / `<invoke>` / `tool_use` / antml variants).
- **Config/runtime separation**: static config (`config`) and runtime policy (`settings`) are managed independently via Admin APIs, enabling hot updates and password rotation with JWT invalidation.
- **Streaming behavior upgrade**: `/v1/responses` and `/v1/chat/completions` now share a more consistent incremental tool-call emission strategy across SDK ecosystems.
- **Improved operability**: `/healthz`, `/readyz`, `/admin/version`, and `/admin/dev/captures` form a tighter post-deploy diagnostics loop.

## Key Capabilities

| Capability | Details |
| --- | --- |
| OpenAI compatible | `GET /v1/models`, `GET /v1/models/{id}`, `POST /v1/chat/completions`, `POST /v1/responses`, `GET /v1/responses/{response_id}`, `POST /v1/embeddings`, `POST /v1/files` |
| Claude compatible | `GET /anthropic/v1/models`, `POST /anthropic/v1/messages`, `POST /anthropic/v1/messages/count_tokens` (plus shortcut paths `/v1/messages`, `/messages`) |
| Gemini compatible | `POST /v1beta/models/{model}:generateContent`, `POST /v1beta/models/{model}:streamGenerateContent` (plus `/v1/models/{model}:*` paths) |
| Multi-account rotation | Auto token refresh, email/mobile dual login |
| Concurrency control | Per-account in-flight limit + waiting queue, dynamic recommended concurrency |
| DeepSeek PoW | Pure Go high-performance solver (DeepSeekHashV1), ms-level response |
| Tool Calling | Anti-leak handling: non-code-block feature match, early `delta.tool_calls`, structured incremental output |
| Admin API | Config management, runtime settings hot-reload, proxy management, account testing/batch test, session cleanup, import/export, Vercel sync, version check |
| WebUI Admin Panel | SPA at `/admin` (bilingual Chinese/English, dark mode) |
| Health Probes | `GET /healthz` (liveness), `GET /readyz` (readiness) |

## Platform Compatibility Matrix

| Tier | Platform | Status |
| --- | --- | --- |
| P0 | Codex CLI/SDK (`wire_api=chat` / `wire_api=responses`) | ✅ |
| P0 | OpenAI SDK (JS/Python, chat + responses) | ✅ |
| P0 | Vercel AI SDK (openai-compatible) | ✅ |
| P0 | Anthropic SDK (messages) | ✅ |
| P0 | Google Gemini SDK (generateContent) | ✅ |
| P1 | LangChain / LlamaIndex / OpenWebUI (OpenAI-compatible integration) | ✅ |

## Model Support

### OpenAI Endpoint (`GET /v1/models`)

| Family | Model ID | thinking | search |
| --- | --- | --- | --- |
| default | `deepseek-chat` | ❌ | ❌ |
| default | `deepseek-reasoner` | ✅ | ❌ |
| default | `deepseek-chat-search` | ❌ | ✅ |
| default | `deepseek-reasoner-search` | ✅ | ✅ |
| expert | `deepseek-expert-chat` | ❌ | ❌ |
| expert | `deepseek-expert-reasoner` | ✅ | ❌ |
| expert | `deepseek-expert-chat-search` | ❌ | ✅ |
| expert | `deepseek-expert-reasoner-search` | ✅ | ✅ |
| vision | `deepseek-vision-chat` | ❌ | ❌ |
| vision | `deepseek-vision-reasoner` | ✅ | ❌ |
| vision | `deepseek-vision-chat-search` | ❌ | ✅ |
| vision | `deepseek-vision-reasoner-search` | ✅ | ✅ |

Besides native IDs, DS2API also accepts common aliases as input (for example `gpt-5`, `gpt-5-mini`, `gpt-5-codex`, `gpt-4.1`, `o3`, `claude-opus-4-6`, `claude-sonnet-4-5`, `gemini-2.5-pro`, `gemini-2.5-flash`), but `/v1/models` returns normalized DeepSeek native model IDs.

### Claude Endpoint (`GET /anthropic/v1/models`)

| Current common model | Default Mapping |
| --- | --- |
| `claude-sonnet-4-5` | `deepseek-chat` |
| `claude-haiku-4-5` (compatible with `claude-3-5-haiku-latest`) | `deepseek-chat` |
| `claude-opus-4-6` | `deepseek-reasoner` |

Override mapping via `claude_mapping` or `claude_model_mapping` in config.
Besides the current primary aliases above, `/anthropic/v1/models` also returns Claude 4.x snapshots plus historical 3.x / 2.x / 1.x IDs and common aliases for legacy client compatibility.

#### Claude Code integration pitfalls (validated)

- Set `ANTHROPIC_BASE_URL` to the DS2API root URL (for example `http://127.0.0.1:5001`). Claude Code sends requests to `/v1/messages?beta=true`.
- `ANTHROPIC_API_KEY` must match an entry in `keys` from `config.json`. Keeping both a regular key and an `sk-ant-*` style key improves client compatibility.
- If your environment has proxy variables, set `NO_PROXY=127.0.0.1,localhost,<your_host_ip>` for DS2API to avoid proxy interception of local traffic.
- If tool calls are rendered as plain text and not executed, first verify the model output uses supported XML/Markup tool blocks (`<tool_call>` / `<function_call>` / `<invoke>` / `tool_use`) rather than standalone JSON `tool_calls`.

### Gemini Endpoint

The Gemini adapter maps model names to DeepSeek native models via `model_aliases` or built-in heuristics, supporting both `generateContent` and `streamGenerateContent` call patterns with full Tool Calling support (`functionDeclarations` → `functionCall` output).

## Quick Start

### Recommended deployment priority

Recommended order when choosing a deployment method:

1. **Download and run release binaries**: the easiest path for most users because the artifacts are already built.
2. **Docker / GHCR image deployment**: suitable for containerized, orchestrated, or cloud environments.
3. **Vercel deployment**: suitable if you already use Vercel and accept its platform constraints.
4. **Run from source / build locally**: suitable for development, debugging, or when you need to modify the code yourself.

### Universal First Step (all deployment modes)

Use `config.json` as the single source of truth (recommended):

```bash
cp config.example.json config.json
# Edit config.json
```

Recommended per deployment mode:
- Local run: read `config.json` directly
- Docker / Vercel: generate Base64 from `config.json` and inject as `DS2API_CONFIG_JSON`, or paste raw JSON directly

### Option 1: Download Release Binaries

GitHub Actions automatically builds multi-platform archives on each Release:

```bash
# After downloading the archive for your platform
tar -xzf ds2api_<tag>_linux_amd64.tar.gz
cd ds2api_<tag>_linux_amd64
cp config.example.json config.json
# Edit config.json
./ds2api
```

### Option 2: Docker / GHCR

```bash
# Pull prebuilt image
docker pull ghcr.io/cjackhwang/ds2api:latest

# Or run a pinned version
# docker pull ghcr.io/cjackhwang/ds2api:v3.0.0

# Prepare env file and config file
cp .env.example .env
cp config.example.json config.json

# Start with compose
docker-compose up -d
```

The default `docker-compose.yml` uses `ghcr.io/cjackhwang/ds2api:latest` and maps host port `6011` to container port `5001`. If you want `5001` exposed directly, set `DS2API_HOST_PORT=5001` (or adjust the `ports` mapping).

Rebuild after updates: `docker-compose up -d --build`

#### Zeabur One-Click (Dockerfile)

1. Click the “Deploy on Zeabur” button above to deploy.
2. After deployment, open `/admin` and login with `DS2API_ADMIN_KEY` shown in Zeabur env/template instructions.
3. Import / edit config in Admin UI (it will be written and persisted to `/data/config.json`).

Note: when Zeabur builds directly from the repo `Dockerfile`, you do not need to pass `BUILD_VERSION`. The image prefers that build arg when provided, and automatically falls back to the repo-root `VERSION` file when it is absent.

### Option 3: Vercel

1. Fork this repo to your GitHub account
2. Import the project on Vercel
3. Set environment variables (minimum: `DS2API_ADMIN_KEY`; recommended to also set `DS2API_CONFIG_JSON`)
4. Deploy

Recommended first step in repo root:

```bash
cp config.example.json config.json
# Edit config.json
```

Recommended: convert `config.json` to Base64 locally, then paste into `DS2API_CONFIG_JSON` to avoid JSON formatting mistakes:

```bash
base64 < config.json | tr -d '\n'
```

> **Streaming note**: `/v1/chat/completions` on Vercel is routed to `api/chat-stream.js` (Node Runtime) for real-time SSE. Auth, account selection, and session/PoW preparation are still handled by the Go internal prepare endpoint; streaming output (including `tools`) is assembled on Node with Go-aligned anti-leak handling.

For detailed deployment instructions, see the [Deployment Guide](docs/DEPLOY.en.md).

### Option 4: Local Run

**Prerequisites**: Go 1.26+, Node.js `20.19+` or `22.12+` (only if building WebUI locally)

```bash
# 1. Clone
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. Configure
cp config.example.json config.json
# Edit config.json with your DeepSeek account info and API keys

# 3. Start
go run ./cmd/ds2api
```

Default local URL: `http://127.0.0.1:5001`

The server actually binds to `0.0.0.0:5001`, so devices on the same LAN can usually reach it through your private IP as well.

> **WebUI auto-build**: On first local startup, if `static/admin` is missing, DS2API will auto-run `npm ci` (only when dependencies are missing) and `npm run build -- --outDir static/admin --emptyOutDir` (requires Node.js). You can also build manually: `./scripts/build-webui.sh`

## Configuration

### `config.json` Example

```json
{
  "keys": ["your-api-key-1", "your-api-key-2"],
  "accounts": [
    {
      "email": "user@example.com",
      "password": "your-password"
    },
    {
      "mobile": "12345678901",
      "password": "your-password"
    }
  ],
  "model_aliases": {
    "gpt-4o": "deepseek-chat",
    "gpt-5": "deepseek-chat",
    "gpt-5-mini": "deepseek-chat",
    "gpt-5-codex": "deepseek-reasoner",
    "o3": "deepseek-reasoner",
    "claude-opus-4-6": "deepseek-reasoner",
    "gemini-2.5-flash": "deepseek-chat"
  },
  "compat": {
    "wide_input_strict_output": true,
    "strip_reference_markers": true
  },
  "responses": {
    "store_ttl_seconds": 900
  },
  "embeddings": {
    "provider": "deterministic"
  },
  "claude_mapping": {
    "fast": "deepseek-chat",
    "slow": "deepseek-reasoner"
  },
  "admin": {
    "jwt_expire_hours": 24
  },
  "runtime": {
    "account_max_inflight": 2,
    "account_max_queue": 0,
    "global_max_inflight": 0,
    "token_refresh_interval_hours": 6
  },
  "auto_delete": {
    "mode": "none"
  }
}
```

- `keys`: API access keys; clients authenticate via `Authorization: Bearer <key>`
- `accounts`: DeepSeek account list, supports `email` or `mobile` login
- `token`: Even if set in `config.json`, it is cleared during load (DS2API does not read persisted tokens from config); runtime tokens are maintained/refreshed in memory only
- `model_aliases`: Map common model names (GPT/Codex/Claude) to DeepSeek models
- `compat.wide_input_strict_output`: Keep `true` (current default policy)
- `compat.strip_reference_markers`: Keep `true`; it strips reference markers from visible output
- `toolcall`: Legacy field; the current behavior is fixed to feature matching + high-confidence early emit, and any config value is ignored
- `responses.store_ttl_seconds`: In-memory TTL for `/v1/responses/{id}`
- `embeddings.provider`: Embeddings provider (`deterministic/mock/builtin` built-in)
- `claude_mapping`: Maps `fast`/`slow` suffixes to corresponding DeepSeek models (still compatible with `claude_model_mapping`)
- `admin`: Admin panel settings (JWT expiry, password hash, etc.), hot-reloadable via Admin Settings API
- `runtime`: Runtime parameters (concurrency limits, queue sizes, managed token refresh interval), hot-reloadable via Admin Settings API; `account_max_queue=0`/`global_max_inflight=0` means auto-calculate from recommended values, `token_refresh_interval_hours=6` is the default forced re-login interval
- `auto_delete.mode`: How to clean up DeepSeek remote chat records after each request completes. Supported values: `none` (default, no deletion), `single` (delete only the current session), `all` (delete all sessions); legacy `auto_delete.sessions=true` is still treated as `all`

### Environment Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `PORT` | Service port | `5001` |
| `LOG_LEVEL` | Log level | `INFO` (`DEBUG`/`WARN`/`ERROR`) |
| `DS2API_ADMIN_KEY` | Admin login key | `admin` |
| `DS2API_JWT_SECRET` | Admin JWT signing secret | Same as `DS2API_ADMIN_KEY` |
| `DS2API_JWT_EXPIRE_HOURS` | Admin JWT TTL in hours | `24` |
| `DS2API_CONFIG_PATH` | Config file path | `config.json` |
| `DS2API_CONFIG_JSON` | Inline config (JSON or Base64) | — |
| `DS2API_ENV_WRITEBACK` | Auto-write env-backed config to file and transition to file mode (`1/true/yes/on`) | Disabled |
| `DS2API_STATIC_ADMIN_DIR` | Admin static assets dir | `static/admin` |
| `DS2API_AUTO_BUILD_WEBUI` | Auto-build WebUI on startup | Enabled locally, disabled on Vercel |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | Max in-flight requests per account | `2` |
| `DS2API_ACCOUNT_MAX_QUEUE` | Waiting queue limit | `recommended_concurrency` |
| `DS2API_GLOBAL_MAX_INFLIGHT` | Global max in-flight requests | `recommended_concurrency` |
| `DS2API_VERCEL_INTERNAL_SECRET` | Vercel hybrid streaming internal auth | Falls back to `DS2API_ADMIN_KEY` |
| `DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS` | Stream lease TTL seconds | `900` |
| `DS2API_DEV_PACKET_CAPTURE` | Local dev packet capture switch (record recent request/response bodies) | Enabled by default on non-Vercel local runtime |
| `DS2API_DEV_PACKET_CAPTURE_LIMIT` | Number of captured sessions to retain (auto-evict overflow) | `20` |
| `DS2API_DEV_PACKET_CAPTURE_MAX_BODY_BYTES` | Max recorded bytes per captured response body | `5242880` |
| `VERCEL_TOKEN` | Vercel sync token | — |
| `VERCEL_PROJECT_ID` | Vercel project ID | — |
| `VERCEL_TEAM_ID` | Vercel team ID | — |
| `DS2API_VERCEL_PROTECTION_BYPASS` | Vercel deployment protection bypass for internal Node→Go calls | — |

> Note: when `DS2API_CONFIG_JSON` is detected, the Admin UI shows mode risk and auto-persistence status (including `DS2API_CONFIG_PATH` and mode-transition hints).

## Authentication Modes

For business endpoints (`/v1/*`, `/anthropic/*`, Gemini routes), DS2API supports two modes:

| Mode | Description |
| --- | --- |
| **Managed account** | Use a key from `config.keys` via `Authorization: Bearer ...` or `x-api-key`; DS2API auto-selects an account |
| **Direct token** | If the token is not in `config.keys`, DS2API treats it as a DeepSeek token directly |

Optional header `X-Ds2-Target-Account`: Pin a specific managed account (value is email or mobile).
Gemini routes also accept `x-goog-api-key`, or `?key=` / `?api_key=` when no auth header is present.

## Concurrency Model

```
Per-account inflight = DS2API_ACCOUNT_MAX_INFLIGHT (default 2)
Recommended concurrency = account_count × per_account_inflight
Queue limit = DS2API_ACCOUNT_MAX_QUEUE (default = recommended concurrency)
429 threshold = inflight + queue ≈ account_count × 4
```

- When inflight slots are full, requests enter a waiting queue — **no immediate 429**
- 429 is returned only when total load exceeds inflight + queue capacity
- `GET /admin/queue/status` returns real-time concurrency state

## Tool Call Adaptation

When `tools` is present in the request, DS2API performs anti-leak handling:

1. Toolcall feature matching is enabled only in **non-code-block context** (fenced examples are ignored)
2. The parser currently targets XML/Markup-family tool syntax (`<tool_call>` / `<function_call>` / `<invoke>` / `tool_use` / antml variants); standalone JSON `tool_calls` payloads are not treated as executable calls by default
3. `responses` streaming strictly uses official item lifecycle events (`response.output_item.*`, `response.content_part.*`, `response.function_call_arguments.*`)
4. `responses` supports and enforces `tool_choice` (`auto`/`none`/`required`/forced function); `required` violations return `422` for non-stream and `response.failed` for stream
5. The output protocol follows the client request (OpenAI / Claude / Gemini native shapes); model-side prompting can prefer XML, and the compatibility layer handles the protocol-specific translation

> Note: the current parser still prioritizes “parse successfully whenever possible”; hard allow-list rejection for undeclared tool names is not enabled yet.

## Local Dev Packet Capture

This is for debugging issues such as Responses reasoning streaming and tool-call handoff. When enabled, DS2API stores the latest N DeepSeek conversation payload pairs (request body + upstream response body), defaulting to 20 entries with auto-eviction; each response body is capped at 5 MB by default.

Enable example:

```bash
DS2API_DEV_PACKET_CAPTURE=true \
DS2API_DEV_PACKET_CAPTURE_LIMIT=20 \
go run ./cmd/ds2api
```

Inspect/clear (Admin JWT required):

- `GET /admin/dev/captures`: list captured items (newest first)
- `DELETE /admin/dev/captures`: clear captured items
- `GET /admin/dev/raw-samples/query?q=keyword&limit=20`: search current in-memory captures by prompt keyword and group `completion + continue` by `chat_session_id`
- `POST /admin/dev/raw-samples/save`: persist a selected capture chain as `tests/raw_stream_samples/<sample-id>/`

Response fields include:

- `request_body`: full payload sent to DeepSeek
- `response_body`: concatenated raw upstream stream body text
- `response_truncated`: whether body-size truncation happened

The save endpoint can target a chain by `query`, `chain_key`, or `capture_id`. Example:

```json
{"query":"Guangzhou weather","sample_id":"gz-weather-from-memory"}
```

## Documentation Index

| Document | Description |
| --- | --- |
| [API.md](API.md) / [API.en.md](API.en.md) | API reference with request/response examples |
| [DEPLOY.md](docs/DEPLOY.md) / [DEPLOY.en.md](docs/DEPLOY.en.md) | Deployment guide (local/Docker/Vercel/systemd) |
| [CONTRIBUTING.md](docs/CONTRIBUTING.md) / [CONTRIBUTING.en.md](docs/CONTRIBUTING.en.md) | Contributing guide |
| [TESTING.md](docs/TESTING.md) | Testsuite guide |

## Testing

```bash
# Unit tests (Go + Node)
./tests/scripts/run-unit-all.sh

# One-command live end-to-end tests (real accounts, full request/response logs)
./tests/scripts/run-live.sh

# Or with custom flags
go run ./cmd/ds2api-tests \
  --config config.json \
  --admin-key admin \
  --out artifacts/testsuite \
  --timeout 120 \
  --retries 2
```

```bash
# Release-blocking gates
./tests/scripts/check-stage6-manual-smoke.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm ci --prefix webui && npm run build --prefix webui
```

## Release Artifact Automation (GitHub Actions)

Workflow: `.github/workflows/release-artifacts.yml`

- **Trigger**: only on GitHub Release `published` (normal pushes do not trigger builds)
- **Outputs**: multi-platform archives (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`) + `sha256sums.txt`
- **Container publishing**: GHCR only (`ghcr.io/cjackhwang/ds2api`)
- **Each archive includes**: `ds2api` executable, `static/admin`, WASM file (with embedded fallback support), config template, README, LICENSE

## Disclaimer

This project is built through reverse engineering and is provided for learning, research, personal experimentation, and internal validation only. No commercial authorization is granted, and no warranty of stability, fitness, or results is provided.
The author and repository maintainers are not responsible for any direct or indirect loss, account suspension, data loss, legal risk, or third-party claims arising from use, modification, distribution, deployment, or reliance on this project.

Do not use this project in ways that violate service terms, agreements, laws, or platform rules. Before any commercial use, review the `LICENSE`, the relevant terms, and confirm that you have the author's written permission.
