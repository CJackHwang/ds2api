# 修复: 模型思维链中出现工具调用时报错

## 上下文

DeepSeek 模型推理时可能将 `<tool_calls>` XML 发送在 SSE 的 **THINK 片段**中（而非 RESPONSE 片段）。当前代码仅在 **text 内容**中检测工具调用，忽略 thinking，导致：

| 场景 | 表现 | 影响 |
|------|------|------|
| 流式 + 全部在 thinking | `finalize()` 触发 "empty output" 429 错误 | **致命 — 必报错** |
| 流式 + 部分在 text | 工具调用漏检，`reasoning_content` 泄露 XML | 功能缺失 |
| 非流式 | 工具调用静默丢失，`reasoning_content` 泄露 XML | 功能缺失 |

**受影响路径**：
- `/v1/chat/completions` (stream & non-stream)
- `/v1/responses` (stream & non-stream) — **同样存在**

## 根因（已由 Codex 审核确认）

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| 1 | `sse/parser.go` | 71-107, 280-325 | `ParseSSEChunkForContent()` 只按 path/type 分 thinking/text；`splitThinkingParts()` 只识别 `</think>` |
| 2 | `chat_stream_runtime.go` | 136 | `finalize()` 仅对 `s.text` 做工具调用解析 |
| 3 | `chat_stream_runtime.go` | 255-263 | `onParsed()` 中 thinking 不走 tool sieve |
| 4 | `render_chat.go` | 10 | `BuildChatCompletion()` 仅对 `finalText` 解析 |
| 5 | `handler_chat.go` | 165 | `shouldWriteUpstreamEmptyOutputError(finalText)` |
| 6 | `chat_stream_runtime.go` | 204 | `finalize()` 中 `finalText == ""` 门禁 |
| 7 | `responses_handler.go` | 124 | `/v1/responses` 同样问题 |
| 8 | `responses_stream_runtime_core.go` | 128 | `/v1/responses` stream 同样问题 |
| 9 | `render_responses.go` | 12 | `/v1/responses` non-stream 同样问题 |
| 10 | `leaked_output_sanitize.go` | 39-52 | 未清理 `<tool_calls>` XML |

## 修复方案

### 第一阶段：核心修复（chat + responses 平行修复）

#### 1. `chat_stream_runtime.go:131-220` — finalize() 流式路径修复
```go
// 第 136 行后：fallback 检测 thinking 中的工具调用
detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, s.toolNames)
if len(detected.Calls) == 0 {
    detected = toolcall.ParseStandaloneToolCallsDetailed(finalThinking, s.toolNames)
    if len(detected.Calls) > 0 {
        // 清理 thinking 中的 XML，防止泄露
        finalThinking = cleanToolCallXML(finalThinking)
        s.finalThinking = finalThinking
    }
}
```
- 修改第 204 行空输出检查：加上 `len(detected.Calls) == 0` 的条件
- 找到工具调用后正常走第 137-154 行 emit 逻辑

#### 2. `handler_chat.go:165` — handleNonStream() 非流式修复
```go
// 第 165 行之前增加
if shouldWriteUpstreamEmptyOutputError(finalText) {
    // fallback: 检查 thinking 中是否有工具调用
    detected := toolcall.ParseStandaloneToolCallsDetailed(finalThinking, toolNames)
    if len(detected.Calls) > 0 {
        // 不报空输出错误，让 BuildChatCompletion 处理
    } else {
        // 原逻辑 — 报 429
    }
}
```

#### 3. `render_chat.go:9-30` — BuildChatCompletion() 增加 thinking fallback
```go
detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, toolNames)
if len(detected.Calls) == 0 {
    detected = toolcall.ParseStandaloneToolCallsDetailed(finalThinking, toolNames)
    if len(detected.Calls) > 0 {
        finalThinking = cleanToolCallXML(finalThinking)
    }
}
```

#### 4. `chat_stream_runtime.go:255-263` — onParsed() 流式 thinking 过滤
```go
// 发送 reasoning_content delta 前检测并过滤工具调用 XML
// 如果 cleanedText 包含 <tool_calls>，送入 toolstream 或跳过
// 不发送裸 XML 给客户端
```

#### 5. `leaked_output_sanitize.go:39-52` — 增加工具调用 XML 清理
```go
var leakedToolCallXMLPattern = regexp.MustCompile(`(?is)<tool_calls>.*?</tool_calls>`)
// 在 sanitizeLeakedOutput() 中调用（需在工具调用检测之后）
```

#### 6. `/v1/responses` 路径同步修复
- `responses_handler.go:124` — `handleNonStreamResponses()`
- `responses_stream_runtime_core.go:128` — `finalizeResponses()`
- `render_responses.go:12` — `BuildResponseObject()`

### 第二阶段：回归测试

#### 7. `handler_toolcall_test.go` — 新增测试用例
- `response/thinking_content` 含 `<tool_calls>` XML → 断言 `finish_reason="tool_calls"`，不再 429
- `response/thinking_content` 混合文本 + `<tool_calls>` → 断言正确解析
- 确保 `reasoning_content` 不含裸 XML

## 修改文件清单

| 文件 | 修改类型 |
|------|----------|
| `internal/httpapi/openai/chat/chat_stream_runtime.go` | 核心修改 |
| `internal/httpapi/openai/chat/handler_chat.go` | 核心修改 |
| `internal/format/openai/render_chat.go` | 核心修改 |
| `internal/httpapi/openai/shared/leaked_output_sanitize.go` | 防泄漏补充 |
| `internal/httpapi/openai/responses/handler.go` | 对称修复 |
| `internal/httpapi/openai/responses/responses_stream_runtime_core.go` | 对称修复 |
| `internal/format/openai/render_responses.go` | 对称修复 |
| `internal/httpapi/openai/chat/handler_toolcall_test.go` | 回归测试 |

## 验证方法

```bash
# 1. 编译验证
cd D:\Source\ds2api && go build ./...

# 2. 运行现有测试
go test ./internal/httpapi/openai/chat/...

# 3. 手动测试（用 raw_capture 或 curl）
# 构造 thinking 含 <tool_calls> 的 SSE 流，验证不再 429

# 4. 检查 reasoning_content 不含 XML
```
