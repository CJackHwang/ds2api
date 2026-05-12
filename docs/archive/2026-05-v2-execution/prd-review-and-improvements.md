# PRD 评审与改进建议（针对 `docs/ds_2_api_prd.md`）

> Status: archived review record. Future planning is governed by [`../../v2-prd.md`](../../v2-prd.md).

文档导航：[总览](../../../README.MD) / [文档索引](../../README.md) / [归档索引](./README.md) / [PRD v1](./ds_2_api_prd.md) / [开发路线总览](./dev-roadmap.md) / [Tool Parser v2 计划](./dev-plan-toolparser.md) / [Context Engine 计划](./dev-plan-context-engine.md) / [治理计划](./dev-plan-governance.md)

> 本文是对 `docs/ds_2_api_prd.md`（以下简称 “PRD v1”）的评审与改进建议，不直接改写原文。
> 落地以本目录下三份 `dev-plan-*.md` 与 `dev-roadmap.md` 为准；PRD v1 仅作为规划输入保留。

## 1. 评审结论摘要

PRD v1 的方向判断（“先打地基再扩展协议”）是合理的，但在落地维度上与当前代码现状有几处明显错位：

1. **Stage A 应该是“加固 + 可观测”，而不是“从 0 重建”**。
2. **Context Engine 的落点必须显式锚定到 `internal/promptcompat` → `internal/completionruntime` 之间**，否则会与 `AGENTS.md` 的 *Protocol Adapter Boundary* 规则冲突。
3. **PR 切分与节奏建议没有对接仓库现成的 PR 门禁**，需要在每个里程碑结束做一次完整门禁。
4. **多个被列为 P0 的“当前问题”需要先在代码里复核证据**，再决定是否纳入第一波改造（避免 over-engineering）。
5. **文档同步是仓库硬规则**，PRD v1 没有显式约束这一点。

下文逐条展开。

## 2. 与现有实现的差距对照

### 2.1 Tool Call / Stream Parser：已经不是空地

PRD v1 “Stage A” 把 tokenizer / quarantine buffer / streaming state machine / AST / emitter / parity 测试列为待建项，但仓库里已有大量对应实现：

| 能力 | 已有位置 | 状态 |
| --- | --- | --- |
| DSML / canonical XML 归一与漂移容错 | `internal/toolcall/toolcalls_scan.go`、`toolcalls_parse_markup.go` | done |
| Tool sieve（流式防泄漏 + 半截 chunk 隔离） | `internal/toolstream/tool_sieve_core.go`、`tool_sieve_state.go`、`tool_sieve_xml.go`、`tool_sieve_jsonscan.go` | done |
| CDATA 保护、fenced code block 防误杀 | `internal/toolstream/tool_sieve_xml.go`、`tool_sieve_xml_test.go`、`fence_edge_sieve_test.go` | done |
| Node 端 sieve 平价 | `internal/js/helpers/stream-tool-sieve/` + `tests/scripts/run-unit-node.sh` | done |
| Tool call AST → OpenAI tool_calls emitter | `internal/toolcall/toolcalls_format.go`、`internal/format/openai/` | done |
| 输出语义归一（thinking / tool_call / usage） | `internal/assistantturn/turn.go` | done |
| 解析 fixtures | `tests/compat/fixtures/toolcalls/`、`tests/raw_stream_samples/*` | done |
| feature flag（off / shadow / enforce） | 暂无统一抽象 | **missing** |
| 置信度模型 + shadow diff 上报 | 暂无 | **missing** |
| 解析事件级可观测性指标 | 暂无（仅有 `internal/devcapture` / `internal/rawsample` 抓包） | **missing** |
| Fuzz / benchmark smoke | 暂无系统化入口 | **partial** |

**结论**：Stage A 的真正交付应是 **加固现有实现 + 补 feature flag、shadow diff、可观测性、回归 fuzz**，而不是再造一个 parser 内核。

### 2.2 Context State Engine：基本是空地，但落点要谨慎

仓库里没有 `ContextSegment` / `ContextPlan` 概念，但 prompt 与 history 的归一化已经集中在 `internal/promptcompat`：

- `request_normalize.go` / `message_normalize.go` / `responses_input_normalize.go`：协议请求归一为标准 turn。
- `history_transcript.go`：历史拼接。
- `file_refs.go`：文件引用与 current input file 注入。
- `thinking_injection.go`：thinking / reasoning 摘要注入。
- `prompt_build.go` + `tool_prompt.go`：最终拼装。

PRD v1 没有提及这些已有模块。建议把 Context Engine 显式定位为：

> 在 `promptcompat` 之后、`completionruntime` 之前的一层独立编译器（`internal/contextengine/`）。
> 它消费 `promptcompat` 的标准 turn + history，输出 `ContextPlan`；`completionruntime` 仍然只看最终 prompt + payload。

这样符合 `AGENTS.md` 的 *Protocol Adapter Boundary*：协议形状先归一，业务逻辑集中在 normalized 层；Context Engine 永远不感知 OpenAI / Claude / Gemini 协议形状。

### 2.3 “当前主要问题”需要先复核证据

PRD v1 §1.2 列出的若干问题，应先在代码里复核再决定 P0/P1：

| 问题表象 | 复核入口 | 处理建议 |
| --- | --- | --- |
| 默认 Admin key | `internal/auth/admin.go`、`internal/config/codec.go` | 复核默认值与 fail-closed 行为后再排期 |
| query key 兼容 | `internal/auth/`、`internal/server/router.go` | 评估生产风险，可能比 parser 改造更紧急 |
| Vercel/Go 行为不一致 | `api/chat-stream.js` ↔ `internal/toolstream` | 现有 Node sieve 已存在，需要先跑 parity 看真实 diff，再决定改造规模 |
| 历史裁剪导致 tool_call/tool_result 断链 | `internal/promptcompat/history_transcript.go` | 先收集真实失败样本到 fixtures，再设计 invariant checker |
| 重复读文件 | `internal/promptcompat/file_refs.go` | 现有 file ref 是否已带 digest？需先看一遍 |

**建议**：M0 阶段产出一份《问题证据清单》（issue 链接 / fixture / 复现步骤），不在 PRD 阶段直接把这些列为已知缺陷。这与 user rule 的 “先定位根因、再下最小修复” 是一致的。

## 3. 节奏与 PR 切分建议

### 3.1 节奏

PRD v1 提出 “16×2h Phase / 4–5 个工作日 / 6–8 个正式 PR / 单工程师 + Subagent”。问题：

- 每个 Phase 2 小时不计 PR 门禁（`scripts/lint.sh` + `check-refactor-line-gate.sh` + `run-unit-all.sh` + `webui` build）耗时；门禁本身在该项目是分钟级。
- 单工程师并行多 worktree 容易 drift，与 `Change Scope` 规则冲突。
- “4–5 天稳定版” 对于改 prompt / tool / context 主链路是激进上界。

**改为里程碑（按周）**：M0 盘点 → M1 加固与回归 → M2 feature flag + shadow → M3 联调 / 发布候选。每个主题 plan 内部各自展开 milestone，详细见 `dev-plan-*.md`。

### 3.2 PR 切分

PRD v1 建议 “正式 PR 6–8 个”。这部分基本可以保留，但要补两条：

- 每个 PR 必须能独立通过 `AGENTS.md` 的 PR Gate 命令；
- 涉及 prompt / tool / history 的 PR 必须同时更新 `docs/prompt-compatibility.md` 与 `docs/toolcall-semantics.md`（现有文档同步硬规则）。

### 3.3 Subagent / worktree

PRD v1 §5 的 Subagent 与 worktree 安排有参考价值，但不属于权威开发规划，建议在 v2 中**降级为附录**。新规划只规定：分支命名、PR 门禁、合入策略；Agent 是否使用、用几个，由执行者按情况决定。

## 4. feature flag 与可观测性

### 4.1 feature flag 三态需要落到具体目录

PRD v1 提了 “off / shadow / enforce”，但没有指明配置入口。建议：

- 配置加载落到 `internal/config/`（参照 `models.go`、`store.go` 的现有模式），新增 `parser.v2`、`context.engine` 两个开关，默认 `off`。
- 运行时入口在 `internal/server/router.go` 装配阶段读取，沿请求上下文向下传递；不要散在 handler。
- WebUI Admin 设置页（`webui/src/features/settings/`）添加只读展示，不暴露切换按钮（避免误操作）。

### 4.2 置信度策略要可证伪

PRD v1 直接给了 0.85 / 0.4 阈值。建议改为：

1. M0 收集 “该被识别为 tool_call 的样本” 与 “容易误判为 tool_call 的普通文本” 两组 fixtures（可放 `tests/compat/fixtures/toolcalls/confidence/`）。
2. M1 在 shadow 模式下产出混淆矩阵；阈值据此反推。
3. 不在 PRD 阶段固定数值。

### 4.3 可观测性指标先调研再命名

PRD v1 §11.3 列出的指标（TTFT / parser_state / suppressed_token_count / context_plan_id 等）应先与 `internal/devcapture`、`internal/rawsample`、`internal/responsehistory` 的现有埋点对齐，避免重名或语义漂移。建议在 governance 计划里单独排一项 “埋点盘点”。

## 5. 文档同步硬约束

PRD v1 没有提到，但仓库规则要求：

- 任何改 prompt / tool / history / file ref / completion payload 行为的变更，必须同次 PR 更新 `docs/prompt-compatibility.md`。
- 任何改 tool call 解析语义的变更，必须同次 PR 更新 `docs/toolcall-semantics.md`。
- 模块结构调整必须同步 `docs/ARCHITECTURE.md`。

在 v2 三份 plan 里，每个里程碑的 “Definition of Done” 都包含文档同步检查项。

## 6. 边界约束

PRD v1 §3.2 “暂不纳入本期” 的内容（多 Provider 抽象、WebUI TS 重构、多租户、Context Debug UI、默认强制启用新 parser）建议保留作为长期目标，由 `dev-plan-governance.md` 持有。

## 7. 改进建议清单（提交给 v2 plan 的输入）

1. 把 PRD v1 §4.1 改造叙述从 “建 parser” 改为 “补全 feature flag / 置信度 / shadow diff / 可观测性”。
2. 把 PRD v1 §4.2 显式锚定到 `internal/contextengine/`（新包）+ `promptcompat ↔ completionruntime` 边界。
3. 把 PRD v1 §6 16-Phase 表替换为 milestone 表（见各主题 plan）。
4. 在每个 plan 的 “验收” 里强制列出 PR Gate 命令与文档同步项。
5. 在 `AGENTS.md` 增加 `Branch Discipline` 小节（参见 `dev-roadmap.md` §分支与集成策略）。
6. PRD v1 §1.2 的 “当前问题表” 改为 M0 “问题证据清单” 的输入，不直接当成已确认缺陷。
7. PRD v1 §5 Subagent / worktree 内容降级为附录性质的执行参考。
8. 置信度阈值改为 “由 fixtures + 混淆矩阵反推”，不预先固定数值。

## 8. 与原 PRD 的关系

- 不修改 PRD v1 主体；仅在其顶部加 `Status: v1 设想（planning input）` 状态行，指向本评审文档与 v2 三份 plan。
- v2 plan 与本评审在出现冲突时，以 v2 plan 为准。
