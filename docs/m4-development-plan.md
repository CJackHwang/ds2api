# M4/M5 后续开发执行规划

文档导航：[文档索引](./README.md) / [未来主规划](./v2-prd.md) / [History Analyzer](./history-analyzer-design.md) / [Auto Continue](./auto-continue-design.md) / [Capability Router](./capability-router.md) / [Release Readiness](./release-readiness.md) / [WebUI v2](./webui-v2-observability.md)

> Status: execution plan for the `docs/v2-prd.md` roadmap.
> 本文把方向型 PRD 拆成后续可执行的里程碑、任务边界和验收口径。若本文与 `docs/v2-prd.md` 冲突，以 `docs/v2-prd.md` 的产品方向为准；若本文与代码实现冲突，以代码和专项设计文档为准。

## 1. 现状基线

| 方向 | 当前已具备 | 主要缺口 |
|---|---|---|
| Tool Parser | `internal/toolcall` / `internal/toolstream`、Go/Node sieve、confidence、shadow diff、fuzz/bench seed、`parser_v2.mode` | 缺真实历史样本自动归类、shadow 汇总报告、WebUI 风险样本面板 |
| Context Engine | `internal/contextengine`、ContextPlan、token budget、tool pair、reasoning summary、shadow 日志、Admin context-plan API | 缺从历史中自动发现 context 异常、Agent profile、Task Memory / Decision Log |
| History 数据 | `internal/chathistory`、`internal/responsehistory`、`internal/devcapture`、`internal/rawsample`、`internal/observe` | 缺统一分析器、规则 ID、报告格式、样本导出流程 |
| Runtime | `internal/completionruntime` 已统一 non-stream 启动、空输出 retry、账号切换重试、current-input 文件重传 | 缺 Auto Continue detector / merger / trace / 配置开关 |
| Capability | 模型 alias、thinking/search/vision/model_type 分散在 `internal/config`、`promptcompat`、upload/client 路径 | 缺显式 capability profile、冲突策略、能力矩阵 |
| WebUI | 已有账号、设置、Chat History、FeatureFlagsSection、API tester | 缺诊断中心、release readiness、parser/context/continue/account/search 观测页 |

## 2. 总体顺序

后续开发按“先证据、再主链路、最后产品化”的顺序推进：

```text
M4.0 Release Readiness baseline
  -> M4.1 History Analyzer CLI / report
  -> M4.2 Parser + Context shadow report
  -> M4.3 Auto Continue MVP
  -> M4.4 Capability Router profile
  -> M4.5 WebUI v2 diagnostics
  -> M5 Agent long-task context
```

核心约束：

- 任何会改变响应行为的能力默认 `off`，先 `shadow` 收集证据，再进入 `enforce`。
- `History Analyzer` 和 `Release Readiness` 优先做，因为它们给后续改造提供样本和晋级依据。
- `Auto Continue` 第一版只处理纯文本 continuation；遇到 tool call / JSON mode / structured output 默认跳过。
- `Capability Router` 第一阶段只做 profile + trace + warning，不直接重写模型选择。
- WebUI 先展示诊断结果和风险解释，再开放危险配置写操作。

## 3. 里程碑任务

### M4.0 Release Readiness Baseline

目标：建立统一发布候选报告，把 parser/context/continue/capability/account 的风险放进同一张表。

任务：

- 新增 release readiness 报告格式，见 [release-readiness.md](./release-readiness.md)。
- 汇总 M3 归档 checklist、当前 feature flag 状态、单元测试、live smoke、shadow diff 数据。
- 明确每个高风险功能从 `off` 到 `shadow`、从 `shadow` 到 `enforce` 的证据要求。

DoD：

- `docs/release-readiness.md` 中的报告模板可直接用于 PR / release。
- 报告能引用 History Analyzer 的异常统计。
- 不需要改主请求链路。

### M4.1 History Analyzer

目标：把已有历史、响应归档、抓包和结构化日志变成可执行诊断报告。

任务：

- 新建 `internal/historyanalyzer` 规则引擎，规则和报告格式见 [history-analyzer-design.md](./history-analyzer-design.md)。
- 第一版提供离线 CLI，扫描 `chathistory` / `responsehistory` 导出或本地数据文件。
- 输出 Markdown + JSON 报告，包含 rule_id、category、severity、evidence、suggested_action。
- 支持把高价值异常样本导出到 fixtures 候选目录，但默认不自动加入回归集。

DoD：

- 能识别 Tool / Context / Continue / Capability / Account-Runtime 五类问题。
- 默认脱敏，不输出 token、账号凭证、完整敏感 prompt。
- 不影响主请求链路。

### M4.2 Parser / Context Shadow Report

目标：让 parser/context 是否可晋级由数据决定。

任务：

- 基于 History Analyzer 汇总 marker leak、false positive、tool pair 断链、reasoning 膨胀、budget trim 风险。
- 为 `parser_v2.mode=shadow` 和 `context_engine.mode=shadow` 增加报告入口。
- 输出 shadow report：样本数、diff 率、严重样本、建议 fixtures、是否满足晋级条件。

DoD：

- 能生成 parser/context 两类独立报告。
- 每个报告都有明确的 `promote_to_shadow` / `promote_to_enforce` 建议，默认保守。
- 报告结论可追溯到具体脱敏样本或日志字段。

### M4.3 Auto Continue MVP

目标：解决长输出截断和 SSE 中断的第一层体感问题。

任务：

- 增加 `auto_continue.mode`：`off` / `shadow` / `enforce`。
- 实现 continuation detector：代码块未闭合、JSON 未闭合、DeepSeek incomplete/continue 状态、stream 异常中断。
- 先支持 OpenAI Chat non-stream，再支持 OpenAI Chat stream。
- 合并输出时记录 trace：continue_count、reason、merge_strategy、stop_reason、fallback_reason。
- tool call / JSON mode / structured output / 多协议 surface 第一版跳过。

DoD：

- `shadow` 模式只检测和记录，不续写。
- `enforce` 模式有 `max_continue_count`、`max_total_ms`、`max_extra_tokens` 上限。
- 回滚只需设回 `off`。
- 详见 [auto-continue-design.md](./auto-continue-design.md)。

### M4.4 Capability Router

目标：把 search、thinking、vision、nothinking、current-input 文件化的冲突显式化。

任务：

- 定义 `ModelCapabilityProfile`，见 [capability-router.md](./capability-router.md)。
- 为现有模型 alias 生成 capability matrix。
- 在 request normalization 阶段输出 trace/warning，不直接改变行为。
- 结合 History Analyzer 识别 search/thinking/current-input 相关异常。

DoD：

- WebUI 能展示模型能力矩阵。
- 每次请求可解释 search/thinking/vision/current-input 的最终策略。
- 第一阶段不接入多 Provider，不改变 DeepSeek 专用路线。

### M4.5 WebUI v2 Diagnostics

目标：把 M4.0-M4.4 的诊断结果产品化。

任务：

- 新增诊断入口：History Analysis、Release Readiness、Parser、Context、Auto Continue、Capability、Account。
- Feature Flags 面板展示风险等级、当前模式、最近报告状态。
- Chat History 详情页展示 analyzer 结论和建议动作。
- Account 页面补充 in-flight、queue、429、切换、冷却、成功率、延迟等指标。

DoD：

- 普通部署者能在 WebUI 里看到“问题属于哪一类、下一步做什么”。
- 危险开关默认只读或二次确认。
- 详见 [webui-v2-observability.md](./webui-v2-observability.md)。

### M5 Agent Long-Task Context

目标：让 ds2api 更适合 Codex、Claude Code、OpenCode、Trae 等长任务 Agent。

任务：

- 在 Context Engine 上叠加 Agent profile。
- 引入 Task Memory / Decision Log 的 shadow 版本。
- File Snapshot 使用 digest 管理“已读 / 已复用 / 已失效”。
- 建立 Agent E2E 测试集：多轮读文件、工具调用、长输出、搜索、恢复。

DoD：

- 多轮任务连续性有 History Analyzer 指标支撑。
- 重复读文件次数下降。
- tool_call / tool_result 不断链。
- Agent profile enforce 前必须有 shadow 报告。

## 4. PR 切分建议

| 顺序 | 建议分支 | 内容 |
|---|---|---|
| 1 | `docs/m4-release-readiness` | 完成 release readiness 文档和报告模板 |
| 2 | `feat/m4-history-analyzer-core` | 规则模型、报告结构、脱敏工具复用 |
| 3 | `feat/m4-history-analyzer-cli` | 离线 CLI、Markdown/JSON 输出 |
| 4 | `feat/m4-shadow-report` | Parser / Context shadow report 汇总 |
| 5 | `feat/m4-auto-continue-config` | 配置、flag、shadow detector |
| 6 | `feat/m4-auto-continue-nonstream` | OpenAI Chat non-stream continuation |
| 7 | `feat/m4-auto-continue-stream` | OpenAI Chat stream merge |
| 8 | `feat/m4-capability-router-profile` | capability profile、trace、WebUI matrix |
| 9 | `feat/m4-webui-diagnostics` | WebUI 诊断页和报告展示 |
| 10 | `feat/m5-agent-context-shadow` | Agent profile / Task Memory shadow |

## 5. 门禁

每个代码 PR 至少运行：

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
```

涉及 streaming、Auto Continue、真实账号、release 的 PR，额外运行：

```bash
./tests/scripts/run-live.sh
```

涉及 prompt / tool / context / API 行为变更时，同步更新：

- `docs/prompt-compatibility.md`
- `docs/toolcall-semantics.md`
- `docs/ARCHITECTURE.md` / `docs/ARCHITECTURE.en.md`
- `API.md` / `API.en.md`
