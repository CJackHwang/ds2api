# ds2api 双核心改造 PRD

> Status: archived planning input. Future planning is governed by [`../../v2-prd.md`](../../v2-prd.md).
>
> **Status: v1 设想（planning input, 2026-05）。**
> 本文档保留作为 v2 阶段的规划输入，**不再作为执行口径**。落地以下列 v2 文档为准：
>
> - 评审与差距分析：[`prd-review-and-improvements.md`](./prd-review-and-improvements.md)
> - 路线总览（含分支与集成策略）：[`dev-roadmap.md`](./dev-roadmap.md)
> - 主题计划：[`dev-plan-toolparser.md`](./dev-plan-toolparser.md)、[`dev-plan-context-engine.md`](./dev-plan-context-engine.md)、[`dev-plan-governance.md`](./dev-plan-governance.md)
>
> 当本文与 v2 plan 出现冲突时，以 v2 plan 为准。

## 0. 文档目的

本文档回顾我们围绕 `CJackHwang/ds2api` 的全部讨论，形成一份面向后续开发的产品与工程规划。重点聚焦两个高收益改造方向：

1. **Tool Call / 流式解析改造**：解决特殊 token 泄漏、工具调用解析不稳定、Go/Node/Vercel 行为不一致等问题。
2. **Context State Engine / 上下文状态引擎改造**：解决长任务上下文孤立、重复读文件、历史提示词干扰、tool_call/tool_result 断链等问题。

本文既是 PRD，也是工程执行规划，适合交给 Claude Code / Codex / Subagent 作为阶段任务拆解依据。

---

## 1. 项目分析简要总结

### 1.1 项目定位

`ds2api` 当前已经不是简单的 API 转换器，而是一个多协议 LLM 网关雏形：

- 对外兼容 OpenAI、Claude、Gemini、Ollama 等协议。
- 对内通过 DeepSeek Web / 上游能力完成实际推理。
- 包含 Go 后端、Vercel Node 流式桥、React 管理后台、账号池、PoW、文件处理、聊天历史、工具调用解析等模块。

因此，它的核心价值在于：**让不同客户端以熟悉的 API 协议调用 DeepSeek 能力**。它的核心风险也在于：**协议适配、流式输出、工具调用、上下文状态、账号调度全部耦合在一起，复杂度已经显著上升**。

### 1.2 当前主要问题

| 问题方向 | 具体表现 | 影响 |
|---|---|---|
| Tool Call / 流式解析 | DeepSeek 特殊 token 泄漏、tool call 被当成正文、半截 chunk 误发、乱码 | 影响正确性，Agent 客户端无法稳定调用工具 |
| Go / Node / Vercel 行为不一致 | 本地 Go 路径和 Vercel Node 流式桥清理/解析逻辑不完全一致 | 部署环境不同，问题复现困难 |
| 上下文处理薄弱 | 多轮对话像重新开始，历史 prompt 干扰模型，重复读文件 | 长任务体验弱，接近官方 API 的程度不足 |
| tool_call/tool_result 断链 | 历史裁剪后工具结果和工具调用不能配对 | Agent loop 易腐化，任务续接能力下降 |
| 安全与运维治理不足 | 默认 Admin key、query key 兼容、日志脱敏、CORS 管控等仍需加强 | 生产部署风险较高 |
| WebUI 与文档维护压力 | 管理后台与文档规模膨胀，工程边界不够清晰 | 后续维护成本上升 |

### 1.3 我们的总体判断

项目下一阶段不应继续以“发现一个问题补一个正则 / 补一个 prompt”的方式推进，而应升级为：

```text
字符串补丁式适配
  -> 事件驱动协议内核
  -> 可测试上下文编译器
  -> 多 Provider LLM Gateway
  -> 可观测、可灰度、可回滚的产品级系统
```

短期最关键的不是继续扩展更多 API，而是先把两个核心地基打稳：

1. **输出正确性地基**：Tool Call / Stream Parser。
2. **长任务连续性地基**：Context State Engine。

---

## 2. 产品目标

### 2.1 总目标

将 ds2api 从“多协议转发 + prompt 拼接 + 正则清理”升级为“可验证、可观测、可扩展的 LLM Gateway”。

### 2.2 用户侧目标

| 用户角色 | 目标 |
|---|---|
| 普通 API 使用者 | 流式输出干净，错误更少，OpenAI/Claude/Gemini 兼容体验更稳定 |
| Claude Code / Codex / OpenCode 等 Agent 客户端用户 | tool_calls 能被稳定识别，长任务不易断链，少重复读文件 |
| 部署者 | 有灰度开关、回滚方案、日志与故障排查手段 |
| 维护者 | 新增格式变体时有 fixtures、fuzz、benchmark 和协议测试，不再到处补正则 |

### 2.3 成功标准

| 目标 | 验收标准 |
|---|---|
| 特殊 token 不泄漏 | 常见 DeepSeek 控制 token、think tag、tool wrapper 不进入 visible output |
| tool call 结构化 | XML / DSML / JSON tool call 能转成 OpenAI tool_calls 或对应协议事件 |
| 半截流式 chunk 安全 | 对疑似 token / tool tag 先隔离，确认后释放或吞掉 |
| Go/Node 一致 | 关键 fixtures 在 Go 主路径和 Vercel Node 路径输出一致 |
| 长任务连续性提升 | 历史、文件快照、工具结果、摘要能被编译成 ContextPlan |
| 上下文可解释 | 能输出 ContextPlan，说明哪些内容被保留、摘要、裁剪、复用 |
| 可灰度可回滚 | parser v2 与 context engine 默认支持 shadow / enforce / off 模式 |

---

## 3. 改造范围

### 3.1 本期范围

本期重点做两个 MVP 级但可长期演进的核心模块：

#### A. Tool Call / Stream Parser

目标：把特殊 token、半截 chunk、工具调用 wrapper、JSON tool call、XML/DSML tool call 统一解析为结构化事件。

核心能力：

- Unicode / 特殊 token Normalizer。
- Tokenizer。
- Streaming quarantine buffer。
- Streaming state machine。
- ToolCall AST。
- OpenAI tool_calls emitter。
- Go / Node parity tests。
- fuzz seed / benchmark smoke。
- feature flag 与 shadow mode。

#### B. Context State Engine

目标：把当前 prompt 拼接升级成上下文编译，管理历史、文件快照、工具结果、思考摘要和 token budget。

核心能力：

- ContextSegment。
- ContextPlan。
- messages -> segments。
- history -> segments。
- file snapshot / current input file segment。
- reasoning summary / decision log。
- token budget / priority trimming。
- tool_call/tool_result 配对保护。
- Chat / Responses shadow 接入。

### 3.2 暂不纳入本期

| 暂不做 | 原因 |
|---|---|
| 完整多 Provider 抽象 | 需要更多架构调整，可放下一阶段 |
| WebUI 全面 TypeScript 重构 | 重要但不是当前最高风险 |
| 多租户 RBAC / 计费 / 审计 | 属于产品化阶段 |
| 完整 Context Debug UI | 本期先输出 ContextPlan 日志/接口，后续再做页面 |
| 默认强制启用新 parser / context engine | 本期先 shadow，再灰度，不直接替换主路径 |

---

## 4. 核心方案设计

## 4.1 Tool Call / Stream Parser 方案

### 4.1.1 目标架构

```text
Raw Stream Chunk
  -> Normalizer
  -> Tokenizer
  -> Quarantine Buffer
  -> Streaming Parser
  -> ToolCall AST / ParserEvent
  -> Protocol Emitter
  -> OpenAI / Claude / Gemini Response
```

### 4.1.2 关键设计

#### Normalizer

处理：

- 全角竖线 `｜` 与半角 `|`。
- `▁` 与 `_`。
- 大小写规整。
- 常见 DeepSeek marker。
- 零宽字符、BOM、异常空白。

#### Quarantine Buffer

用于处理半截流式内容，例如：

```text
chunk1: <｜Ass
chunk2: istant▁END▁OF
chunk3: ▁TOOL_CALLS｜>
```

策略：

- 发现疑似特殊 token 前缀时暂存。
- 等待后续 chunk。
- 如果确认是控制 token，吞掉。
- 如果不是控制 token，释放为普通文本。

#### Streaming Parser

建议状态：

```text
OutsideText
MaybeSpecialToken
MaybeToolCall
InToolCalls
InInvoke
InParameter
InCDATA
InJSONToolCall
InMarkdownFence
Recovery
```

#### ToolCall AST

```go
Type ToolCallAST struct {
    ID         string
    Name       string
    Arguments  map[string]any
    Raw        string
    Confidence float64
    Source     string
}
```

#### 置信度判断

不是所有 XML / JSON 都是 tool call，因此需要置信度：

| 信号 | 权重 |
|---|---|
| 出现完整 tool_calls wrapper | 高 |
| 工具名在 declared tools 内 | 高 |
| 参数能解析 | 高 |
| 位于模型工具调用区域 | 高 |
| 位于 Markdown 代码块 | 低 |
| 明显是用户解释文本 | 低 |

建议策略：

```text
confidence >= 0.85: 作为 tool_call
0.4 <= confidence < 0.85: 继续等待更多 chunk
confidence < 0.4: 作为普通文本释放
```

---

## 4.2 Context State Engine 方案

### 4.2.1 目标架构

```text
Raw Request Messages
  -> Segment Extractor
  -> Session State Loader
  -> File Snapshot Loader
  -> Reasoning Summary Builder
  -> Token Budget Planner
  -> Tool Pair Invariant Checker
  -> ContextPlan
  -> Final Prompt / Provider Payload
```

### 4.2.2 核心数据结构

```go
Type ContextSegment struct {
    ID        string
    Type      string // system, user, assistant, tool_call, tool_result, file_snapshot, summary, reasoning_summary
    Source    string // request, history, tool, file, cache
    Priority  int
    TokenCost int
    Digest    string
    Content   string
    Metadata  map[string]any
}
```

```go
Type ContextPlan struct {
    SegmentsIncluded []ContextSegment
    SegmentsTrimmed  []TrimmedSegment
    TokenBudget      TokenBudgetReport
    ReusedFiles      []FileSnapshotRef
    ToolPairs        []ToolPairRef
    Warnings         []string
}
```

### 4.2.3 编译原则

| 原则 | 说明 |
|---|---|
| 当前用户请求最高优先级 | 不能被历史噪声挤掉 |
| system / developer / 工具协议不可随意裁剪 | 保持行为约束 |
| tool_call 和 tool_result 必须配对 | 防止 Agent loop 断链 |
| 文件上下文用 digest 识别 | 避免重复读文件和重复上传 |
| thinking 不直接原文注入 | 默认注入 reasoning summary / decision log |
| 裁剪要可解释 | ContextPlan 记录保留、裁剪、摘要原因 |

---

## 5. 开发组织方式

### 5.1 最新开发假设

| 项目 | 当前设定 |
|---|---:|
| 工程师人数 | 1 位主控工程师 |
| AI 工具 | Claude Code / Codex / Subagent |
| 1 个 Phase | 约 2 小时 |
| 每个 Phase | 包含 2 到 4 个 Subagent 小 PR |
| 每天赶工 | 5 到 6 个 Phase |
| 推荐正式 PR | 6 到 8 个 |
| 推荐周期 | 核心开发 3 天，稳定化 1 到 2 天，总计 4 到 5 个工作日；稳妥按 1 周规划 |

### 5.2 Subagent 使用原则

| 规则 | 说明 |
|---|---|
| 工程师负责架构与主链路 | Subagent 不直接决定关键接口 |
| Subagent 负责小任务 | fixtures、测试、局部实现、文档、review |
| 每个 Subagent 限定目录 | 避免越界修改主链路 |
| 主链路串行接入 | Chat / Responses / stream runtime 不允许多个 Agent 同时乱改 |
| 每个 Phase 结束必须集成 | 不让 worktree 长时间漂移 |
| 新功能默认 feature flag | off / shadow / enforce 三态 |

### 5.3 Worktree 建议

| worktree | 用途 |
|---|---|
| `ds2api-integration` | 总集成分支 |
| `ds2api-parser` | Tool Parser 主开发 |
| `ds2api-parser-tests` | parser fixtures / fuzz / parity |
| `ds2api-context` | Context Engine 主开发 |
| `ds2api-context-tests` | context fixtures / budget tests |
| `ds2api-release` | docs / release / final review |

---

## 6. 16 Phase 执行规划

### 6.1 总览

| 阶段 | Phase | 方向 | 目标 | 小 PR 数 | 时间 |
|---|---:|---|---|---:|---:|
| Stage 0 | 1-2 | 基线准备 | fixtures、接口、feature flag、worktree | 5 | 4h |
| Stage A | 3-10 | Tool Call / Stream | parser、AST、emitter、主链路 shadow、Go/Node 对齐 | 24 | 16h |
| Stage B | 11-15 | Context Engine | segments、history、file snapshot、budget、Chat/Responses shadow | 15 | 10h |
| Stage C | 16 | 联调发布 | E2E、文档、灰度、回滚 | 4 | 2h |
| 合计 | 16 |  | 推荐稳定版 | 约 48 | 约 32h |

### 6.2 详细 Phase 表

| Phase | 2 小时目标 | Subagent 小 PR 建议 | 小 PR 数 | 优先级 |
|---:|---|---|---:|---|
| 1 | 建立基线、收集问题样本 | toolcall fixtures、context fixtures、回归命令 | 3 | P0 |
| 2 | 定义接口与 feature flag | parser flag、context flag、worktree/CI skeleton | 2 | P0 |
| 3 | Tool Call fixtures 强化 | 特殊 token、半截 chunk、XML/JSON toolcall | 3 | P0 |
| 4 | Normalizer / Tokenizer | unicode normalize、special marker、tokenizer tests | 3 | P0 |
| 5 | Quarantine buffer | 半截 token 隔离、释放策略、边界测试 | 2 | P0 |
| 6 | Streaming parser MVP | 状态机、recovery、基础事件输出、单测 | 4 | P0 |
| 7 | ToolCall AST + OpenAI emitter | AST、OpenAI tool_calls、non-stream emitter | 3 | P0 |
| 8 | 误杀保护与性能 | 普通 XML/Markdown/代码块、fuzz seed、benchmark smoke | 3 | P0 |
| 9 | Chat stream / non-stream shadow 接入 | stream shadow、non-stream shadow、feature flag | 3 | P0 |
| 10 | Go / Node / Vercel 对齐 | JS parity、fixtures 对齐、回滚开关 | 3 | P0 |
| 11 | Context 核心模型 | ContextSegment、ContextPlan、messages -> segments | 3 | P1 |
| 12 | History / file snapshot | history -> segments、current input file、digest/TTL | 3 | P1 |
| 13 | Reasoning summary | decision log、summary segment、配置项 | 2 | P1 |
| 14 | Token budget / 裁剪策略 | priority trim、tool_call/tool_result 配对保护、测试 | 4 | P1 |
| 15 | Context 接入 Chat / Responses shadow | Chat shadow、Responses shadow、ContextPlan debug | 3 | P1 |
| 16 | E2E、文档、灰度发布 | Claude Code/OpenCode E2E、docs、rollback、release checklist | 4 | P2 |

---

## 7. 正式 PR 切分

虽然每个 Phase 内部可以有 2 到 4 个 Subagent 小 PR，但正式暴露给主线的 PR 建议控制在 6 到 8 个。

| 正式 PR | 覆盖 Phase | 内容 |
|---|---|---|
| PR 1 | Phase 1-2 | fixtures、feature flag、开发基线 |
| PR 2 | Phase 3-5 | tokenizer、normalizer、quarantine buffer |
| PR 3 | Phase 6-8 | streaming parser、AST、emitter、fuzz |
| PR 4 | Phase 9-10 | Chat shadow、non-stream、Go/Node parity |
| PR 5 | Phase 11-13 | ContextSegment、ContextPlan、history/file/reasoning |
| PR 6 | Phase 14-15 | token budget、tool pair protection、Chat/Responses shadow |
| PR 7 | Phase 16 | E2E、docs、rollback、release checklist |
| PR 8 可选 | 稳定化 | CI 修复、边界补测、release cleanup |

---

## 8. 每日安排建议

### Day 1：Tool Parser 地基

| 时间 | Phase | 内容 |
|---|---:|---|
| 上午 | 1 | 基线 fixtures |
| 上午 | 2 | feature flag / worktree |
| 下午 | 3 | Tool fixtures |
| 下午 | 4 | Normalizer / Tokenizer |
| 晚上 | 5 | Quarantine buffer |
| 晚上可选 | 6 | Streaming parser MVP |

目标：问题可复现，parser v2 有入口，半截 token 具备隔离能力。

### Day 2：Tool Parser 完成 + Context Engine 启动

| 时间 | Phase | 内容 |
|---|---:|---|
| 上午 | 6/7 | Streaming parser / AST |
| 上午 | 8 | 误杀保护 / fuzz / benchmark |
| 下午 | 9 | Chat stream / non-stream shadow |
| 下午 | 10 | Go / Node / Vercel parity |
| 晚上 | 11 | Context 核心模型 |
| 晚上可选 | 12 | History / file snapshot |

目标：Tool Parser 可 shadow 接入，Go/Node 关键 fixtures 对齐，Context Engine 模型初版完成。

### Day 3：Context Engine 完成 + 主链路联调

| 时间 | Phase | 内容 |
|---|---:|---|
| 上午 | 12/13 | file snapshot / reasoning summary |
| 上午 | 14 | token budget / tool pair protection |
| 下午 | 15 | Chat / Responses shadow 接入 |
| 下午 | 16 | E2E / 文档 / 灰度 |
| 晚上 | Buffer | CI / 回归修复 |

目标：ContextPlan 可输出，Chat / Responses 基础回归通过，Claude Code / OpenCode 初步 E2E 完成。

### Day 4-5：稳定化

| 天数 | 内容 |
|---|---|
| Day 4 | 修 E2E、修 parser 边界、修 context 裁剪问题、补文档 |
| Day 5 | 灰度策略、回滚验证、CI 稳定、最终 release PR |

---

## 9. 验收标准

### 9.1 Tool Call / Stream Parser 验收

| 项目 | 标准 |
|---|---|
| 特殊 token 泄漏 | 常见控制 token 不进入 visible output |
| 半截 chunk | 疑似 token 不提前释放 |
| tool call 结构化 | XML / DSML / JSON tool call 能生成结构化 tool_calls |
| 误杀保护 | 普通 XML、Markdown、代码块不被误判为 tool_call |
| stream 完整性 | role、delta、finish_reason、usage、DONE 正常 |
| Go/Node parity | 核心 fixtures 输出一致 |
| 性能 | 长文本 smoke benchmark 无明显退化 |
| 可回滚 | feature flag 可切回旧逻辑 |

### 9.2 Context Engine 验收

| 项目 | 标准 |
|---|---|
| ContextPlan | 能输出保留、裁剪、摘要、复用说明 |
| 历史连续性 | 多轮任务不再明显孤立 |
| 文件复用 | 已读文件可通过 digest/snapshot 复用 |
| token budget | 当前任务优先，不被历史挤掉 |
| tool pair | tool_call/tool_result 不被裁剪断链 |
| reasoning summary | 可保留推理成果，但不直接塞完整 thinking |
| Chat / Responses shadow | 不破坏旧行为，可对比新旧 prompt |
| 可灰度 | off / shadow / enforce 三态可配置 |

### 9.3 E2E 验收

| 客户端 | 场景 |
|---|---|
| OpenAI SDK | chat stream / non-stream / tool_calls |
| Claude Code | 长任务、文件读取、工具调用、续接 |
| OpenCode / 类 Agent 客户端 | tool_call/tool_result loop |
| Vercel 部署 | Node stream 与 Go 主路径一致性 |
| Docker / 本地 Go | 基础 API 与 Admin 不回归 |

---

## 10. 风险与应对

| 风险 | 表现 | 应对 |
|---|---|---|
| Parser 误杀正常文本 | 普通 XML/Markdown 被吞掉 | 置信度机制 + 误杀 fixtures |
| 流式延迟增加 | quarantine buffer 等待过长 | 设置最大等待时间和最大缓冲长度 |
| Go/Node 不一致 | Vercel 与本地行为不同 | parity fixtures 强制对齐 |
| Context 裁剪错误 | tool_result 留下但 tool_call 被裁掉 | tool pair invariant checker |
| Prompt 行为变化太大 | 模型输出风格漂移 | shadow mode 对比，新能力默认关闭 |
| Subagent PR 过多 | review 与 CI 爆炸 | 小 PR 合入 Phase 分支，正式 PR 控制在 6-8 个 |
| 主链路冲突 | Chat/Responses 被多个 Agent 同时改 | 主链路只允许主控工程师串行接入 |

---

## 11. 后续开发方向

完成本期双核心改造后，建议进入下一轮产品化路线。

### 11.1 Provider 抽象

将 DeepSeek Web 上游从核心 runtime 中解耦，引入 Provider interface：

```text
OpenAI / Claude / Gemini Surface
  -> CanonicalRequest
  -> Provider Interface
  -> DeepSeek Web / DeepSeek Official / Qwen / Kimi / GLM
  -> CanonicalAssistantTurn
  -> Surface Emitter
```

目标：从 DeepSeek Web Adapter 升级成多 Provider LLM Gateway。

### 11.2 安全治理

| 方向 | 建议 |
|---|---|
| Admin 默认 key | 生产环境 fail-closed，强制设置安全密码 |
| 密码存储 | 从 sha256 升级到 bcrypt / argon2id |
| query key | 默认关闭，仅兼容模式开启 |
| 日志脱敏 | token、email、mobile、query key、文件内容脱敏 |
| CORS | 支持 allowlist，Admin 与 API 分开配置 |

### 11.3 可观测性

增加：

- TTFT。
- Upstream TTFT。
- parser_state。
- suppressed_token_count。
- tool_call_count。
- context_plan_id。
- trimmed_segment_count。
- file_snapshot_reused_count。
- retry_count。
- account_switch_count。

### 11.4 WebUI 与文档

| 方向 | 建议 |
|---|---|
| WebUI | TypeScript、统一 API client、路由鉴权、状态管理、测试 |
| 文档 | README 减肥，拆成 deploy、api、security、toolcall、context、troubleshooting |
| Debug UI | 后续展示 ContextPlan、parser event、tool_call trace |

---

## 12. 最终建议

### 12.1 优先级

```text
P0：Tool Call / Stream Parser
P1：Context State Engine
P2：Chat / Responses / Vercel 联调
P3：E2E、文档、灰度、回滚
P4：Provider 抽象、安全治理、WebUI 产品化
```

### 12.2 推荐执行节奏

```text
Day 1：Tool Parser 地基
Day 2：Tool Parser 接入 + Context Engine 地基
Day 3：Context Engine 接入 + E2E
Day 4：修复、回归、文档、灰度
Day 5：发布候选与稳定化
```

### 12.3 一句话结论

本期改造的核心不是“再补几个兼容点”，而是给 ds2api 安装两块新的底盘：

```text
Tool Call / Stream Parser 负责输出正确性；
Context State Engine 负责长任务连续性。
```

一位工程师配合 Subagent 与 worktree，可以用 16 个 2 小时 Phase，在 4 到 5 个工作日内完成推荐稳定版。正式 PR 控制在 6 到 8 个，新能力全部通过 feature flag 先 shadow、再灰度、再默认启用。这样既能快速推进，也能避免把主链路改成一锅冒泡的协议火锅。
