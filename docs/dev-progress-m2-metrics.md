# M2 Tool Parser Metrics — 开发进度文档

> 临时工作文档，合并后可删除。

## 目标

`dev-plan-toolparser.md §2.3` 要求的可观测性指标写入结构化日志，以及 WebUI Admin 只读展示 flag 状态。

## PR 拆分

| PR   | 分支                          | 内容                                          | 状态     |
|------|-------------------------------|-----------------------------------------------|----------|
| #8   | `feat/m2-toolparser-flag`     | parser.v2 三态 config                         | ✅ 已合并 |
| #9   | `feat/m2-toolparser-shadow-diff` | shadow diff 全链路接入                      | ✅ 已合并 |
| #10  | `fix/toolparser-shadow-diff-bugs` | 修复 thinking-path 偽 diff + 正规化不对称  | ✅ 已合并 |
| PR-3a | `feat/m2-toolparser-metrics` | Go 后端：observe 扩展 + sieve 统计 + Admin API | 🔄 当前   |
| PR-3b | `feat/m2-toolparser-webui`   | WebUI：flag 只读展示                          | ⏳ 待开发 |

---

## PR-3a 详细任务清单

### Step 1 — 扩展 `internal/observe/request_metrics.go`

新增字段：
```
ParserV2Mode        string  // parser.v2 当前模式（off/shadow/enforce）
ToolCallCount       int     // 本次请求最终检测到的 tool call 数
ShadowDiffRanCount  int     // RunShadowDiff 实际执行次数（mode=shadow）
ShadowDiffHitCount  int     // shadow diff 发现差异次数
SuppressedCharCount int     // sieve 拦截未下发的字节数（代理 token 数）
```

新增 context helpers（nil-safe）：
- `SetParserV2Mode(ctx, mode)`
- `AddToolCallCount(ctx, n)`
- `IncrShadowDiffRan(ctx)`
- `IncrShadowDiffHit(ctx)`
- `AddSuppressedCharCount(ctx, n)`

- [ ] 完成

### Step 2 — 更新 `internal/observe/middleware.go`

`[completion_request]` slog 行追加：
```
"parser_v2_mode", parserV2Mode,
"parser_tool_call_count", toolCallCount,
"parser_shadow_diff_ran", shadowDiffRanCount,
"parser_shadow_diff_hit", shadowDiffHitCount,
"parser_suppressed_char_count", suppressedCharCount,
```

- [ ] 完成

### Step 3 — `assistantturn.BuildOptions` 加 `Ctx context.Context`

- 在 `BuildOptions` 加 `Ctx context.Context`（nil-safe，不传时静默跳过）
- `BuildTurnFromCollected`：捕获 `RunShadowDiff` 返回值，写入 observe
- `BuildTurnFromStreamSnapshot`：同上
- 约 8 处 handler 调用点加 `Ctx: r.Context()`（chat/responses/claude/gemini × stream + nonstream）

涉及文件：
- `internal/assistantturn/turn.go`
- `internal/httpapi/openai/chat/handler_chat.go`
- `internal/httpapi/openai/chat/empty_retry_runtime.go`（stream runtime 内部）
- `internal/httpapi/openai/responses/responses_handler.go`
- `internal/httpapi/openai/responses/empty_retry_runtime.go`
- `internal/httpapi/claude/handler_messages.go`
- `internal/httpapi/gemini/handler_stream_runtime.go`
- `internal/completionruntime/nonstream.go`

- [ ] 完成

### Step 4 — `toolstream.State` 追踪 suppressed 字节数

- `internal/toolstream/tool_sieve_state.go`（或 core）：加 `suppressedCharCount int`
- `internal/toolstream/tool_sieve_core.go`：原 `_ = captured` 改为 `state.suppressedCharCount += len(captured)`
- 新增 `State.SuppressedCharCount() int`
- 各 stream runtime `finalize` 末尾：`observe.AddSuppressedCharCount(ctx, sieve.SuppressedCharCount())`

涉及文件：
- `internal/toolstream/tool_sieve_state.go`
- `internal/toolstream/tool_sieve_core.go`
- `internal/httpapi/openai/chat/chat_stream_runtime.go`
- `internal/httpapi/openai/responses/responses_stream_runtime_core.go`
- `internal/httpapi/claude/stream_runtime_core.go`
- `internal/httpapi/gemini/handler_stream_runtime.go`

- [ ] 完成

### Step 5 — Admin Settings API 加只读字段

- `internal/httpapi/admin/settings/handler_settings_read.go`：追加 `"parser_v2"` key
- `internal/httpapi/admin/settings/deps.go`：`ConfigReader` 接口加 `ParserV2Mode() string`
- 相关 test mock 加 `ParserV2Mode()`

- [ ] 完成

---

## PR-3a DoD

- [ ] `go build ./...` 通过
- [ ] `go test ./...` 全绿（含 observe/toolstream/admin 新测试）
- [ ] `./scripts/lint.sh` 0 issues
- [ ] `./tests/scripts/run-unit-all.sh` 通过
- [ ] `[completion_request]` 日志含 5 个 parser 字段
- [ ] Admin `GET /admin/settings` 返回 `parser_v2.mode`

---

## PR-3b 详细任务清单

### Step 1 — `webui/src/features/settings/ParserFlagsSection.jsx`

只读区块，badge 样式：
- `off` → 灰色 badge
- `shadow` → 蓝色 badge
- `enforce` → 绿色 badge
- 环境变量覆盖时显示提示文字

### Step 2 — 接入 form

- `useSettingsForm.js`：`DEFAULT_FORM` + `fromServerForm` 加 `parser_v2`
- `SettingsContainer.jsx`：`<RuntimeSection>` 后渲染 `<ParserFlagsSection>`

### Step 3 — i18n

中英文各加：
- `settings.parserFlagsTitle`
- `settings.parserV2Mode`
- `settings.parserV2ModeOff / Shadow / Enforce`
- `settings.parserV2EnvOverride`

---

## 字段命名对照（与 governance 计划对齐）

| 日志字段                      | 来源                          |
|-------------------------------|-------------------------------|
| `parser_v2_mode`              | `BuildOptions.ParserV2Mode`   |
| `parser_tool_call_count`      | `len(calls)` in BuildTurn*    |
| `parser_shadow_diff_ran`      | `ShadowDiffRecord.Ran` 累计   |
| `parser_shadow_diff_hit`      | `ShadowDiffRecord.HasDiff` 累计|
| `parser_suppressed_char_count`| `toolstream.State` capture 累计|
