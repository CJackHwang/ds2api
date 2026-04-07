# DS2API Architecture & Project Layout

Language: [中文](ARCHITECTURE.md) | [English](ARCHITECTURE.en.md)

> This file is the single architecture source for directory layout, module boundaries, and execution flow.

## 1. Top-level Layout (expanded)

> Notes: this is the **fully expanded** project directory list (excluding metadata/dependency dirs such as `.git/` and `webui/node_modules/`), with each folder annotated by purpose.

```text
ds2api/
├── .github/                              # GitHub collaboration and CI config
│   ├── ISSUE_TEMPLATE/                   # Issue templates
│   └── workflows/                        # GitHub Actions workflows
├── api/                                  # Serverless entrypoints (Vercel Go/Node)
├── app/                                  # Application-level handler assembly
├── cmd/                                  # Executable entrypoints
│   ├── ds2api/                           # Main service bootstrap
│   └── ds2api-tests/                     # E2E testsuite CLI bootstrap
├── docs/                                 # Project documentation
├── internal/                             # Core implementation (non-public packages)
│   ├── account/                          # Account pool, inflight slots, waiting queue
│   ├── adapter/                          # Multi-protocol adapters
│   │   ├── claude/                       # Claude protocol adapter
│   │   ├── gemini/                       # Gemini protocol adapter
│   │   └── openai/                       # OpenAI adapter and shared execution core
│   ├── admin/                            # Admin API (config/accounts/ops)
│   ├── auth/                             # Auth/JWT/credential resolution
│   ├── claudeconv/                       # Claude message conversion helpers
│   ├── compat/                           # Compatibility and regression helpers
│   ├── config/                           # Config loading/validation/hot reload
│   ├── deepseek/                         # DeepSeek upstream client capabilities
│   │   └── transport/                    # DeepSeek transport details
│   ├── devcapture/                       # Dev capture and troubleshooting
│   ├── format/                           # Response formatting layer
│   │   ├── claude/                       # Claude output formatting
│   │   └── openai/                       # OpenAI output formatting
│   ├── js/                               # Node runtime related logic
│   │   ├── chat-stream/                  # Node streaming bridge
│   │   ├── helpers/                      # JS helper modules
│   │   │   └── stream-tool-sieve/        # JS implementation of tool sieve
│   │   └── shared/                       # Shared semantics between Go/Node
│   ├── prompt/                           # Prompt composition
│   ├── rawsample/                        # Raw sample read/write and management
│   ├── server/                           # Router and middleware assembly
│   ├── sse/                              # SSE parsing utilities
│   ├── stream/                           # Unified stream consumption engine
│   ├── testsuite/                        # Testsuite execution framework
│   ├── textclean/                        # Text cleanup
│   ├── toolcall/                         # Tool-call parsing and repair
│   ├── translatorcliproxy/               # Cross-protocol translation bridge
│   ├── util/                             # Shared utility helpers
│   ├── version/                          # Version query/compare
│   └── webui/                            # WebUI static hosting logic
├── plans/                                # Stage plans and manual QA records
├── pow/                                  # PoW standalone implementation + benchmarks
├── scripts/                              # Build/release helper scripts
├── tests/                                # Test assets and scripts
│   ├── compat/                           # Compatibility fixtures + expected outputs
│   │   ├── expected/                     # Expected output samples
│   │   └── fixtures/                     # Fixture inputs
│   │       ├── sse_chunks/               # SSE chunk fixtures
│   │       └── toolcalls/                # Tool-call fixtures
│   ├── node/                             # Node unit tests
│   ├── raw_stream_samples/               # Upstream raw SSE samples
│   │   ├── content-filter-trigger-20260405-jwt3/          # Content-filter terminal sample
│   │   ├── continue-thinking-snapshot-replay-20260405/    # Continue-thinking sample
│   │   ├── guangzhou-weather-reasoner-search-20260404/    # Search/reference sample
│   │   ├── markdown-format-example-20260405/              # Markdown sample
│   │   └── markdown-format-example-20260405-spacefix/     # Space-fix sample
│   ├── scripts/                          # Test entry scripts
│   └── tools/                            # Testing helper tools
└── webui/                                # React admin console source
    ├── public/                           # Static assets
    └── src/                              # Frontend source code
        ├── app/                          # Routing/state scaffolding
        ├── components/                   # Shared UI components
        ├── features/                     # Feature modules
        │   ├── account/                  # Account management page
        │   ├── apiTester/                # API tester page
        │   ├── settings/                 # Settings page
        │   └── vercel/                   # Vercel sync page
        ├── layout/                       # Layout components
        ├── locales/                      # i18n strings
        └── utils/                        # Frontend utilities
```

## 2. Primary Request Flow

```mermaid
flowchart LR
    C[Client/SDK] --> R[internal/server/router.go]
    R --> OA[OpenAI Adapter]
    R --> CA[Claude Adapter]
    R --> GA[Gemini Adapter]
    R --> AD[Admin API]

    CA --> BR[translatorcliproxy]
    GA --> BR
    BR --> CORE[internal/adapter/openai ChatCompletions]
    OA --> CORE

    CORE --> AUTH[internal/auth + config key/account resolver]
    CORE --> POOL[internal/account queue + concurrency]
    CORE --> TOOL[internal/toolcall parser + sieve]
    CORE --> DS[internal/deepseek client]
    DS --> U[DeepSeek upstream]
```

## 3. Responsibilities in `internal/`

- `internal/server`: router tree + middlewares (health, protocol routes, Admin/WebUI).
- `internal/adapter/openai`: shared execution core (chat/responses/embeddings + tool semantics).
- `internal/adapter/{claude,gemini}`: protocol wrappers only (no duplicated upstream execution).
- `internal/translatorcliproxy`: structure translation between Claude/Gemini and OpenAI.
- `internal/deepseek`: upstream request/session/PoW/SSE handling.
- `internal/stream` + `internal/sse`: stream parsing and incremental assembly.
- `internal/toolcall`: JSON/XML/invoke/text-kv tool-call parsing + anti-leak sieve.
- `internal/admin`: config/accounts/vercel sync/version/dev-capture endpoints.
- `internal/config`: config loading/validation + runtime settings hot-reload.
- `internal/account`: managed account pool, inflight slots, waiting queue.

## 4. WebUI Runtime Relation

- `webui/` stores frontend source (Vite + React).
- Runtime serves static output from `static/admin`.
- On first local startup, if `static/admin` is missing, DS2API may auto-build it (Node.js required).

## 5. Documentation Split Strategy

- Onboarding & quick start: `README.MD` / `README.en.md`
- Architecture & layout: `docs/ARCHITECTURE*.md` (this file)
- API contracts: `API.md` / `API.en.md`
- Deployment/testing/contributing: `docs/DEPLOY*`, `docs/TESTING.md`, `docs/CONTRIBUTING*`
- Deep topics: `docs/toolcall-semantics.md`, `docs/DeepSeekSSE行为结构说明-2026-04-05.md`
