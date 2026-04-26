# 修复: 模型思维链中出现工具调用时报错

## 问题概述

DeepSeek 模型在推理过程中（thinking/reasoning）输出工具调用时，SSE 片段类型为 `THINK` 而非 `RESPONSE`，导致 ds2api 无法检测到工具调用并报 "empty output" 错误。

## 根因

1. SSE 解析器 `splitThinkingParts()` 仅按 `</think>` 分割 thinking/text，不处理 `<tool_calls>`
2. 流式路径 `finalize()` 仅对 `s.text` 做工具调用解析，忽略 `s.thinking`
3. 非流式路径 `BuildChatCompletion()` 仅对 `finalText` 解析
4. 空输出检查只认 `finalText == ""`，未考虑 thinking 中的工具调用
5. `sanitizeLeakedOutput()` 未清理 `<tool_calls>` XML
6. `/v1/responses` 路径同样存在此问题

## 影响范围

- 流式模式: 工具调用出现在 thinking 中时 → 429 "empty output" 错误
- 非流式模式: 工具调用静默丢失
- 所有路径: reasoning_content 泄露原始 XML

## 修复步骤

### 步骤 1: chat_stream_runtime.go — finalize() 增加 thinking fallback

在 `finalize()` 中，如果 `finalText` 未检测到工具调用，则对 `finalThinking` 执行 fallback 检测:

```go
detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, s.toolNames)
if len(detected.Calls) == 0 {
    detected = toolcall.ParseStandaloneToolCallsDetailed(finalThinking, s.toolNames)
    if len(detected.Calls) > 0 {
        finalThinking = cleanToolCallXML(finalThinking)
        s.finalThinking = finalThinking
    }
}
```

### 步骤 2: handler_chat.go — handleNonStream() 增加 thinking fallback

在 `shouldWriteUpstreamEmptyOutputError(finalText)` 之前增加 thinking 工具调用检测:

```go
if shouldWriteUpstreamEmptyOutputError(finalText) {
    detected := toolcall.ParseStandaloneToolCallsDetailed(finalThinking, toolNames)
    if len(detected.Calls) > 0 {
        finalThinking = cleanToolCallXML(finalThinking)
        // 继续执行 BuildChatCompletion
    } else {
        // 原空输出错误逻辑
    }
}
```

### 步骤 3: render_chat.go — BuildChatCompletion() 增加 thinking fallback

```go
detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, toolNames)
if len(detected.Calls) == 0 {
    detected = toolcall.ParseStandaloneToolCallsDetailed(finalThinking, toolNames)
    if len(detected.Calls) > 0 {
        finalThinking = cleanToolCallXML(finalThinking)
    }
}
```

### 步骤 4: leis_resource.go — 添加 cleanToolCallXML 清理

添加 `cleanToolCallXML()` 函数，用正则去除 `<tool_calls>...</tool_calls>` 块。

### 步骤 5: leaked_output_sanitize.go — 增加 XML 清理规则

新增 `leakedToolCallXMLPattern`，防止任何路径泄露原始工具调用 XML。

### 步骤 6: /v1/responses 路径同步修复

- `responses_handler.go`: `handleNonStreamResponses()` 同样修复
- `responses_stream_runtime_core.go`: `finalizeResponses()` 同样修复
- `render_responses.go`: `BuildResponseObject()` 同样修复

### 步骤 7: 回归测试

在 `handler_toolcall_test.go` 中添加 thinking 含工具调用的测试用例。

## 验证方法

```bash
go build ./...
go test ./internal/httpapi/openai/chat/...
go test ./internal/httpapi/openai/responses/...
```
