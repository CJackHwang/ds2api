# DS2API 测试指南

语言 / Language: [中文 + English](TESTING.md)

## 概述 | Overview

DS2API 提供两个层级的测试：

| 层级 | 命令 | 说明 |
| --- | --- | --- |
| 单元测试（Go） | `./tests/scripts/run-unit-go.sh` | 不需要真实账号 |
| 单元测试（Node） | `./tests/scripts/run-unit-node.sh` | 不需要真实账号 |
| 单元测试（全部） | `./tests/scripts/run-unit-all.sh` | 不需要真实账号 |
| 端到端测试 | `./tests/scripts/run-live.sh` | 使用真实账号执行全链路测试 |

端到端测试集会录制完整的请求/响应日志，用于故障排查。

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

# 发布阻断：阶段 6 手工烟测签字检查（默认读取 plans/stage6-manual-smoke.md）
./tests/scripts/check-stage6-manual-smoke.sh
```

### 端到端测试 | End-to-End Tests

```bash
./tests/scripts/run-live.sh
```

**默认行为**：

1. **Preflight 检查**：
   - `go test ./... -count=1`（单元测试）
   - `./tests/scripts/check-node-split-syntax.sh`（Node 拆分模块语法门禁）
   - `node --test`（如仓库存在 Node 单测文件时执行；当前默认以 Go 测试 + Node 语法门禁为主）
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
go test -v -run 'TestParseToolCalls|TestRepair' ./internal/util/

# 运行单个测试用例
go test -v -run TestParseToolCallsWithDeepSeekHallucination ./internal/util/

# 运行 format 相关测试
go test -v ./internal/format/...

# 运行 adapter 相关测试
go test -v ./internal/adapter/openai/...
```

### 调试 Tool Call 问题 | Debugging Tool Call Issues

当遇到 DeepSeek 工具调用解析问题时，可以使用以下方法：

```bash
# 1. 运行 tool calls 相关的所有测试
go test -v -run 'TestParseToolCalls|TestRepair' ./internal/util/

# 2. 查看测试输出中的详细调试信息
go test -v -run TestParseToolCallsWithDeepSeekHallucination ./internal/util/ 2>&1

# 3. 检查具体测试用例的修复效果
# 测试用例位于 internal/util/toolcalls_test.go，包含：
# - TestParseToolCallsWithDeepSeekHallucination: DeepSeek 典型幻觉输出
# - TestRepairLooseJSONWithNestedObjects: 嵌套对象的方括号修复
# - TestParseToolCallsWithMixedWindowsPaths: Windows 路径处理
# - TestParseToolCallsPreservesWindowsPathLiteralBackslashes: 路径字段防止 `\n/\t` 语义污染
# - TestParseToolCallsMixedContentPathAndNewline: 路径字段与正文换行语义并存
# - tests/node/stream-tool-sieve.test.js strict 场景：mixed prose + tool JSON 仍拦截 tool call
```

### 运行 Node.js 测试

```bash
# 运行 Node 测试
node --test tests/node/stream-tool-sieve.test.js

# 或使用脚本
./tests/scripts/run-unit-node.sh
```

### 跑端到端测试（跳过 preflight）

```bash
go run ./cmd/ds2api-tests --no-preflight
```

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
