# Context Engine 开发计划：渐进引入

文档导航：[总览](../README.MD) / [文档索引](./README.md) / [开发路线总览](./dev-roadmap.md) / [PRD 评审](./prd-review-and-improvements.md) / [兼容主链路](./prompt-compatibility.md) / [架构说明](./ARCHITECTURE.md)

> 本文是 v2 阶段 Context Engine 主题的执行计划。
> **核心原则**：在 `internal/promptcompat`（已承担请求归一）与 `internal/completionruntime`（completion 启动）之间，新增独立的上下文编译层 `internal/contextengine/`，以 shadow → enforce 的方式渐进接入主链路。
> 跨主题硬规则（PR Gate / 文档同步 / 分支命名）见 [`dev-roadmap.md`](./dev-roadmap.md)。

## 1. 现状盘点

### 1.1 当前 prompt / context 主链路

```
HTTP surface（OpenAI / Claude / Gemini）
  → internal/promptcompat（请求归一、history 拼接、file refs、thinking 注入、tool prompt）
  → internal/prompt（最终 prompt 组装）
  → internal/completionruntime（启动 DeepSeek completion、空输出 retry）
  → internal/assistantturn（输出归一）
  → internal/format/*（协议渲染）
```

详见 `docs/prompt-compatibility.md`。

### 1.2 已有但分散的能力

| 能力 | 代码位置 | 状态 |
| --- | --- | --- |
| 请求归一为标准 turn | `internal/promptcompat/request_normalize.go`、`message_normalize.go`、`responses_input_normalize.go` | done |
| history 拼接 | `internal/promptcompat/history_transcript.go` | done |
| 文件引用 / current input file | `internal/promptcompat/file_refs.go` | done |
| thinking / reasoning 注入 | `internal/promptcompat/thinking_injection.go` | done |
| tool prompt 注入 | `internal/promptcompat/tool_prompt.go`、`internal/toolcall/tool_prompt.go` | done |
| 最终 prompt 组装 | `internal/promptcompat/prompt_build.go` | done |
| 上游响应归档 | `internal/responsehistory/`、`internal/chathistory/` | done |

### 1.3 缺失能力（v2 目标）

- 没有显式的 `ContextSegment` / `ContextPlan` 概念，prompt 是“按字符串拼出来”的。
- 没有 token budget / 优先级裁剪；长任务容易把当前请求挤掉。
- 没有 file digest 复用机制；同一文件可能在多轮被重复注入。
- 没有 reasoning summary 中间层；要么注入完整 thinking，要么完全不注入。
- 没有 tool_call / tool_result 配对不变量校验；history 裁剪可能断链。
- 没有可解释输出（“哪些被保留 / 摘要 / 裁剪”）。

## 2. 新模块设计：`internal/contextengine/`

### 2.1 落点

**严格落在** `promptcompat` 的标准 turn 输出之后、`completionruntime` 的 completion 启动之前：

```
promptcompat 标准 turn + history + file refs
  → internal/contextengine（编译为 ContextPlan）
  → internal/prompt（按 ContextPlan 渲染最终 prompt）
  → completionruntime
```

Context Engine **永远不感知** OpenAI / Claude / Gemini 协议形状（与 `AGENTS.md` *Protocol Adapter Boundary* 一致）。

### 2.2 数据结构（草案，最终以代码为准）

```go
type SegmentType string // system / user / assistant / tool_call / tool_result / file_snapshot / summary / reasoning_summary

type ContextSegment struct {
    ID        string
    Type      SegmentType
    Source    string // request / history / tool / file / cache
    Priority  int
    TokenCost int
    Digest    string
    Content   string
    Metadata  map[string]any
}

type ContextPlan struct {
    PlanID            string
    SegmentsIncluded  []ContextSegment
    SegmentsTrimmed   []TrimmedSegment
    TokenBudget       TokenBudgetReport
    ReusedFiles       []FileSnapshotRef
    ToolPairs         []ToolPairRef
    Warnings          []string
}
```

### 2.3 编译原则

- 当前用户请求最高优先级，不被历史挤掉。
- system / developer / 工具协议片段不可随意裁剪。
- tool_call 与 tool_result 必须配对（缺一则一并裁掉，不允许只剩一边）。
- 文件以 digest 标识；命中复用则用引用而非原文。
- thinking 默认转 reasoning summary；完整原文不直接注入。
- 所有保留 / 裁剪 / 摘要决策写入 `ContextPlan.Warnings` 或 `SegmentsTrimmed`，可解释。

## 3. 里程碑

### 3.1 M0：差距复核与接口草案（约 1 周）

任务：

- 在仓库里复核 PRD v1 提出的 “上下文相关问题” 是否真实存在；产出《问题证据清单》（带 fixture / 复现链接）：
  - 多轮对话历史拼接是否有 tool pair 断链？看 `history_transcript.go`。
  - 文件 ref 是否已有 digest？看 `file_refs.go`。
  - thinking 是否会原文注入？看 `thinking_injection.go`。
- 起草 `internal/contextengine/` 的接口草案（仅 `.md` 设计稿 + 占位 `doc.go`，不写实现）。
- 收集典型多轮 / 工具调用 / 长文件场景为 fixtures（`tests/compat/fixtures/context/`）。

DoD：

- 问题证据清单与接口草案进入 PR description 与 `dev-plan-context-engine.md` 附录。
- fixtures 至少 3 组：纯多轮、tool loop、长文件多轮。

分支：`docs/context-engine-design`、`feat/m0-context-fixtures`。

### 3.2 M1：核心数据结构 + 只读编译器（约 1–2 周）

任务：

- 新建 `internal/contextengine/` 包：
  - `segment.go`：`ContextSegment` / `ContextPlan` 等类型。
  - `compile.go`：消费 `promptcompat` 输出，产出 `ContextPlan`。
  - `digest.go`：文件与文本 digest 计算。
  - `*_test.go`：用 M0 fixtures 驱动。
- **不接入主链路**；仅作为可单元测试的纯函数层。
- 同步 `docs/prompt-compatibility.md`：增加 “Context Engine 编译阶段（off）” 一节，说明此时 plan 仅在测试中使用。

DoD：

- `go test ./internal/contextengine/...` 全绿。
- 所有 M0 fixtures 都能产出 ContextPlan 且 token budget 不报告错误。
- `docs/prompt-compatibility.md` 同步。

分支：`feat/m1-context-engine-core`。

### 3.3 M2：feature flag + shadow 注入（约 1–2 周）

任务：

- 在 `internal/config/` 新增 `context.engine` 三态（`off` / `shadow` / `enforce`），默认 `off`。
- 在 `promptcompat` 的最终 prompt 组装阶段挂 hook：
  - `off`：不调用 contextengine。
  - `shadow`：调用 contextengine 产出 ContextPlan，**仅写入观测通道**（不影响最终 prompt）。
  - `enforce`：用 ContextPlan 渲染 prompt（M3 才允许验证开启）。
- 设计观测通道：复用 `internal/devcapture` 或新增 `internal/contextengine/observe.go`，写入 plan 摘要（不含原文，避免敏感泄漏）。
- Admin Debug 接口（只读）：`/admin/context-plan/{request_id}` 返回最近一次 shadow plan 的摘要，供调试（默认关闭，仅 admin 鉴权后可见）。
  - **当前状态**：由分支 `feat/m2-context-debug-api` 落地（plan 摘要内存环形缓冲 + `GET /admin/context-plan` / `GET /admin/context-plan/{request_id}`）。

DoD：

- `context.engine=off` 时主链路零差。
- `shadow` 模式下能稳定产出 plan；不污染最终 prompt。
- `docs/prompt-compatibility.md` 与 `docs/ARCHITECTURE.md` 同步更新。

分支：`feat/m2-context-flag`、`feat/m2-context-shadow`、`feat/m2-context-debug-api`。

### 3.4 M3：tool pair 不变量 + token budget + reasoning summary（约 1–2 周）

任务：

- 在 `compile.go` 内实现：
  - tool_call / tool_result 配对校验与裁剪保护；
  - 优先级 + token budget 裁剪策略；
  - reasoning summary 生成（基于 thinking_injection 的现有逻辑，转成摘要 segment）。
- 用 M0 fixtures + 新增对抗 fixtures（人为构造历史超长、tool 断链、thinking 巨大）做回归。
- 仍仅在 `shadow` 模式下验证；`enforce` 由后续发布候选开启。

DoD：

- 所有不变量 fixtures 通过；ContextPlan.Warnings 能正确指出问题来源。
- shadow 模式下 plan 与旧 prompt diff 在可解释范围内。
- 文档同步：`docs/prompt-compatibility.md` 增 token budget / tool pair 章节。

分支：`feat/m3-context-tool-pair`、`feat/m3-context-budget`、`feat/m3-context-reasoning-summary`。

### 3.5 M4：联调与发布候选（约 1 周）

任务：

- Chat / Responses / Vercel 三条主路径在 `shadow` 模式下跑真实流量；收集 plan diff。
- 与 Tool Parser M3 联调：确保 tool_call 在 ContextPlan 与 parser 双侧语义一致。
- 写发布候选 checklist：何时把 `context.engine` 默认改为 `shadow`、何时 `enforce`。
- 端到端测试：`./tests/scripts/run-live.sh`（脱敏归档到 `artifacts/testsuite/`）。

DoD：

- 发布候选 checklist 入本文档附录。
- PR Gate 全绿。
- `docs/prompt-compatibility.md` 是 ContextPlan 行为的权威描述。

## 4. 接口与边界约束

- `internal/contextengine/` **不依赖** `internal/httpapi/*`，也不依赖 OpenAI / Claude / Gemini surface 类型。
- 输入只能是 `internal/promptcompat` 已归一的标准 turn / history / file refs。
- 输出 `ContextPlan` 是值类型；任何调用方拿到 plan 后都可以独立渲染。
- 在 `enforce` 模式之前，`promptcompat` 的输出仍是最终 prompt 的事实来源。

## 5. 风险与回滚

| 风险 | 应对 |
| --- | --- |
| 编译耗时拖慢主链路 | shadow 模式必须可降级到 `off`；监控编译耗时百分位 |
| ContextPlan 与现有 prompt 行为漂移 | 全部依靠 fixtures + shadow diff 收敛，不主观调阈值 |
| 文件 digest 计算撞库 / 误复用 | digest 算法采用足够长的哈希；M3 加专项 fixtures |
| reasoning summary 丢信息 | 默认保留 “summary + 末尾 N 字符原文”，由 fixtures 校验 |
| 与 history 持久化冲突 | `internal/responsehistory` / `chathistory` 仍存原文，不被 ContextPlan 影响 |

## 6. 验收

- `internal/contextengine/` 包存在并通过单元测试。
- `context.engine` 三态 flag 落到 `internal/config/`，默认 `off`。
- shadow 模式下能稳定产出 plan，diff 可观测。
- tool pair 不变量 / token budget / reasoning summary 三类 fixtures 全部通过。
- `docs/prompt-compatibility.md` 描述 ContextPlan 行为；`docs/ARCHITECTURE.md` 反映新模块位置。

---

## 附录 A — M0-B 问题证据清单

> 复核时间：M0 阶段。格式：现象 / 代码入口 / 结论 / 建议排期。

### A1. 文件引用：无 digest 机制

| 字段 | 详情 |
|------|------|
| **现象** | 同一文件多轮被引用时，每次均独立传递 `file_id`，无内容去重逻辑 |
| **代码入口** | `internal/promptcompat/file_refs.go:CollectOpenAIRefFileIDs` — 只收集字符串 ID，不计算内容摘要 |
| **结论** | 无 SHA-256 digest，Context Engine 无法判断两轮中同一文件是否内容相同，复用优化无从实现 |
| **建议排期** | M1-B `internal/contextengine/digest.go` 实现 `SHA256FileDigest`；M2 接入文件 segment 复用判断 |

### A2. 历史记录：tool_call / tool_result 孤立风险

| 字段 | 详情 |
|------|------|
| **现象** | 多轮历史被裁截到 token 限制时，可能保留 `assistant[tool_calls]` 却丢弃对应的 `role=tool` 结果，形成孤立 tool_call |
| **代码入口** | `internal/promptcompat/history_transcript.go:buildOpenAIHistoryTranscript` — 逐条遍历，**无 tool pair 完整性校验** |
| **关联** | `internal/promptcompat/message_normalize.go:NormalizeOpenAIMessagesForPrompt` 同样无配对验证 |
| **验证场景** | `tests/compat/fixtures/context/orphan_tool_call.json` — assistant 有 `tool_calls` 字段但无后续 `role=tool` 消息 |
| **结论** | 历史截断不保证 tool 对完整，会向模型发送不一致的 prompt，可能触发模型困惑或重复调用工具 |
| **建议排期** | M1-B `internal/contextengine/compile.go` 中加入 `ValidateToolPairInvariant(segments)`；检测孤立时将末尾不完整 tool_call segment 标记为 `TrimmedSegment` 而非丢弃 |

### A3. Thinking 注入：无历史 reasoning 压缩

| 字段 | 详情 |
|------|------|
| **现象** | 历史中每轮 assistant 的 `reasoning_content` 原文被逐字注入 prompt（通过 `formatPromptLabeledBlock`），多轮累积后 reasoning 块可消耗大量 token |
| **代码入口** | `internal/promptcompat/message_normalize.go:buildAssistantContentForPrompt` — `reasoning` 块直接拼接，无大小限制 |
| **关联** | `internal/promptcompat/thinking_injection.go:AppendThinkingInjectionToLatestUser` — 仅注入新的 thinking 提示文字，不压缩历史 reasoning |
| **结论** | 长对话时历史 reasoning 块线性增长；Context Engine M3 需在 `compile.go` 加入 reasoning summary（保留 "摘要 + 末 N 字符原文"）策略 |
| **建议排期** | M1-B `compile.go` 保持 off（不修改现有行为）；M3 实现 `SummarizeReasoningSegment` |

### A4. context fixtures 位置

已创建基线 fixtures（供 M1-B `contextengine` 包的单元测试使用）：

```
tests/compat/fixtures/context/
  plain_multiturn.json          — 纯多轮，无 tool call（428k 长历史，截取前 8k）
  tool_loop_read.json           — assistant tool_call + 配对 tool result
  orphan_tool_call.json         — assistant tool_call 后无 tool result（验证孤立检测）
  long_history_token_budget.json — 525k 历史截取前 16k，token_budget_hint=4096
```

---

## 附录 B — 发布候选 Checklist（M3 Stage 8）

> 状态：`context.engine` 默认仍为 `off`。  
> 本 checklist 定义"何时可以把默认改为 `shadow`"和"何时可以改为 `enforce`"。  
> 每项需有 **代码/日志/fixture 证据链接**，不接受主观判断。

### B1. `off` → `shadow`（在 staging 环境开启 shadow 收集）

| 条件 | 验证方式 | 当前状态 |
|------|---------|---------|
| 所有 M3 单元测试通过（tool pair / budget / reasoning） | `./tests/scripts/run-unit-all.sh` 全绿 | ✅ |
| `Compile()` 在 shadow 模式下不影响最终 prompt（promptcompat 仍是事实来源） | `TestBuildOpenAIPromptShadowMode` + code review | ✅ |
| Shadow plan 日志字段完整（plan_id / segments / trimmed / warnings） | `contextengine.MaybeShadow` 结构化日志 | ✅ |
| Fuzz seed corpus 无 panic（`FuzzParseToolCalls` seed 阶段） | `go test -run FuzzParse` | ✅ |
| Benchmark 基线已记录（无性能悬崖） | `BenchmarkParseToolCallsDSML ≈ 230 µs` | ✅ |
| E2E 跨主题集成测试通过（Chat/Responses/Gemini） | `TestBuildOpenAIPromptShadowMode` 等 4 个 E2E 测试 | ✅ |
| Stage 6 smoke checklist 填写并通过脚本检查 | `./tests/scripts/check-stage6-manual-smoke.sh` | ✅ (automated-CI) |
| **实际流量 shadow 收集 ≥ 24h，无 5xx 新增** | staging 日志审查 | ⏳ 待 staging 部署 |

**晋级操作**：设置 `DS2API_CONTEXT_ENGINE=shadow`（或 config `context_engine.mode: shadow`）部署 staging。

---

### B2. `shadow` → `enforce`（在生产环境启用 ContextPlan 主链路）

在满足 B1 所有条件并完成以下额外检查后方可执行。

| 条件 | 验证方式 | 当前状态 |
|------|---------|---------|
| Shadow 流量 diff 率 < 1% 持续 72h | `[context_engine_shadow] has_diff=true` 日志统计 | ⏳ 待数据 |
| Token budget 触发时 prompt 语义等价（人工 review 10 个截断样本） | 抽样审阅 `artifacts/testsuite/` 日志 | ⏳ 待数据 |
| Reasoning summary 对话质量无回退（A/B 对照 ≥ 50 次调用） | 生产 / staging 对比 | ⏳ 待数据 |
| Orphan tool_call 警告量与预期一致（无异常峰值） | `[context_engine_shadow] warnings` 日志 | ⏳ 待数据 |
| `./tests/scripts/run-live.sh` 端到端通过 | 实时请求脚本 | ⏳ 待 live 环境 |
| DEPLOY.md 已更新（`DS2API_CONTEXT_ENGINE` 说明） | 文档 review | ✅ (见下 §B3) |
| 回滚方案已确认（设 `off` 恢复，无 schema 迁移） | 操作手册 | ✅ 直接设 env 即可 |

**晋级操作**：设置 `DS2API_CONTEXT_ENGINE=enforce`（或 config）部署生产；发布 CHANGELOG。

---

### B3. DEPLOY.md 新增环境变量说明

以下字段需在 `docs/DEPLOY.md` 的环境变量章节补充：

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DS2API_CONTEXT_ENGINE` | `off` | Context Engine 功能开关：`off`（默认，不编译 plan）/ `shadow`（并行编译，日志可观测）/ `enforce`（ContextPlan 控制最终 prompt） |
| `DS2API_PARSER_V2` | `off` | Tool Parser v2 开关（同三态）；`shadow` 模式记录 diff 日志 |
