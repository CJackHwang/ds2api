# Tool Parser v2 开发计划：加固 + 可观测

文档导航：[总览](../README.MD) / [文档索引](./README.md) / [开发路线总览](./dev-roadmap.md) / [PRD 评审](./prd-review-and-improvements.md) / [Tool Calling 语义](./toolcall-semantics.md) / [DeepSeek SSE 行为](./DeepSeekSSE行为结构说明-2026-04-05.md)

> 本文是 v2 阶段 Tool Parser / Stream 主题的执行计划。
> **核心原则**：现有 `internal/toolcall` + `internal/toolstream` + Node sieve 已足够覆盖大多数已知格式，本期目标是 **加固边界、引入 feature flag、shadow diff、可观测性**，而不是另起炉灶。
> 跨主题硬规则（PR Gate / 文档同步 / 分支命名）见 [`dev-roadmap.md`](./dev-roadmap.md)，本文不重复。

## 1. 现状盘点

### 1.1 已有实现

| 能力 | 代码位置 |
| --- | --- |
| DSML / canonical XML 解析、漂移容错、闭合修复 | `internal/toolcall/toolcalls_scan.go`、`toolcalls_parse_markup.go`、`toolcalls_xml.go`、`toolcalls_dsml.go` |
| schema 归一与参数结构化恢复 | `internal/toolcall/toolcalls_schema_normalize.go` |
| Tool sieve（流式防泄漏 + 半截 chunk 隔离 + CDATA 保护） | `internal/toolstream/tool_sieve_core.go`、`tool_sieve_state.go`、`tool_sieve_xml.go`、`tool_sieve_xml_scan.go`、`tool_sieve_jsonscan.go` |
| Tool prompt 注入 | `internal/toolcall/tool_prompt.go`、`internal/promptcompat/tool_prompt.go` |
| 输出语义归一（thinking / tool_call / usage） | `internal/assistantturn/turn.go`、`stream.go` |
| OpenAI / Claude / Gemini 输出格式化 | `internal/format/openai/`、`internal/format/claude/`、`internal/httpapi/gemini/` |
| Node sieve（Vercel 流式桥） | `internal/js/helpers/stream-tool-sieve/`、`api/chat-stream.js` |
| 单元 / 边界 / 回归测试 | `internal/toolcall/*_test.go`、`internal/toolstream/*_test.go`、`tests/scripts/run-unit-all.sh`、`tests/scripts/run-unit-node.sh` |
| 测试 fixtures | `tests/compat/fixtures/toolcalls/`、`tests/compat/fixtures/sse_chunks/`、`tests/raw_stream_samples/*` |
| 语义文档 | `docs/toolcall-semantics.md` |

### 1.2 差距矩阵

| 子能力 | 状态 | 说明 |
| --- | --- | --- |
| Unicode / 特殊 token Normalizer | done | 散落在 `toolcalls_scan.go` 与 sieve；可考虑抽象但非紧急 |
| Quarantine buffer | done | `tool_sieve_state.go` 已实现半截 token 隔离与释放 |
| Streaming state machine | done | `tool_sieve_xml.go` |
| ToolCall AST | partial | `internal/toolcall` 输出 `[]ToolCall`，未抽象成独立 AST 结构；非阻塞 |
| OpenAI emitter | done | `internal/format/openai/` |
| 误杀保护（普通 XML / Markdown / 代码块） | done | `fence_edge_test.go`、`fence_edge_sieve_test.go` 覆盖 |
| Go / Node parity 测试 | partial | 现有 Node 单测已覆盖核心场景；缺统一 fixtures→双端 driver |
| Fuzz / benchmark smoke | **missing** | 无系统化入口 |
| Feature flag（off / shadow / enforce） | **missing** | 需要 `internal/config/` 新增 |
| Shadow diff 收集 | **missing** | 需要在主链路同时跑新旧逻辑并 diff |
| 置信度模型 | **missing** | PRD v1 给的阈值未落地 |
| 解析事件级可观测性 | **missing** | 现有仅 `internal/devcapture` / `internal/rawsample` 抓包 |

> M0 的第一项交付物，就是把这张矩阵在仓库里以 issue / PR 形式钉死，并补上每一项的代码 / 测试链接。

## 2. 里程碑

### 2.1 M0：盘点与 fixtures 基线（约 1 周）

**目标**：让所有改动有可量化的回归基线。

任务：

- 把 §1.2 差距矩阵转成 issue checklist，并补每项的代码 / 测试链接。
- 收集真实失败样本到 fixtures：
  - 误判为 tool_call 的普通 XML / Markdown 文本（`tests/compat/fixtures/toolcalls/false_positive/`）。
  - 漏识别的 DSML / canonical 漂移样本（`tests/compat/fixtures/toolcalls/true_positive/`）。
  - 半截 chunk + 跨 chunk 闭合样本（`tests/compat/fixtures/sse_chunks/`）。
- 跑一次 `tests/scripts/run-unit-all.sh` + `run-unit-node.sh`，记录基线时间与覆盖。

DoD：

- 差距矩阵 issue 已开。
- 新增 fixtures 至少 5 组真阳 / 5 组假阳。
- 基线测试报告写入 PR description。

分支：`docs/toolparser-baseline`、`feat/m0-toolparser-fixtures`。

### 2.2 M1：边界回归与置信度准备（约 1–2 周）

**目标**：在不引入 flag 的前提下，先把现有 parser 的边界 bug 收敛到稳定状态。

任务：

- 依据 M0 fixtures 修复确认的真假阳问题（一个 fixture 一个最小 PR，便于 review）。
- 抽象 “解析候选 + 置信信号” 内部 API（不暴露给调用方），保留旧返回值不变；为 M2 shadow diff 做准备。
- Go / Node 解析路径新增 parity driver：以同一 fixture 同时驱动 `internal/toolstream` 与 `internal/js/helpers/stream-tool-sieve`，diff 输出。

DoD：

- 所有 M0 收集到的真阳样本通过；假阳样本不再被识别为 tool_call。
- parity driver 在 CI 中接入 `run-unit-all.sh`。
- `docs/toolcall-semantics.md` 同步更新（如有语义变化）。

分支命名：`fix/toolparser-<bug-slug>`、`feat/m1-toolparser-parity-driver`。

### 2.3 M2：feature flag + shadow diff + 可观测性（约 1–2 周）

**目标**：新能力可被 shadow 验证而不影响线上行为。

任务：

- 在 `internal/config/` 新增 `parser.v2` 三态开关（`off` / `shadow` / `enforce`），默认 `off`：
  - 加载入口参考 `internal/config/store.go` / `models.go` 模式；
  - 通过请求上下文向下传递，不允许在 handler 内 ad-hoc 读取。
- 在 Chat / Responses / Vercel 主链路接入 shadow 模式：
  - 旧 parser 仍然提供对外结果；
  - 新 parser（含置信度）并行运行，diff 写入观测通道。
- 置信度模型基于 M0 fixtures 的混淆矩阵反推阈值；不预先固定数值。
- 新增可观测性指标（命名先与 governance 计划的埋点盘点对齐）：
  - `parser_state`、`parser_suppressed_token_count`、`parser_tool_call_count`、`parser_shadow_diff_count`。
- WebUI Admin 设置页（`webui/src/features/settings/`）增加 flag 状态只读展示。

DoD：

- `parser.v2=off` 时主链路与 M1 完全一致（行为零差）。
- `parser.v2=shadow` 时输出旁路 diff，无对外影响。
- 文档同步：`docs/toolcall-semantics.md` 描述 shadow 行为；`docs/ARCHITECTURE.md` 标注 flag 入口。

分支命名：`feat/m2-toolparser-flag`、`feat/m2-toolparser-shadow-diff`、`feat/m2-toolparser-metrics`。

### 2.4 M3：联调与发布候选（约 1 周）

**目标**：把 shadow diff 收集到的差异修复到可 enforce 的水平，但**默认仍 off**。

任务：

- 跑一轮真实流量 / `tests/scripts/run-live.sh` 收集 shadow diff。
- 修复 diff 中确认为 “新 parser 错” 的样本；记录 diff 中 “旧 parser 错” 的样本作为下版本默认开启依据。
- Fuzz / benchmark smoke 接入：选若干长文本 + 工具调用混合场景，确保新链路无明显回归。
- 写发布候选 checklist：何时把 `parser.v2` 默认改为 `shadow`、何时改 `enforce`。

DoD：

- shadow diff 收敛到 “旧 parser 错” 占多数。
- 发布候选 checklist 入 `docs/dev-plan-toolparser.md` 附录。
- 所有 PR 通过 PR Gate；`run-live.sh` 报告归档到 `artifacts/testsuite/`（脱敏）。

## 3. 接口与边界约束

- Tool parser 的对外契约（`ParseToolCalls*` 函数签名 + assistant turn 中的 tool_call 字段）在 v2 期间**不允许破坏性变更**。
- Shadow diff 数据流仅在内部观测通道，不暴露给客户端响应。
- Node sieve 与 Go sieve 必须以同一组 fixtures 为唯一事实来源；任何一端独有的兼容补丁必须在 `docs/toolcall-semantics.md` 显式记录。

## 4. 风险与回滚

| 风险 | 应对 |
| --- | --- |
| shadow diff 噪声过大 | 只收集结构化字段（calls / sawToolCallSyntax），原文不入观测通道 |
| 新置信度阈值导致回归 | 阈值由 fixtures 驱动；上线前必须过 M0/M1 全量 fixtures |
| Vercel Node 与 Go 不同步 | parity driver 强制对齐；新增样例必须双端通过 |
| feature flag 残留 | M3 结束时审计 `parser.v2` 引用，文档列出后续清理计划 |

## 5. 验收

- 差距矩阵 §1.2 中所有 `missing` / `partial` 项变为 `done` 或显式延期。
- `parser.v2` 三态可配置；`off` 与 v1 行为一致；`shadow` 提供 diff；`enforce` 可手动启用做内部验证。
- `docs/toolcall-semantics.md` 与本计划交叉链接；新增语义在文档中可查。
- PR Gate 全绿，`run-live.sh` 报告归档。
