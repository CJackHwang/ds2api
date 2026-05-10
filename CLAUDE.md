# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

DS2API 将 DeepSeek Web 对话能力转换为 OpenAI、Claude 与 Gemini 兼容 API。核心后端以 Go 实现，Vercel 流式桥接使用 Node Runtime，前端为 React WebUI 管理台。

## 开发命令

### 本地运行
```bash
go run ./cmd/ds2api
```

### Lint 和格式化
```bash
./scripts/lint.sh                    # Go 格式化 + golangci-lint
gofmt -w <files>                     # 格式化单个文件
```

### 测试
```bash
./tests/scripts/run-unit-all.sh      # 单元测试（Go + Node，无需真实账号）
./tests/scripts/run-unit-go.sh       # 仅 Go 单元测试
./tests/scripts/run-unit-node.sh     # 仅 Node 单元测试
./tests/scripts/run-live.sh          # 端到端全链路测试（需要真实账号）
./tests/scripts/check-cross-build.sh # Release 目标交叉编译检查
```

### 运行特定模块的测试
```bash
go test -v -run TestParseToolCalls ./internal/toolcall
go test -v ./internal/format/...
go test -v ./internal/httpapi/openai/...
```

### WebUI 构建
```bash
npm run build --prefix webui
./scripts/build-webui.sh
```

## PR 门禁

打开或更新 PR 前必须运行：
```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
```

## 架构要点

### 请求主链路

Client → chi Router → HTTP API surface (OpenAI/Claude/Gemini/Admin) → PromptCompat → CompletionRuntime → DeepSeek upstream

```
HTTP API surfaces (openai/{chat,responses,files,embeddings}, claude, gemini)
         ↓
PromptCompat (API → DeepSeek 网页纯文本上下文)
         ↓
Prompt assembly + Auth + Account Pool
         ↓
CompletionRuntime (session/PoW/completion 启动、空输出 retry)
         ↓
Stream → AssistantTurn (归一化 thinking/tool call/usage)
         ↓
Format (OpenAI/Claude 输出格式化) → Client
```

### 核心模块

- `internal/server` - 路由树和中间件
- `internal/httpapi/` - HTTP 协议适配层
- `internal/promptcompat` - API 请求到 DeepSeek 上下文的兼容内核
- `internal/completionruntime` - Go 主路径共享的 completion 执行
- `internal/assistantturn` - 上游输出归一化为统一语义
- `internal/stream` + `internal/sse` - 流式解析与增量处理
- `internal/toolcall` + `internal/toolstream` - DSML/XML 工具调用解析与防泄漏
- `internal/deepseek/client` - DeepSeek 上游调用
- `internal/account` - 托管账号池、并发槽位、等待队列
- `api/` - Vercel Serverless 入口（Go/Node）

### 协议适配边界原则

**不要**让 OpenAI Chat、OpenAI Responses、Claude、Gemini 或其他接口协议格式化拥有共享业务行为。

必须先将协议特定的请求形状归一化为项目标准 request/turn 模型，在统一位置运行共享业务逻辑，再在边界处渲染回目标协议。

需要保持全局一致的行为包括：空输出 retry、thinking/reasoning 处理、tool-call 检测与策略、usage 统计、current-input-file 注入、history 持久化、file/reference 处理、completion payload 组装。

### Vercel 流式处理

Vercel 上的 `/v1/chat/completions` 由 `api/chat-stream.js`（Node Runtime）承接。Go 负责 prepare/release（鉴权、账号租约、completion payload），Node 侧负责实时 SSE 转发并保持与 Go 一致的 tool sieve 语义。

### WebUI

- 源码：`webui/`（Vite + React）
- 构建产物：`static/admin/`
- 本地首次启动时若 `static/admin` 缺失会自动构建

## 文档同步规则

当业务逻辑或用户可见行为变更时，必须在同一次变更中更新对应文档。

`docs/prompt-compatibility.md` 是"API → 网页纯文本上下文"兼容流程的权威文档。如果变更影响消息归一化、tool prompt 注入、prompt 可见工具历史、file/reference 处理、history 拆分或 completion payload 组装，必须同时更新该文档。

## Tool Call 解析

解析层以"尽量解析成功"为优先，所有格式合法的 XML 工具调用都会通过，不做工具名 allow-list 过滤。DSML 只是外壳别名，内部仍以 XML 解析语义为准。

详细语义说明见 `docs/toolcall-semantics.md`。

## 目录结构

```
├── api/              # Serverless 入口（Vercel Go/Node）
├── app/              # 应用级 handler 装配层
├── cmd/              # 可执行程序入口
│   ├── ds2api/       # 主服务
│   └── ds2api-tests/ # E2E 测试 CLI
├── internal/         # 核心业务实现
│   ├── httpapi/      # HTTP surface（openai/claude/gemini/admin）
│   ├── promptcompat/ # API → DeepSeek 上下文兼容层
│   ├── completionruntime/ # completion 执行辅助
│   ├── assistantturn/     # 输出语义归一
│   ├── deepseek/    # 上游 client/protocol/transport
│   └── ...
├── pow/              # PoW 独立实现
├── tests/            # 测试资源与脚本
│   ├── scripts/      # 测试入口脚本
│   ├── raw_stream_samples/ # 原始 SSE 样本
│   └── node/         # Node 单元测试
└── webui/            # React 管理台源码
```

完整架构图见 `docs/ARCHITECTURE.md`。