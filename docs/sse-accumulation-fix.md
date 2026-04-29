# SSE 累积缓冲区修复文档

## 概述

本文档记录 `internal/sse/stream.go` 中 SSE 流式输出累积缓冲区功能的完整实现与修复过程。当上游 DeepSeek SSE 流产生过多小块输出（500+ chunks）时，此功能通过累积文本再批量刷新，将输出块数减少到 ~15-30。

## 修改文件

| 文件 | 修改内容 |
|------|----------|
| `internal/sse/stream.go` | 核心累积逻辑实现与缺陷修复 |
| `internal/sse/stream_test.go` | 新增累积功能单元测试 |
| `internal/sse/stream_edge_test.go` | 适配累积默认行为 |

## 功能设计

### 核心结构体

```go
// AccumulateConfig 定义 SSE 输出缓冲配置
type AccumulateConfig struct {
    Enabled       bool          // 是否启用累积
    MinChars      int           // 触发刷新的最小字符数（默认 150）
    MaxWait       time.Duration // 最大等待时间（默认 80ms）
    FlushOnFinish bool          // 流结束时强制刷新
}
```

### 默认配置

```go
func DefaultAccumulateConfig() AccumulateConfig {
    return AccumulateConfig{
        Enabled:       true,
        MinChars:      150,
        MaxWait:       80 * time.Millisecond,
        FlushOnFinish: true,
    }
}
```

### 函数接口

| 函数 | 说明 |
|------|------|
| `StartParsedLinePump(...)` | 公共入口，使用默认累积配置 |
| `startParsedLinePumpWithConfig(...)` | 内部函数，支持自定义配置 |

## 已知问题与修复记录

### ISS-008 [高] SSE chunk 多 choice 导致 Hermes Agent 等客户端遗漏 content

**问题描述**：
当累积缓冲区 flush 产生一个包含 thinking 和 text 两种 Parts 的 `LineResult` 时，`onParsed` 方法会为每个 Part 创建独立的 choice，导致单个 SSE chunk 包含多个 choices：

```json
{"choices":[
  {"delta":{"reasoning_content":"完整思考...", "role":"assistant"}, "index":0},
  {"delta":{"content":"Hello! How can I assist you today?"}, "index":0}
]}
```

部分客户端（如 Hermes Agent）只处理 `choices[0].delta`，会遗漏 `choices[1]` 中的 content。

**修复方案**：
将同一 `LineResult` 的所有 Parts 合并到同一个 delta 对象中，生成单一 choice：

```json
{"choices":[
  {"delta":{
    "reasoning_content":"完整思考...",
    "content":"Hello! How can I assist you today?",
    "role":"assistant"
  }, "index":0}
]}
```

**修改文件**：`internal/httpapi/openai/chat/chat_stream_runtime.go` 的 `onParsed` 方法

**同时修复**：`ToolDetectionThinkingParts` 未通过累积缓冲区传递，导致 tool call 检测失败。在 `flushBuffer` 和累积循环中新增 `toolDetectionThinkingBuffer` 收集和传递这些 parts。

---

### ISS-001 [致命] flushBuffer 闭包中 done 写入导致 goroutine 泄漏

**问题描述**：
`flushBuffer` 闭包在 context 取消时直接向 `done` channel 写入错误后 return。但 goroutine 末尾还有 `done <- scanner.Err()`，如果 flushBuffer 已经写入过 done，末尾写入将永久阻塞，导致 `defer close(out)` 不执行，消费者永久阻塞。

**修复方案**：
引入 `pumpErr` 变量在闭包中收集错误，只在 goroutine 末尾统一写入一次：

```go
// goroutine 开头声明
var pumpErr error

// flushBuffer 中 ctx.Done 分支
case <-ctx.Done():
    pumpErr = ctx.Err()
    return  // 不直接写 done

// goroutine 末尾
if pumpErr != nil {
    done <- pumpErr
} else {
    done <- scanner.Err()
}
```

**影响范围**：所有 ctx.Done 分支（flushBuffer、Stop 处理、非累积模式）

---

### ISS-003 [高] Stop 信号处理中内容重复发送

**问题描述**：
Stop 信号到达时，`result.Parts` 先被追加到 buffer 并通过 `flushBuffer` 发送，随后又通过 `out <- result` 发送原始 result，导致消费者收到两份相同内容。

**修复方案**：
flush 后发送一个只带 Stop 标志的空 LineResult，不包含 Parts：

```go
stopResult := LineResult{
    Parsed:   true,
    Stop:     true,
    NextType: currentType,
    // Parts 不再发送，已包含在 flush 中
}
select {
case out <- stopResult:
case <-ctx.Done():
    pumpErr = ctx.Err()
    return
}
```

---

### ISS-004 [高] pendingType 在 thinking/text 切换时被错误覆盖

**问题描述**：
单一 `pendingType` 变量同时服务 textBuffer 和 thinkingBuffer，在交替场景下可能产生类型标记错误。

**修复方案**：
拆分为两个独立变量：

```go
var textPendingType string
var thinkingPendingType string

// flushBuffer 中
if thinkingLen > 0 {
    parts = append(parts, ContentPart{Text: thinkingBuffer.String(), Type: thinkingPendingType})
}
if textLen > 0 {
    parts = append(parts, ContentPart{Text: textBuffer.String(), Type: textPendingType})
}

// 累积逻辑中
if p.Type == "thinking" {
    thinkingBuffer.WriteString(p.Text)
    thinkingPendingType = "thinking"
} else {
    textBuffer.WriteString(p.Text)
    textPendingType = p.Type
}
```

---

### ISS-005 [中] 多余的字节切片拷贝

**问题描述**：
`line := append([]byte{}, scanner.Bytes()...)` 创建多余拷贝，`scanner.Bytes()` 返回的切片在下一次 `Scan()` 前是安全的。

**修复方案**：
```go
result := ParseDeepSeekContentLine(scanner.Bytes(), thinkingEnabled, currentType)
```

---

### ISS-006 [低] StartParsedLinePumpWithConfig 缺乏外部调用者

**问题描述**：
该函数仅被 `StartParsedLinePump` 内部调用，无业务侧自定义配置入口。

**修复方案**：
改为非导出函数 `startParsedLinePumpWithConfig`，避免不必要的 API 暴露。

---

### ISS-007 [低] 新增累积功能无单元测试

**新增测试用例**：

| 测试名 | 验证内容 |
|--------|----------|
| `TestAccumulateFlushOnMinChars` | 累积到 MinChars 时正确 flush |
| `TestAccumulateDisabled` | 禁用累积时保持原有即时发送行为 |
| `TestFlushOnFinish` | 流结束时 flush 剩余内容 |
| `TestContextCancellation` | context 取消时 goroutine 正确退出 |

## 核心逻辑说明

### 刷新条件

`flushBuffer` 在以下任一条件满足时执行刷新：

1. **强制刷新**：`force == true`（流结束或收到 Stop 信号）
2. **字符阈值**：`textLen >= cfg.MinChars`（默认 150 字符）
3. **混合内容**：`thinkingLen > 0 && textLen >= 50`（有思考内容且文本达 50 字符）
4. **超时刷新**：`elapsed >= cfg.MaxWait && (textLen > 0 || thinkingLen > 0)`（默认 80ms）

### 内容分离策略

- thinking 内容到达时：先 flush text buffer，再累积 thinking
- text 内容到达时：直接累积到 textBuffer
- flush 时 thinking 和 text 分别作为独立的 ContentPart 发送

### Stop 信号处理流程

```
收到 Stop → 将 result.Parts 追加到 buffer → flushBuffer(true) → 发送空 stopResult
```

## 重新应用指南

当上游更新覆盖了此修复时，按以下步骤重新应用：

### 1. 确认需要修复的文件

检查以下文件是否存在对应特征：

| 文件 | 检查特征 |
|------|----------|
| `internal/sse/stream.go` | 有 `AccumulateConfig`、`startParsedLinePumpWithConfig`、`pumpErr`、`textPendingType`、`thinkingPendingType`、`toolDetectionThinkingBuffer` |
| `internal/httpapi/openai/chat/chat_stream_runtime.go` | `onParsed` 方法中使用 `mergedDelta` 合并所有 delta 字段 |

如果缺失，说明修复被覆盖。

### 2. 对比本文档中的修复方案

逐一对照 ISS-001 到 ISS-007 的修复方案，确认每个问题都已处理。

### 3. 运行测试验证

```bash
cd /Users/wang/Desktop/project/ds2api/ds2api-dev

# SSE 累积测试
go test -v ./internal/sse/ -run "TestAccumulate|TestFlushOnFinish|TestContextCancellation"

# OpenAI chat 流测试（含 tool call 和 multi-choice 合并）
go test -v ./internal/httpapi/openai/chat/ -run "ToolCall|Stream|Thinking|Status"
```

### 4. 编译验证

```bash
GOOS=linux GOARCH=amd64 go build -o ds2api ./cmd/ds2api
file ds2api
```

预期输出：`ELF 64-bit LSB executable, x86-64`

## 测试运行

完整测试命令：

```bash
# 运行 SSE 包所有测试
go test -v -timeout 60s ./internal/sse/

# 仅运行累积相关测试
go test -v -timeout 60s ./internal/sse/ -run "TestAccumulate|TestFlushOnFinish|TestContextCancellation"

# 运行 lint
./scripts/lint.sh
```

## 版本历史

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-04-29 | v1.0 | 初始实现，修复 ISS-001 到 ISS-007 |
| 2026-04-29 | v1.1 | 修复 ISS-008：SSE chunk 多 choice 兼容性问题，合并 delta 到单一 choice；新增 toolDetectionThinkingBuffer 累积 |

## 注意事项

1. **不要覆盖 config.json**：部署时保留服务器原有配置
2. **Go 版本要求**：>= 1.21
3. **兼容性**：累积功能默认启用，现有测试需要显式设置 `Enabled: false` 来测试原始即时发送行为
4. **性能影响**：累积会增加约 80ms 的延迟，但显著减少 SSE 块数量
