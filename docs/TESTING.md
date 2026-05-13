# DS2API 测试指南

语言 / Language: 中文 + English（同页）

文档导航： [总览](../README.MD) / [架构说明](./ARCHITECTURE.md) / [部署指南](./DEPLOY.md) / [接口文档](../API.md)

## 概述 | Overview

DS2API 提供两个层级的测试：

| 层级 | 命令 | 说明 |
| --- | --- | --- |
| 单元测试（Go） | `./tests/scripts/run-unit-go.sh` | 不需要真实账号 |
| 单元测试（Node） | `./tests/scripts/run-unit-node.sh` | 不需要真实账号 |
| 单元测试（全部） | `./tests/scripts/run-unit-all.sh` | 不需要真实账号 |
| Release 目标交叉编译 | `./tests/scripts/check-cross-build.sh` | 覆盖发布包支持的 GOOS/GOARCH |
| 端到端测试 | `./tests/scripts/run-live.sh` | 使用真实账号执行全链路测试 |

端到端测试集会录制完整的请求/响应日志，用于故障排查。
Node 单元测试脚本会先做 `node --check` 语法门禁，再以 `--test-concurrency=1` 串行执行测试文件，减少模块级共享状态带来的干扰。

---

## PR 门禁 | PR Gates

打开或更新 PR 前，按 `.github/workflows/quality-gates.yml` 的同等本地门禁执行：

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
```

说明：

- `./scripts/lint.sh` 会运行 Go 格式化检查和 `golangci-lint`；修改 Go 文件后仍建议先执行 `gofmt -w <files>`。
- `run-unit-all.sh` 串行调用 Go 与 Node 单元测试入口。
- CI 还会额外在 macOS/Windows 跑 Go 单测，并执行 release 目标交叉编译检查。
- `run-live.sh` 是真实账号端到端测试，适合作为发布或高风险改动后的补充验证，不属于每次 PR 的固定本地门禁。

---

## 快速开始 | Quick Start

### 单元测试 | Unit Tests

```bash
./tests/scripts/run-unit-all.sh
```

```bash
# 或按语言拆分执行
./tests/scripts/run-unit-go.sh
./tests/scripts/run-unit-node.sh
```

```bash
# 结构与流程门禁
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/check-node-split-syntax.sh
./tests/scripts/check-cross-build.sh
```

说明：`plans/stage6-manual-smoke.md` 已移除，阶段 6 手工烟测不再作为当前 CI 或发布门禁。

### 端到端测试 | End-to-End Tests

```bash
./tests/scripts/run-live.sh
```

**默认行为**：

1. **Preflight 检查**：
   - `go test ./... -count=1`（单元测试）
   - `./tests/scripts/check-node-split-syntax.sh`（Node 拆分模块语法门禁）
   - `node --test --test-concurrency=1 tests/node/stream-tool-sieve.test.js tests/node/chat-stream.test.js tests/node/chat-history-utils.test.js tests/node/js_compat_test.js`
   - `npm run build --prefix webui`（WebUI 构建检查）

2. **隔离启动**：复制 `config.json` 到临时目录，启动独立服务进程

3. **场景测试**：
   - ✅ OpenAI 非流式 / 流式
   - ✅ Claude 非流式 / 流式
   - ✅ Admin API（登录 / 配置 / 账号管理）
   - ✅ Tool Calling
   - ✅ 并发压力测试
   - ✅ Search 模型

4. **结果收集**：继续执行所有用例（不中断），写入最终汇总

如果你只想跳过这些 preflight 检查，可以直接运行 `go run ./cmd/ds2api-tests --no-preflight`。

---

## CLI 参数 | CLI Flags

```bash
go run ./cmd/ds2api-tests \
  --config config.json \
  --admin-key admin \
  --out artifacts/testsuite \
  --port 0 \
  --timeout 120 \
  --retries 2 \
  --no-preflight=false \
  --keep 5
```

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `--config` | 配置文件路径 | `config.json` |
| `--admin-key` | Admin 密钥 | `DS2API_ADMIN_KEY` 环境变量，回退 `admin` |
| `--out` | 产物输出根目录 | `artifacts/testsuite` |
| `--port` | 测试服务端口（`0` = 自动分配空闲端口） | `0` |
| `--timeout` | 单个请求超时秒数 | `120` |
| `--retries` | 网络/5xx 请求重试次数 | `2` |
| `--no-preflight` | 跳过 preflight 检查 | `false` |
| `--keep` | 保留最近几次测试结果（`0` = 全部保留） | `5` |

---

## 自动清理 | Auto Cleanup

每次测试运行完成后，程序会自动扫描输出目录（`--out`），按时间排序保留最近 `--keep` 次运行的结果，超出部分自动删除。

- 默认保留 **5** 次
- 设置 `--keep 0` 可关闭自动清理
- 被删除的旧运行目录会打印日志提示

---

## 产物结构 | Artifact Layout

每次运行会创建一个以运行 ID 命名的目录：

```text
artifacts/testsuite/<run_id>/
├── summary.json          # 机器可读报告
├── summary.md            # 人类可读报告
├── server.log            # 测试期间服务端日志
├── preflight.log         # Preflight 命令输出
└── cases/
    └── <case_id>/
        ├── request.json      # 请求体
        ├── response.headers  # 响应头
        ├── response.body     # 响应体
        ├── stream.raw        # 原始 SSE 数据（流式用例）
        ├── assertions.json   # 断言结果
        └── meta.json         # 元信息（耗时、状态码等）
```

---

## Trace 关联 | Trace Binding

每个测试请求自动注入 trace 信息，便于快速定位问题：

| 位置 | 格式 |
| --- | --- |
| 请求头 | `X-Ds2-Test-Trace: <trace_id>` |
| 查询参数 | `__trace_id=<trace_id>` |

当用例失败时，`summary.md` 中会包含 trace ID。你可以快速搜索对应的服务端日志：

```bash
rg "<trace_id>" artifacts/testsuite/<run_id>/server.log
```

---

## 退出码 | Exit Code

| 退出码 | 含义 |
| --- | --- |
| `0` | 所有用例通过 ✅ |
| `1` | 有用例失败 ❌ |

可将测试集作为本地发布门禁使用（CI/CD 集成）。

---

## 安全提醒 | Sensitive Data Warning

⚠️ 测试集会存储**完整的原始请求/响应载荷**用于调试。

- **不要**将 artifacts 目录上传到公开仓库
- **不要**在 Issue tracker 中分享未脱敏的 artifact 文件
- 如需分享日志，请先手动清除敏感信息（token、密码等）

---

## 常见用法 | Common Usage

### 仅跑单元测试

```bash
go test ./...
```

### 运行特定模块的单元测试

```bash
# 运行 tool calls 相关测试（推荐用于调试 tool call 解析问题）
go test -v -run 'TestParseToolCalls|TestProcessToolSieve|TestRepair' ./internal/toolcall ./internal/toolstream

# 运行单个测试用例
go test -v -run TestParseToolCallsAllowsAllEmptyParameterPayload ./internal/toolcall

# 运行 format 相关测试
go test -v ./internal/format/...

# 运行 HTTP API 相关测试
go test -v ./internal/httpapi/openai/...
```

### 调试 Tool Call 问题 | Debugging Tool Call Issues

当遇到 DeepSeek 工具调用解析问题时，可以使用以下方法：

```bash
# 1. 运行 tool calls 相关的所有测试
go test -v -run 'TestParseToolCalls|TestProcessToolSieve|TestRepair' ./internal/toolcall ./internal/toolstream

# 2. 查看测试输出中的详细调试信息
go test -v -run TestProcessToolSieveReleasesMalformedExecutableXMLBlock ./internal/toolstream 2>&1

# 3. 检查具体测试用例的修复效果
# 重点测试位于 internal/toolcall/toolcalls_test.go 与 internal/toolstream/tool_sieve_xml_test.go，包含：
# - TestParseToolCallsAllowsAllEmptyParameterPayload: 空参数结构化保留
# - TestProcessToolSieveReleasesMalformedExecutableXMLBlock: malformed XML wrapper 释放为文本
# - TestRepairLooseJSONWithNestedObjects: 嵌套对象的方括号修复
```

### 运行 Node.js 测试

```bash
# 运行 Node 测试
node --test --test-concurrency=1 tests/node/stream-tool-sieve.test.js tests/node/chat-stream.test.js tests/node/chat-history-utils.test.js tests/node/js_compat_test.js

# 或使用脚本
./tests/scripts/run-unit-node.sh
```

### 跑端到端测试（跳过 preflight）

```bash
go run ./cmd/ds2api-tests --no-preflight
```

### 运行原始流仿真（独立工具）

```bash
./tests/scripts/run-raw-stream-sim.sh
```

说明：
- 该工具默认重放 `tests/raw_stream_samples/manifest.json` 声明的 canonical 样本，按上游 SSE 顺序做 1:1 仿真解析。
- 默认校验不出现 `FINISHED` 文本泄露，并要求存在结束信号。
- 默认**不**把 `raw accumulated_token_usage` 与本地解析 token 做强一致校验（当前实现以内容估算为准）；如需强校验可显式加 `--fail-on-token-mismatch`。
- 每次运行都会把本地派生结果写入 `artifacts/raw-stream-sim/<run-id>/<sample-id>/replay.output.txt`，并输出结构化报告。
- 如果你有历史基线目录，可以通过 `--baseline-root` 让工具直接做文本对比。
- 更完整的协议级行为结构说明见 [DeepSeekSSE行为结构说明-2026-04-05.md](./DeepSeekSSE行为结构说明-2026-04-05.md)。

### 对单个样本做回放比对

```bash
./tests/scripts/compare-raw-stream-sample.sh markdown-format-example-20260405-spacefix
```

说明：
- 该脚本会从 raw-only 样本目录读取 `upstream.stream.sse`。
- 回放结果会写入 `artifacts/raw-stream-sim/<run-id>/<sample-id>/`，便于直接查阅。
- 如果传入历史基线目录，脚本会自动对比当前回放输出和基线文本。

### 采集永久样本

本地启动服务后，可以直接打：

```bash
POST /admin/dev/raw-samples/capture
```

这个接口会把请求元信息和上游原始流写入 `tests/raw_stream_samples/<sample-id>/`，以后可以直接拿来做回放和字段分析。派生输出会在本地回放时再生成，不再落在样本目录里。

### 从内存抓包查询并保存样本

如果问题刚刚在本地复现过，也可以先查当前进程内存里的抓包，再选择性落盘：

```bash
GET /admin/dev/raw-samples/query?q=广州&limit=10
POST /admin/dev/raw-samples/save
{"chain_key":"session:xxxx","sample_id":"tmp-from-memory"}
```

说明：
- `query` 会按 `chat_session_id` 把 `completion + continue` 归并成一条链，适合定位接续思考问题。
- `save` 支持用 `query`、`chain_key` 或 `capture_id` 选中目标。
- 生成的样本目录仍然是 `tests/raw_stream_samples/<sample-id>/`，可以直接喂给回放脚本。

### 指定输出目录和超时

```bash
go run ./cmd/ds2api-tests \
  --out /tmp/ds2api-test \
  --timeout 60
```

### 在 CI 中使用

```bash
# 确保 config.json 存在且包含有效测试账号
./tests/scripts/run-live.sh
exit_code=$?
if [ $exit_code -ne 0 ]; then
  echo "Tests failed! Check artifacts for details."
  exit 1
fi
```
