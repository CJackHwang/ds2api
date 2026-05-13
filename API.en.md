# DS2API API Reference

Language: [中文](API.md) | [English](API.en.md)

This document describes the actual behavior of the current Go codebase.

Docs: [Overview](README.en.md) / [Architecture](docs/ARCHITECTURE.en.md) / [Deployment](docs/DEPLOY.en.md) / [Testing](docs/TESTING.md)

---

## Table of Contents

- [Basics](#basics)
- [Configuration Best Practice](#configuration-best-practice)
- [Authentication](#authentication)
- [Route Index](#route-index)
- [Health Endpoints](#health-endpoints)
- [OpenAI-Compatible API](#openai-compatible-api)
- [Claude-Compatible API](#claude-compatible-api)
- [Gemini-Compatible API](#gemini-compatible-api)
- [Ollama API](#ollama-api)
- [Admin API](#admin-api)
- [Error Payloads](#error-payloads)
- [cURL Examples](#curl-examples)

---

## Basics

| Item | Details |
| --- | --- |
| Base URL | `http://localhost:5001` or your deployment domain |
| Default Content-Type | `application/json` |
| Health probes | `GET /healthz`, `GET /readyz` |
| CORS | Enabled (uniformly covers `/v1/*`, `/anthropic/*`, `/v1beta/models/*`, `/api/*`, and `/admin/*`; echoes the browser `Origin` when present, otherwise `*`; default allow-list includes `Content-Type`, `Authorization`, `X-API-Key`, `X-Ds2-Target-Account`, `X-Ds2-Source`, `X-Vercel-Protection-Bypass`, `X-Goog-Api-Key`, `Anthropic-Version`, `Anthropic-Beta`, and also accepts third-party preflight-requested headers such as `x-stainless-*`; `/v1/chat/completions` on Vercel Node Runtime matches the same behavior; internal-only `X-Ds2-Internal-Token` remains blocked) |

- All JSON request bodies must be valid UTF-8; malformed byte sequences are rejected on ingress with `400 invalid json`.

### 3.0 Adapter-Layer Notes

- OpenAI / Claude / Gemini protocols are now mounted on one shared `chi` router tree assembled in `internal/server/router.go`.
- Adapter responsibilities are streamlined to: **request normalization → DeepSeek invocation → protocol-shaped rendering**, reducing legacy split-logic paths.
- Tool-calling semantics are aligned between Go and Node runtime: models should output the halfwidth-pipe DSML shell `<|DSML|tool_calls>` → `<|DSML|invoke name="...">` → `<|DSML|parameter name="...">`; DS2API also accepts DSML wrapper aliases such as `<dsml|tool_calls>` and `<|tool_calls>`, common DSML separator drift such as `<|DSML tool_calls>`, collapsed DSML local names such as `<DSMLtool_calls>`, control-separator drift such as `<DSML␂tool_calls>` / raw STX `\x02`, CJK angle bracket, fullwidth-bang / ideographic-comma separator drift, PascalCase local-name drift, and trailing attribute separator drift such as `<DSM|parameter name="command"|>...〈/DSM|parameter〉`, `<！DSML！invoke name=“Bash”>`, `<、DSML、tool_calls>`, `<DSmartToolCalls>`, or `<DSMLtool_calls※>`, arbitrary protocol prefixes such as `<proto💥tool_calls>`, and legacy canonical XML `<tool_calls>` → `<invoke name="...">` → `<parameter name="...">`. The scanner normalizes fixed local names (`tool_calls` / `invoke` / `parameter`) with non-structural separators before or after them back to XML before parsing, and also tolerates CDATA opener drift such as `<！[CDATA[` / `<、[CDATA[`; only wrapped tool blocks or the narrow missing-opening-wrapper repair path enter the tool path, while bare `<invoke>` does not count as supported syntax. JSON literal parameter bodies are preserved as structured values, explicit empty or whitespace-only parameters are preserved as empty strings, malformed complete wrappers are released as plain text, and loose CDATA is narrowly repaired at final parse/flush when it can preserve a complete outer tool call.
- `Admin API` separates static config from runtime policy: `/admin/config*` for configuration state, `/admin/settings*` for runtime behavior.
- When upstream returns a thinking-only response with no visible text, the Go main path and the Vercel Node streaming path retry once in the same DeepSeek session: it appends the prompt suffix `"Previous reply had no visible output. Please regenerate the visible final answer or tool call now."` and sets `parent_message_id`. If that same-account retry would still end as `429 upstream_empty_output`, managed-account mode switches to the next available account, creates a fresh session, and retries the original payload once before returning 429.
- Citation/reference marker boundary: streaming output hides upstream `[citation:N]` / `[reference:N]` placeholders by default; non-stream output converts DeepSeek search reference markers into Markdown links.

---

## Configuration Best Practice

Use `config.json` as the single source of truth:

```bash
cp config.example.json config.json
# Edit config.json (keys/accounts)
```

Use it per deployment mode:

- Local run: read `config.json` directly
- Docker / Vercel: generate Base64 from `config.json`, then set `DS2API_CONFIG_JSON`, or paste raw JSON directly

```bash
DS2API_CONFIG_JSON="$(base64 < config.json | tr -d '\n')"
```

For Vercel one-click bootstrap, you can set only `DS2API_ADMIN_KEY` first, then import config at `/admin` and sync env vars from the "Vercel Sync" page.

---

## Authentication

### Business Endpoints (`/v1/*`, `/anthropic/*`, `/v1beta/models/*`)

Two header formats accepted:

| Method | Example |
| --- | --- |
| Bearer Token | `Authorization: Bearer <token>` |
| API Key Header | `x-api-key: <token>` (no `Bearer` prefix) |
| Gemini-compatible | `x-goog-api-key: <token>` or `?key=<token>` / `?api_key=<token>` |

**Auth behavior**:

- Token is in `config.keys` → **Managed account mode**: DS2API auto-selects an account via rotation
- Token is not in `config.keys` → **Direct token mode**: treated as a DeepSeek token directly

**Optional header**: `X-Ds2-Target-Account: <email_or_mobile>` — Pin a specific managed account; if the target account does not exist or the managed-account queue is exhausted, the request returns `429`, and current responses do not include `Retry-After`. If the account exists but login/refresh fails, the request returns the underlying `401` or upstream error. Without a pinned target, managed-account completion requests try one alternate-account fresh retry before returning an empty-output 429; pinned-target requests and requests with no other available account do not switch.
Gemini-compatible clients can also send `x-goog-api-key`, `?key=`, or `?api_key=` as the caller credential source.

### Admin Endpoints (`/admin/*`)

| Endpoint | Auth |
| --- | --- |
| `POST /admin/login` | Public |
| `GET /admin/verify` | `Authorization: Bearer <jwt>` (JWT only) |
| Other `/admin/*` | `Authorization: Bearer <jwt>` or `Authorization: Bearer <admin_key>` |

---

## Route Index

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| GET | `/healthz` | None | Liveness probe |
| HEAD | `/healthz` | None | Liveness probe (no body) |
| GET | `/readyz` | None | Readiness probe |
| HEAD | `/readyz` | None | Readiness probe (no body) |
| GET | `/v1/models` | None | OpenAI model list |
| GET | `/v1/models/{id}` | None | OpenAI single-model query (alias accepted) |
| POST | `/v1/chat/completions` | Business | OpenAI chat completions |
| POST | `/v1/responses` | Business | OpenAI Responses API (stream/non-stream) |
| GET | `/v1/responses/{response_id}` | Business | Query stored response (in-memory TTL) |
| POST | `/v1/embeddings` | Business | OpenAI Embeddings API |
| POST | `/v1/files` | Business | OpenAI Files upload (multipart/form-data) |
| GET | `/v1/files/{file_id}` | Business | Retrieve uploaded file status |
| GET | `/anthropic/v1/models` | None | Claude model list |
| POST | `/anthropic/v1/messages` | Business | Claude messages |
| POST | `/anthropic/v1/messages/count_tokens` | Business | Claude token counting |
| POST | `/v1/messages` | Business | Claude shortcut path |
| POST | `/messages` | Business | Claude shortcut path |
| POST | `/v1/messages/count_tokens` | Business | Claude token counting shortcut |
| POST | `/messages/count_tokens` | Business | Claude token counting shortcut |
| POST | `/v1beta/models/{model}:generateContent` | Business | Gemini non-stream |
| POST | `/v1beta/models/{model}:streamGenerateContent` | Business | Gemini stream |
| POST | `/v1/models/{model}:generateContent` | Business | Gemini non-stream compat path |
| POST | `/v1/models/{model}:streamGenerateContent` | Business | Gemini stream compat path |
| GET | `/api/version` | None | Ollama version endpoint |
| GET | `/api/tags` | None | Ollama model list |
| POST | `/api/show` | None | Ollama model capability query (returns `id` + `capabilities`) |
| POST | `/admin/login` | None | Admin login |
| GET | `/admin/verify` | JWT | Verify admin JWT |
| GET | `/admin/vercel/config` | Admin | Read preconfigured Vercel creds |
| GET | `/admin/config` | Admin | Read sanitized config |
| POST | `/admin/config` | Admin | Update config |
| GET | `/admin/settings` | Admin | Read runtime settings |
| PUT | `/admin/settings` | Admin | Update runtime settings (hot reload) |
| POST | `/admin/settings/password` | Admin | Update admin password and invalidate old JWTs |
| POST | `/admin/config/import` | Admin | Import config (merge/replace) |
| GET | `/admin/config/export` | Admin | Export full config (`config`/`json`/`base64`) |
| POST | `/admin/keys` | Admin | Add API key (optional `name`/`remark`) |
| PUT | `/admin/keys/{key}` | Admin | Update API key metadata |
| DELETE | `/admin/keys/{key}` | Admin | Delete API key |
| GET | `/admin/proxies` | Admin | List proxies |
| POST | `/admin/proxies` | Admin | Add proxy |
| PUT | `/admin/proxies/{proxyID}` | Admin | Update proxy (empty password keeps old secret) |
| DELETE | `/admin/proxies/{proxyID}` | Admin | Delete proxy (auto-unbind referenced accounts) |
| POST | `/admin/proxies/test` | Admin | Test proxy connectivity |
| GET | `/admin/accounts` | Admin | Paginated account list |
| POST | `/admin/accounts` | Admin | Add account |
| PUT | `/admin/accounts/{identifier}` | Admin | Update account name/remark |
| DELETE | `/admin/accounts/{identifier}` | Admin | Delete account |
| PUT | `/admin/accounts/{identifier}/proxy` | Admin | Bind/unbind proxy for an account |
| GET | `/admin/queue/status` | Admin | Account queue status |
| POST | `/admin/accounts/test` | Admin | Test one account |
| POST | `/admin/accounts/test-all` | Admin | Test all accounts |
| POST | `/admin/accounts/sessions/delete-all` | Admin | Delete all sessions for one account |
| POST | `/admin/import` | Admin | Batch import keys/accounts |
| POST | `/admin/test` | Admin | Test API through service |
| POST | `/admin/dev/raw-samples/capture` | Admin | Fire one request and persist it as a raw sample |
| GET | `/admin/dev/raw-samples/query` | Admin | Search current in-memory capture chains by prompt keyword |
| POST | `/admin/dev/raw-samples/save` | Admin | Persist a selected in-memory capture chain as a raw sample |
| POST | `/admin/vercel/sync` | Admin | Sync config to Vercel |
| GET | `/admin/vercel/status` | Admin | Vercel sync status |
| POST | `/admin/vercel/status` | Admin | Vercel sync status / draft compare |
| GET | `/admin/export` | Admin | Export config JSON/Base64 |
| GET | `/admin/dev/captures` | Admin | Read local packet-capture entries |
| DELETE | `/admin/dev/captures` | Admin | Clear local packet-capture entries |
| GET | `/admin/chat-history` | Admin | Read server-side conversation history |
| DELETE | `/admin/chat-history` | Admin | Clear server-side conversation history |
| GET | `/admin/chat-history/{id}` | Admin | Read one server-side conversation entry |
| DELETE | `/admin/chat-history/{id}` | Admin | Delete one server-side conversation entry |
| PUT | `/admin/chat-history/settings` | Admin | Update conversation history retention limit |
| GET | `/admin/version` | Admin | Check current version and latest Release |

OpenAI `/v1/*` paths are canonical. For clients configured with the bare DS2API service URL, the same OpenAI handlers are also exposed through root shortcuts: `/models`, `/models/{id}`, `/chat/completions`, `/responses`, `/responses/{response_id}`, `/embeddings`, `/files`, and `/files/{file_id}`.

---

## Health Endpoints

### `GET /healthz`

```json
{"status": "ok"}
```

### `GET /readyz`

```json
{"status": "ready"}
```

---

## OpenAI-Compatible API

### `GET /v1/models`

No auth required. Returns the currently supported DeepSeek native model list.

**Response**:

```json
{
  "object": "list",
  "data": [
    {"id": "deepseek-v4-flash", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-flash-nothinking", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-pro", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-pro-nothinking", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-flash-search", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-flash-search-nothinking", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-pro-search", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-pro-search-nothinking", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-vision", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-v4-vision-nothinking", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []}
  ]
}
```

> Note: `/v1/models` returns normalized DeepSeek native model IDs. Common aliases are accepted only as request input and are not expanded as separate items in this endpoint.

### Model Alias Resolution

For `chat` / `responses` / `embeddings`, DS2API follows a wide-input/strict-output policy:

1. Match DeepSeek native model IDs first.
2. Then match exact keys in `model_aliases`.
3. If the request name ends with `-nothinking`, resolve the base alias and append the corresponding no-thinking variant.
4. If still unmatched, return `invalid_request_error`. Unknown model families are not guessed heuristically; add explicit compatibility names through `model_aliases`.

Built-in aliases come from `internal/config/models.go`; `config.model_aliases` can override or add mappings at runtime. Excerpt:

- OpenAI / Codex: `gpt-4o`, `gpt-4.1`, `gpt-5`, `gpt-5.5`, `gpt-5-codex`, `gpt-5.3-codex`, `codex-mini-latest`
- OpenAI reasoning: `o1`, `o3`, `o3-deep-research`, `o4-mini`
- Claude: `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5`, `claude-3-5-sonnet-latest`
- Gemini: `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-3.1-pro`, `gemini-3-pro`, `gemini-3-flash`, `gemini-3.1-flash-lite`, `gemini-pro-vision`
- Other exact built-in aliases: `llama-3.1-70b-instruct`, `qwen-max`

Aliases with a `-nothinking` suffix also map to the corresponding forced no-thinking DeepSeek model.

Current vision support resolves only to `deepseek-v4-vision` and does not expose a separate `vision-search` variant.

Retired historical families such as `claude-1.*`, `claude-2.*`, `claude-instant-*`, and `gpt-3.5*` are explicitly rejected.

### `POST /v1/chat/completions`

> Path note: besides the canonical `/v1/chat/completions`, DS2API also accepts the root shortcut `/chat/completions`. On Vercel Runtime, `vercel.json` rewrites only the canonical `/v1/chat/completions` path to the Node streaming bridge; the root shortcut stays on the Go primary path. Use `/v1/chat/completions` on Vercel when real-time streaming is required.

**Headers**:

```http
Authorization: Bearer your-api-key
Content-Type: application/json
```

**Request body**:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | ✅ | DeepSeek native models + common aliases (`gpt-5.5`, `gpt-5.4-mini`, `gpt-5.3-codex`, `o3`, `claude-opus-4-6`, `gemini-2.5-pro`, `gemini-3.1-pro`, `gemini-3-flash`, etc.); `-nothinking` suffixes force thinking / reasoning off |
| `messages` | array | ✅ | OpenAI-style messages |
| `stream` | boolean | ❌ | Default `false` |
| `tools` | array | ❌ | Function calling schema |
| `temperature`, etc. | any | ❌ | Accepted but final behavior depends on upstream |

#### Non-Stream Response

```json
{
  "id": "<chat_session_id>",
  "object": "chat.completion",
  "created": 1738400000,
  "model": "deepseek-v4-pro",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "final response",
        "reasoning_content": "reasoning trace (when thinking is enabled)"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30,
    "completion_tokens_details": {
      "reasoning_tokens": 5
    }
  }
}
```

#### Streaming (`stream=true`)

SSE format: each frame is `data: <json>\n\n`, terminated by `data: [DONE]`.

```text
data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"reasoning_content":"..."},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"content":"..."},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{...}}

data: [DONE]
```

**Field notes**:

- First delta includes `role: assistant`
- When thinking is enabled, the stream may emit `delta.reasoning_content`
- Text emits `delta.content`
- Last chunk includes `finish_reason` and `usage`
- Token counting prefers pass-through from upstream DeepSeek SSE (`accumulated_token_usage` / `token_usage`), and only falls back to local estimation when upstream usage is absent. Failed/interrupted endings (for example `response.failed`) may not include `usage`

#### Tool Calls

When `tools` is present, DS2API performs anti-leak handling:

**Non-stream**: If detected, returns `message.tool_calls`, `finish_reason=tool_calls`, `message.content=null`.

```json
{
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_xxx",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"city\":\"beijing\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ]
}
```

**Stream**: Once high-confidence toolcall features are matched, DS2API emits `delta.tool_calls` immediately (without waiting for full argument closure), then keeps sending argument deltas; confirmed tool-call fragments are not forwarded as `delta.content`.

Additional notes:

- The parser treats the recommended halfwidth-pipe DSML shell tool blocks (`<|DSML|tool_calls>` / `<|DSML|invoke name="...">` / `<|DSML|parameter name="...">`), DSML wrapper aliases (`<dsml|tool_calls>`, `<|tool_calls>`), common DSML separator drift (`<|DSML tool_calls>` / `<|DSML invoke>` / `<|DSML parameter>`), collapsed DSML local names (`<DSMLtool_calls>` / `<DSMLinvoke>` / `<DSMLparameter>`), control-separator drift (`<DSML␂tool_calls>` / raw STX `\x02`), CJK angle bracket, fullwidth-bang / ideographic-comma separator drift, PascalCase local-name drift, and trailing attribute separator drift (`<DSM|parameter name="command"|>...〈/DSM|parameter〉` / `<！DSML！invoke name=“Bash”>` / `<、DSML、tool_calls>` / `<DSmartToolCalls>` / `<DSMLtool_calls※>`), arbitrary protocol prefixes (`<proto💥tool_calls>`), and legacy canonical XML tool blocks (`<tool_calls>` / `<invoke name="...">` / `<parameter name="...">`) as executable tool calls. These shells normalize non-structural separators back to XML first, while internal parsing remains XML-based; CDATA opener drift such as `<！[CDATA[` / `<、[CDATA[` is also normalized for parameter bodies. Legacy `<tools>`, `<tool_call>`, `<tool_name>`, `<param>`, `<function_call>`, `tool_use`, antml variants, and standalone JSON `tool_calls` payloads are treated as plain text; complete but malformed wrappers are also released as plain text.
- The parser no longer drops tool calls solely because parameter values are empty; explicit empty strings or whitespace-only parameters become empty strings in structured `tool_calls`. Prompting still tells the model not to emit blank parameters, and missing/empty argument rejection belongs in the tool executor or client schema validation.
- If the final visible response text is empty but the reasoning stream contains an executable tool call, Chat / Responses emits a standard OpenAI `tool_calls` / `function_call` output during finalization. If thinking/reasoning was not enabled by the client, that reasoning text is used only for detection and is not exposed as visible text or `reasoning_content`.
- `tool_calls` shown inside fenced markdown code blocks (for example, ```json ... ```) are treated as examples, not executable calls.

---

### `GET /v1/models/{id}`

No auth required. Alias values are accepted as path params (for example `gpt-4o`), and the returned object is the mapped DeepSeek model.

### `POST /v1/responses`

OpenAI Responses-style endpoint, accepting either `input` or `messages`.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | ✅ | Supports native models + alias mapping |
| `input` | string/array/object | ❌ | One of `input` or `messages` is required |
| `messages` | array | ❌ | One of `input` or `messages` is required |
| `instructions` | string | ❌ | Prepended as a system message |
| `stream` | boolean | ❌ | Default `false` |
| `tools` | array | ❌ | Same tool detection/translation policy as chat |
| `tool_choice` | string/object | ❌ | Supports `auto`/`none`/`required` and forced function selection (`{"type":"function","name":"..."}`) |

**Non-stream**: Returns a standard `response` object with an ID like `resp_xxx`, and stores it in in-memory TTL cache.
If `tool_choice=required` and no valid tool call is produced, DS2API returns HTTP `422` (`error.code=tool_choice_violation`).

**Stream (SSE)**: minimal event sequence:

```text
event: response.created
data: {"type":"response.created","id":"resp_xxx","status":"in_progress",...}

event: response.output_item.added
data: {"type":"response.output_item.added","response_id":"resp_xxx","item":{"type":"message|function_call",...},...}

event: response.content_part.added
data: {"type":"response.content_part.added","response_id":"resp_xxx","part":{"type":"output_text",...},...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","response_id":"resp_xxx","item_id":"msg_xxx","output_index":0,"content_index":0,"delta":"..."}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","response_id":"resp_xxx","call_id":"call_xxx","delta":"..."}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","response_id":"resp_xxx","call_id":"call_xxx","name":"tool","arguments":"{...}"}

event: response.content_part.done
data: {"type":"response.content_part.done","response_id":"resp_xxx",...}

event: response.output_item.done
data: {"type":"response.output_item.done","response_id":"resp_xxx","item":{"type":"message|function_call",...},...}

event: response.completed
data: {"type":"response.completed","response":{...}}

data: [DONE]
```

If `tool_choice=required` is violated in stream mode, DS2API emits `response.failed` then `[DONE]` (no `response.completed`).

> Current behavior: the parser tries to extract structured tool calls and does not enforce a hard allow-list reject; your tool executor should still validate against a whitelist before executing.

### `GET /v1/responses/{response_id}`

Business auth required. Fetches cached responses created by `POST /v1/responses` (caller-scoped; only the same key/token can read).

> Backed by in-memory TTL store. Default TTL is `900s` (configurable via `responses.store_ttl_seconds`).

### `POST /v1/embeddings`

Business auth required. Returns OpenAI-compatible embeddings shape.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | ✅ | Supports native models + alias mapping |
| `input` | string/array | ✅ | Supports string, string array, token array |

> Requires `embeddings.provider`. Current supported values: `mock` / `deterministic` / `builtin` (all three use the same local deterministic implementation). If missing/unsupported, returns standard error shape with HTTP 501.

### `POST /v1/files`

Business auth required. OpenAI Files-compatible upload endpoint; currently only `multipart/form-data` is supported.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `file` | file | ✅ | Binary payload |
| `purpose` | string | ❌ | Forwarded purpose field |

Constraints and behavior:

- `Content-Type` must be `multipart/form-data` (otherwise `400`).
- Total request size limit is **100 MiB** (over-limit returns `413`).
- Success returns an OpenAI `file` object (`id/object/bytes/filename/purpose/status`, etc.) and includes `account_id` for source-account tracing.

### `GET /v1/files/{file_id}`

Business auth required. Retrieves the current DeepSeek upload status for a file and returns an OpenAI `file` object. Returns `404` when no matching file is found.

---

## Claude-Compatible API

Besides `/anthropic/v1/*`, DS2API also supports shortcut paths: `/v1/messages`, `/messages`, `/v1/messages/count_tokens`, `/messages/count_tokens`.
Implementation-wise this path is unified on the OpenAI Chat Completions parse-and-translate pipeline to avoid maintaining divergent parsing chains.

### `GET /anthropic/v1/models`

No auth required.

**Response**:

```json
{
  "object": "list",
  "data": [
    {"id": "claude-sonnet-4-6", "object": "model", "created": 1715635200, "owned_by": "anthropic"},
    {"id": "claude-haiku-4-5", "object": "model", "created": 1715635200, "owned_by": "anthropic"},
    {"id": "claude-opus-4-6", "object": "model", "created": 1715635200, "owned_by": "anthropic"}
  ],
  "first_id": "claude-opus-4-6",
  "last_id": "claude-3-haiku-20240307",
  "has_more": false
}
```

> Note: the example is partial; besides the current primary aliases, the real response also includes Claude 4.x snapshots plus historical 3.x IDs and common aliases.

### `POST /anthropic/v1/messages`

**Headers**:

```http
x-api-key: your-api-key
Content-Type: application/json
anthropic-version: 2023-06-01
```

> `anthropic-version` is optional; DS2API auto-fills `2023-06-01` when absent.

**Request body**:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | ✅ | For example `claude-sonnet-4-6` / `claude-opus-4-6` / `claude-haiku-4-5` (compatible with `claude-3-5-haiku-latest`), plus historical Claude model IDs |
| `messages` | array | ✅ | Claude-style messages |
| `max_tokens` | number | ❌ | Auto-filled to `8192` when omitted; not strictly enforced by upstream bridge |
| `stream` | boolean | ❌ | Default `false` |
| `system` | string | ❌ | Optional system prompt |
| `tools` | array | ❌ | Claude tool schema |
| `thinking` | object | ❌ | Anthropic thinking config; translated into downstream reasoning control, and ignored by `-nothinking` models |
| `temperature` | number | ❌ | Passed through to the downstream bridge; if `temperature` and `top_p` are both present, `temperature` wins |
| `top_p` | number | ❌ | Passed through when `temperature` is absent |
| `stop_sequences` | array | ❌ | Passed through as downstream stop sequences |
| `tool_choice` | string/object | ❌ | Supports `auto` / `none` / `required` / `{"type":"function","name":"..."}` and is translated to downstream tool choice |

> Note: `thinking`, `temperature`, `top_p`, `stop_sequences`, and `tool_choice` are translated through the compatibility bridge. Final behavior still depends on the selected model and upstream support. When both `temperature` and `top_p` are present, `temperature` takes precedence.

#### Non-Stream Response

```json
{
  "id": "msg_1738400000000000000",
  "type": "message",
  "role": "assistant",
  "model": "claude-sonnet-4-6",
  "content": [
    {"type": "text", "text": "response"}
  ],
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 12,
    "output_tokens": 34
  }
}
```

If tool use is detected, `stop_reason` becomes `tool_use` and `content` contains `tool_use` blocks.

#### Streaming (`stream=true`)

SSE uses paired `event:` + `data:` lines. Event type is also in JSON `type`.

```text
event: message_start
data: {"type":"message_start","message":{...}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: ping
data: {"type":"ping"}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}
```

**Notes**:

- Models that support thinking emit `thinking` blocks / `thinking_delta` by default; explicit thinking disablement or `-nothinking` models suppress them
- `signature_delta` is not emitted (DeepSeek does not provide verifiable thinking signatures)
- In `tools` mode, the stream avoids leaking raw tool JSON and does not force `input_json_delta`

### `POST /anthropic/v1/messages/count_tokens`

**Request**:

```json
{
  "model": "claude-sonnet-4-6",
  "messages": [
    {"role": "user", "content": "Hello"}
  ]
}
```

**Response**:

```json
{
  "input_tokens": 5
}
```

---

## Gemini-Compatible API

Supported paths:

- `/v1beta/models/{model}:generateContent`
- `/v1beta/models/{model}:streamGenerateContent`
- `/v1/models/{model}:generateContent` (compat path)
- `/v1/models/{model}:streamGenerateContent` (compat path)

Authentication is the same as other business routes (`Authorization: Bearer <token>` or `x-api-key`).
Implementation-wise this path is unified on the OpenAI Chat Completions parse-and-translate pipeline to avoid maintaining divergent parsing chains.

### `POST /v1beta/models/{model}:generateContent`

Request body accepts Gemini-style `contents` / `tools`. Model names can use aliases and are mapped to DeepSeek models.

Response uses Gemini-compatible fields, including:

- `candidates[].content.parts[].text`
- `candidates[].content.parts[].thought=true` for thinking output
- `candidates[].content.parts[].functionCall` (when tool call is produced)
- `usageMetadata` (`promptTokenCount` / `candidatesTokenCount` / `totalTokenCount`)

### `POST /v1beta/models/{model}:streamGenerateContent`

Returns SSE (`text/event-stream`), each chunk as `data: <json>`:

- regular text: incremental text chunks
- thinking: incremental chunks with `parts[].thought=true`
- `tools` mode: buffered and emitted as `functionCall` at finalize phase
- final chunk: includes `finishReason: "STOP"` and `usageMetadata`
- Token counting prefers pass-through from upstream DeepSeek SSE (`accumulated_token_usage` / `token_usage`), and only falls back to local estimation when upstream usage is absent

---

## Ollama API

- `POST /api/show` request body: `{"model":"<model-id>"}`.
- Response uses lowercase `id` (not `ID`) and includes `capabilities` for Ollama-style clients and strict schemas.

Example response:

```json
{
  "id": "deepseek-v4-flash",
  "capabilities": ["tools", "thinking"]
}
```

## Admin API

### `POST /admin/login`

Public endpoint.

**Request**:

```json
{
  "admin_key": "admin",
  "expire_hours": 24
}
```

`expire_hours` is optional, default `24`.

**Response**:

```json
{
  "success": true,
  "token": "<jwt>",
  "expires_in": 86400
}
```

### `GET /admin/verify`

Requires JWT: `Authorization: Bearer <jwt>`

**Response**:

```json
{
  "valid": true,
  "expires_at": 1738400000,
  "remaining_seconds": 72000
}
```

### `GET /admin/vercel/config`

Returns Vercel preconfiguration status. Environment variables are preferred, then the saved `vercel` config block is used as a fallback.

```json
{
  "has_token": true,
  "token_preview": "vc****en",
  "token_source": "config",
  "project_id": "prj_xxx",
  "team_id": null
}
```

### `GET /admin/config`

Returns sanitized config, including both `keys` and `api_keys`.

```json
{
  "keys": ["k1", "k2"],
  "api_keys": [
    {"key": "k1", "name": "Primary", "remark": "Production"},
    {"key": "k2", "name": "Backup", "remark": "Load test"}
  ],
  "env_backed": false,
  "env_source_present": true,
  "env_writeback_enabled": true,
  "config_path": "/data/config.json",
  "vercel": {
    "has_token": true,
    "token_preview": "vc****en",
    "project_id": "prj_xxx",
    "team_id": ""
  },
  "accounts": [
    {
      "identifier": "user@example.com",
      "email": "user@example.com",
      "mobile": "",
      "has_password": true,
      "has_token": true,
      "token_preview": "abcde..."
    }
  ],
  "model_aliases": {
    "claude-sonnet-4-6": "deepseek-v4-flash",
    "claude-opus-4-6": "deepseek-v4-pro"
  }
}
```

### `POST /admin/config`

Only updates `keys`, `api_keys`, `accounts`, and `model_aliases`.
If both `api_keys` and `keys` are sent, the structured `api_keys` entries win so `name` / `remark` metadata is preserved; `keys` remains a legacy fallback.

**Request**:

```json
{
  "keys": ["k1", "k2"],
  "api_keys": [
    {"key": "k1", "name": "Primary", "remark": "Production"},
    {"key": "k2", "name": "Backup", "remark": "Load test"}
  ],
  "accounts": [
    {"email": "user@example.com", "password": "pwd", "token": ""}
  ],
  "model_aliases": {
    "claude-sonnet-4-6": "deepseek-v4-flash",
    "claude-opus-4-6": "deepseek-v4-pro"
  }
}
```

### `GET /admin/settings`

Reads runtime settings and status, including:

- `success`
- `admin` (`has_password_hash`, `jwt_expire_hours`, `jwt_valid_after_unix`, `default_password_warning`)
- `runtime` (`account_max_inflight`, `account_max_queue`, `global_max_inflight`, `token_refresh_interval_hours`)
- `responses` / `embeddings`
- `auto_delete` (`mode`: `none` / `single` / `all`; legacy `sessions=true` is still treated as `all`)
- `current_input_file` (`enabled` defaults to `true`, plus `min_chars`)
- `thinking_injection` (`enabled` defaults to `true`, `prompt`, and `default_prompt`)
- `model_aliases`
- `env_backed`, `needs_vercel_sync`
- `toolcall` policy is fixed to `feature_match + high` and is no longer returned or editable via settings

### `PUT /admin/settings`

Hot-updates runtime settings. Supported fields:

- `admin.jwt_expire_hours`
- `runtime.account_max_inflight` / `runtime.account_max_queue` / `runtime.global_max_inflight` / `runtime.token_refresh_interval_hours`
- `responses.store_ttl_seconds`
- `embeddings.provider`
- `auto_delete.mode`
- `current_input_file.enabled` / `current_input_file.min_chars`
- `thinking_injection.enabled` / `thinking_injection.prompt`
- `model_aliases`
- `toolcall` policy is fixed and is no longer writable through settings

### `POST /admin/settings/password`

Updates admin password and invalidates existing JWTs.

Request example:

```json
{"new_password":"your-new-password"}
```

It also accepts `{"password":"your-new-password"}`.

### `POST /admin/config/import`

Imports full config with:

- `mode=merge` (default)
- `mode=replace`

The request can send config directly, or wrapped as `{"config": {...}, "mode":"merge"}`.
Query params `?mode=merge` / `?mode=replace` are also supported.
`replace` mode replaces the full config shape while preserving Vercel sync metadata. `merge` mode merges `keys`, `api_keys`, `accounts`, and `model_aliases`, and overwrites non-empty fields under `admin`, `runtime`, `responses`, and `embeddings`. Manage `auto_delete` and `current_input_file` via `/admin/settings` or the config file; legacy `compat` and `toolcall` fields are ignored.

> Note: `merge` mode does not update `auto_delete` or `current_input_file`.

### `GET /admin/config/export`

Exports full config in three forms: `config`, `json`, and `base64`.

### `POST /admin/keys`

```json
{"key": "new-api-key", "name": "Primary", "remark": "Production"}
```

**Response**: `{"success": true, "total_keys": 3}`

### `PUT /admin/keys/{key}`

Updates the `name` / `remark` of the specified API key. The path `key` is read-only and cannot be changed.

```json
{"name": "Backup", "remark": "Load test"}
```

**Response**: `{"success": true, "total_keys": 3}`

### `DELETE /admin/keys/{key}`

**Response**: `{"success": true, "total_keys": 2}`

### `GET /admin/proxies`

Lists proxy configs (password is never returned; use `has_password` as a marker).

### `POST /admin/proxies`

Adds a proxy. Request accepts `id` (optional; auto-generated when omitted), `name`, `type` (`http` / `socks5`), `host`, `port`, `username`, `password`.

### `PUT /admin/proxies/{proxyID}`

Updates a proxy. If `password` is an empty string, the existing secret is preserved.

### `DELETE /admin/proxies/{proxyID}`

Deletes a proxy and automatically clears `proxy_id` on all accounts that reference it.

### `POST /admin/proxies/test`

Tests proxy connectivity: provide `proxy_id` to test a saved proxy; omit it to run a one-off test using proxy fields in the request body.

### `GET /admin/accounts`

**Query params**:

| Param | Default | Range |
| --- | --- | --- |
| `page` | `1` | ≥ 1 |
| `page_size` | `10` | 1–5000 |
| `q` | empty | Filter by identifier / email / mobile |

**Response**:

```json
{
  "items": [
    {
      "identifier": "user@example.com",
      "email": "user@example.com",
      "mobile": "",
      "has_password": true,
      "has_token": true,
      "token_preview": "abc...",
      "test_status": "ok"
    }
  ],
  "total": 25,
  "page": 1,
  "page_size": 10,
  "total_pages": 3
}
```

Returned items also include `test_status`, usually `ok` or `failed`.

### `POST /admin/accounts`

```json
{"email": "user@example.com", "password": "pwd"}
```

**Response**: `{"success": true, "total_accounts": 6}`

### `PUT /admin/accounts/{identifier}`

Updates the `name` / `remark` of the specified account. The path `identifier` can be email or mobile and cannot be changed.

```json
{"name": "Primary account", "remark": "Shared with the team"}
```

**Response**: `{"success": true, "total_accounts": 6}`

### `DELETE /admin/accounts/{identifier}`

`identifier` can be email, mobile, or the synthetic id for token-only accounts (`token:<hash>`).

**Response**: `{"success": true, "total_accounts": 5}`

### `PUT /admin/accounts/{identifier}/proxy`

Updates proxy binding for a specific account.

- Request body: `{"proxy_id":"..."}`.
- Use empty `proxy_id` to unbind proxy.
- `identifier` supports email / mobile / token-only synthetic id.

### `GET /admin/queue/status`

```json
{
  "available": 3,
  "in_use": 1,
  "total": 4,
  "available_accounts": ["a@example.com"],
  "in_use_accounts": ["b@example.com"],
  "max_inflight_per_account": 2,
  "global_max_inflight": 8,
  "recommended_concurrency": 8,
  "waiting": 0,
  "max_queue_size": 8
}
```

| Field | Description |
| --- | --- |
| `available` | Accounts that still have spare inflight capacity |
| `in_use` | Number of occupied in-flight slots |
| `total` | Total accounts |
| `available_accounts` | List of account IDs with remaining inflight capacity |
| `in_use_accounts` | List of account IDs currently in use |
| `max_inflight_per_account` | Per-account inflight limit |
| `global_max_inflight` | Global inflight limit |
| `recommended_concurrency` | Suggested concurrency (`total × max_inflight_per_account`) |
| `waiting` | Number of queued requests currently waiting |
| `max_queue_size` | Waiting queue limit |

### `POST /admin/accounts/test`

| Field | Required | Notes |
| --- | --- | --- |
| `identifier` | ✅ | email / mobile / token-only synthetic id |
| `model` | ❌ | default `deepseek-v4-flash` |
| `message` | ❌ | if empty, only session creation is tested |

**Response**:

```json
{
  "account": "user@example.com",
  "success": true,
  "response_time": 1240,
  "message": "API test successful (session creation only)",
  "model": "deepseek-v4-flash",
  "session_count": 0,
  "config_writable": true,
  "config_warning": ""
}
```

If a `message` is provided, `thinking` may also be included when the upstream response carries reasoning text.

When the configured file path is not writable (for example, read-only `/app/config.json` inside some containers), login/session testing still proceeds; `config_warning` is returned to indicate token persistence failed and the token is memory-only until restart.

### `POST /admin/accounts/test-all`

Optional request field: `model`.

```json
{
  "total": 5,
  "success": 4,
  "failed": 1,
  "results": [...]
}
```

The internal concurrency limit is currently fixed at 5.

### `POST /admin/accounts/sessions/delete-all`

Deletes all DeepSeek sessions for a specific account. Request example:

```json
{"identifier":"user@example.com"}
```

Response:

```json
{"success": true, "message": "删除成功"}
```

If the account is missing or deletion fails, `success` becomes `false` and `message` contains the error.
The current handler returns the Chinese literal `删除成功` on success.

### `POST /admin/import`

Batch import keys and accounts.

**Request**:

```json
{
  "keys": ["k1", "k2"],
  "accounts": [
    {"email": "user@example.com", "password": "pwd", "token": ""}
  ]
}
```

**Response**:

```json
{
  "success": true,
  "imported_keys": 2,
  "imported_accounts": 1
}
```

### `POST /admin/test`

Test API availability through the service itself.

| Field | Required | Default |
| --- | --- | --- |
| `model` | ❌ | `deepseek-v4-flash` |
| `message` | ❌ | `你好` |
| `api_key` | ❌ | First key in config |

**Response**:

```json
{
  "success": true,
  "status_code": 200,
  "response": {"id": "..."}
}
```

### `POST /admin/dev/raw-samples/capture`

Internally issues one `/v1/chat/completions` request through the service, then persists the request metadata and raw upstream SSE into `tests/raw_stream_samples/<sample-id>/`.

Common request fields:

| Field | Required | Default | Notes |
| --- | --- | --- | --- |
| `message` | No | `你好` | Convenience single-turn user message |
| `messages` | No | Auto-derived from `message` | OpenAI-style message array |
| `model` | No | `deepseek-v4-flash` | Target model |
| `stream` | No | `true` | Recommended to keep streaming enabled so raw SSE is recorded |
| `api_key` | No | First configured key | Business API key to use |
| `sample_id` | No | Auto-generated | Sample directory name |

On success, the response headers include:

- `X-Ds2-Sample-Id`
- `X-Ds2-Sample-Dir`
- `X-Ds2-Sample-Meta`
- `X-Ds2-Sample-Upstream`

If the request itself succeeds but the process did not record a new upstream capture, the endpoint returns:

```json
{"detail":"no upstream capture was recorded"}
```

### `GET /admin/dev/raw-samples/query`

Searches the current process's in-memory capture entries and groups `completion + continue` rounds by `chat_session_id`.

**Query parameters**:

| Param | Default | Notes |
| --- | --- | --- |
| `q` | empty | Fuzzy match against request/response text |
| `limit` | `20` | Max number of chains returned |

**Response fields** include:

- `items[].chain_key`
- `items[].capture_ids`
- `items[].round_count`
- `items[].initial_label`
- `items[].request_preview`
- `items[].response_preview`

### `POST /admin/dev/raw-samples/save`

Persists one selected in-memory capture chain into `tests/raw_stream_samples/<sample-id>/`.

Any one of these selectors is accepted:

```json
{"chain_key":"session:xxxx","sample_id":"tmp-from-memory"}
```

```json
{"capture_id":"cap_xxx","sample_id":"tmp-from-memory"}
```

```json
{"query":"Guangzhou weather","sample_id":"tmp-from-memory"}
```

The success payload includes `sample_id`, `dir`, `meta_path`, and `upstream_path`.

### `POST /admin/vercel/sync`

| Field | Required | Notes |
| --- | --- | --- |
| `vercel_token` | ❌ | If empty or `__USE_PRECONFIG__`, read env, then saved config |
| `project_id` | ❌ | Fallback: `VERCEL_PROJECT_ID`, then saved config |
| `team_id` | ❌ | Fallback: `VERCEL_TEAM_ID`, then saved config |
| `auto_validate` | ❌ | Default `true` |
| `save_credentials` | ❌ | Default `true`; saves explicitly supplied Vercel credentials for the next sync |

**Success response**:

```json
{
  "success": true,
  "validated_accounts": 3,
  "message": "Config synced, redeploying...",
  "deployment_url": "https://..."
}
```

Or manual deploy required:

```json
{
  "success": true,
  "validated_accounts": 3,
  "message": "Config synced to Vercel, please trigger redeploy manually",
  "manual_deploy_required": true
}
```

Failed account checks are returned in `failed_accounts`, and any saved Vercel credentials are returned in `saved_credentials`.

### `GET /admin/vercel/status`

```json
{
  "synced": true,
  "last_sync_time": 1738400000,
  "has_synced_before": true,
  "env_backed": false,
  "config_hash": "....",
  "last_synced_hash": "....",
  "draft_hash": "....",
  "draft_differs": false
}
```

`POST /admin/vercel/status` can also accept `config_override` to compare a draft config against the current synced config.

### `GET /admin/export`

```json
{
  "json": "{...}",
  "base64": "ey4uLn0="
}
```

This is the same payload as `GET /admin/config/export`, just with a shorter path.

### `GET /admin/version`

Checks the current build version and the latest GitHub Release:

```json
{
  "success": true,
  "current_version": "3.0.0",
  "current_tag": "v3.0.0",
  "source": "file:VERSION",
  "checked_at": "2026-03-29T00:00:00Z",
  "latest_tag": "v3.0.0",
  "latest_version": "3.0.0",
  "release_url": "https://github.com/CJackHwang/ds2api/releases/tag/v3.0.0",
  "published_at": "2026-03-28T12:00:00Z",
  "has_update": false
}
```

If GitHub API access fails, the response includes `check_error` while still returning HTTP 200.

### `GET /admin/dev/captures`

Reads local packet-capture status and recent entries (Admin auth required):

- `enabled`
- `limit`
- `max_body_bytes`
- `items`

### `DELETE /admin/dev/captures`

Clears packet-capture entries:

```json
{"success":true,"detail":"capture logs cleared"}
```

---

## Error Payloads

Compatible routes (`/v1/*`, `/anthropic/*`) use the same error envelope:

```json
{
  "error": {
    "message": "...",
    "type": "invalid_request_error",
    "code": "invalid_request",
    "param": null
  }
}
```

Admin routes keep `{"detail":"..."}`.

Gemini routes use Google-style errors:

```json
{
  "error": {
    "code": 400,
    "message": "invalid json",
    "status": "INVALID_ARGUMENT"
  }
}
```

Clients should handle HTTP status code plus `error` / `detail` fields.

**Common status codes**:

| Code | Meaning |
| --- | --- |
| `401` | Authentication failed (invalid key/token, or expired admin JWT) |
| `429` | Too many requests (exceeded inflight + queue capacity, or upstream thinking-only output with no visible answer; managed-account mode first tries one alternate-account fresh retry; current responses do not include `Retry-After`) |
| `503` | Model unavailable or upstream error |

---

## cURL Examples

### OpenAI Non-Stream

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

### OpenAI Stream

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [{"role": "user", "content": "Explain quantum entanglement"}],
    "stream": true
  }'
```

### OpenAI Responses (Stream)

```bash
curl http://localhost:5001/v1/responses \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5-codex",
    "input": "Write a hello world in golang",
    "stream": true
  }'
```

### OpenAI Embeddings

```bash
curl http://localhost:5001/v1/embeddings \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "input": ["first text", "second text"]
  }'
```

### OpenAI with Search

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-flash-search",
    "messages": [{"role": "user", "content": "Latest news today"}],
    "stream": true
  }'
```

### OpenAI Tool Calling

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "What is the weather in Beijing?"}],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather for a city",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string", "description": "City name"}
            },
            "required": ["city"]
          }
        }
      }
    ]
  }'
```

### Gemini Non-Stream

```bash
curl "http://localhost:5001/v1beta/models/gemini-2.5-pro:generateContent" \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Introduce Go in three sentences"}]
      }
    ]
  }'
```

### Gemini Stream

```bash
curl "http://localhost:5001/v1beta/models/gemini-2.5-flash:streamGenerateContent" \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Write a short summary"}]
      }
    ]
  }'
```

### Claude Non-Stream

```bash
curl http://localhost:5001/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### Claude Stream

```bash
curl http://localhost:5001/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-opus-4-6",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Explain relativity"}],
    "stream": true
  }'
```

### Admin Login

```bash
curl http://localhost:5001/admin/login \
  -H "Content-Type: application/json" \
  -d '{"admin_key": "admin"}'
```

### Pin Specific Account

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "X-Ds2-Target-Account: user@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```
