# History Analyzer 设计草案

文档导航：[文档索引](./README.md) / [未来主规划](./v2-prd.md) / [M4/M5 执行规划](./m4-development-plan.md) / [Release Readiness](./release-readiness.md)

> Status: M4 design draft.
> 目标是先用规则化分析把已有历史数据转成可执行诊断报告，不在第一版引入 LLM 自动判读。

## 1. 目标

History Analyzer 回答五个问题：

1. 这次请求是否异常？
2. 异常属于 Tool、Context、Continue、Capability 还是 Account/Runtime？
3. 证据来自哪里？
4. 应该补 fixtures、改 parser/context、开 Auto Continue，还是检查账号/配置？
5. 当前 release 是否有足够证据进入 shadow / enforce？

## 2. 数据来源

| 来源 | 当前状态 | 用途 |
|---|---|---|
| `internal/chathistory` | 已保存模型、stream、messages、history_text、final_prompt、reasoning、content、error、usage、elapsed | 主分析源 |
| `internal/responsehistory` | 已归档 DeepSeek 上游 assistant text / thinking / tool-call 原始片段 | 判断上游原文与对外响应差异 |
| `internal/devcapture` | 已保存脱敏请求 / 响应 body，带截断标记 | 复盘异常请求 |
| `internal/rawsample` | 已保存原始 SSE 样本 | 深挖流式解析、continue、控制状态 |
| `internal/observe` | 已有 request metrics：TTFT、retry、account switch、parser/context 字段 | 聚合统计和 release readiness |
| `internal/contextengine` plan buffer | 已有 context plan 摘要 | 分析裁剪、warnings、tool pair 风险 |

## 3. 第一版范围

做：

- 离线 CLI 扫描本地历史 / 导出 JSON。
- 规则化分析，不调用 LLM。
- 输出 Markdown + JSON。
- 默认脱敏。
- 生成 fixtures 候选清单。

不做：

- 不自动修改 fixtures。
- 不自动切换 feature flag。
- 不阻断主请求。
- 不读取未授权的外部日志系统。
- 不把完整 prompt / token / 账号凭证写入报告。

## 4. Phase 切分

M4.1 按 stacked PR 推进：

| Phase | 目标 | 边界 |
|---|---|---|
| P1 Core | `internal/historyanalyzer` 核心模型、规则接口、规则 ID 元数据、脱敏证据构造 | 不读取历史文件、不实现具体 HA_* 检测、不提供 CLI |
| P2 Ingest | 把 `chathistory` / `responsehistory` / dev capture / raw sample 引用归一成 `AnalysisRecord` | 只做数据接入和脱敏摘要，不输出最终报告 |
| P3 Rules | 实现首批确定性规则和合成样本单测 | 不调用 LLM，不自动修改 fixtures |
| P4 CLI | 离线 CLI、Markdown/JSON 输出、fixture candidate 清单 | 不接 Admin API，不影响主请求链路 |

每个 Phase 完成后执行偏差检查：是否默认脱敏、是否未改主请求链路、是否没有提前切换 feature flag、是否仍能被 Release Readiness 报告引用。

## 5. 报告模型

```go
type Finding struct {
    RuleID          string
    Category        string // tool | context | continue | capability | account_runtime | protocol
    Severity        string // info | warning | high | critical
    RequestID       string
    SessionID       string
    Evidence        []Evidence
    SuggestedAction string
    FixtureHint     *FixtureHint
}

type Report struct {
    GeneratedAt string
    Scope       ReportScope
    Summary     Summary
    Findings    []Finding
    Readiness   *ReadinessSummary
}
```

P1 已将完整报告结构拆为：

- `AnalysisRecord`：规则输入的统一中间记录。
- `Finding`：单条异常诊断，包含 rule_id、category、severity、request_id、session_id、evidence、suggested_action。
- `Report`：离线分析报告，包含 generated_at、scope、summary、findings、readiness 和 metadata。
- `Rule`：确定性规则接口，后续 P3 逐条实现 HA_* 检测。

## 6. 规则集

| Rule ID | Category | 检测内容 | 建议动作 |
|---|---|---|---|
| `HA_TOOL_MARKER_LEAK` | tool | visible output 中出现 DSML / tool wrapper / DeepSeek control marker | 加入 parser leak corpus |
| `HA_TOOL_CALL_AS_TEXT` | tool | 上游原文像 tool call，但对外响应没有结构化 tool_calls | 补 parser true-positive fixture |
| `HA_TOOL_FALSE_POSITIVE` | tool | Markdown / XML 示例被误识别或正文缺失 | 补 false-positive fixture |
| `HA_CONTEXT_TOOL_PAIR_ORPHAN` | context | history / ContextPlan 中 tool_call 和 tool_result 断链 | 修 Context Engine pair policy |
| `HA_CONTEXT_REASONING_BLOAT` | context | reasoning 历史过大，挤压当前请求 | 调整 reasoning summary |
| `HA_CONTEXT_CURRENT_INPUT_MISMATCH` | context | DS2API_HISTORY / DS2API_TOOLS 文件缺失或 hash 异常 | 检查 current-input 注入 |
| `HA_CONTINUE_CANDIDATE` | continue | 输出截断、代码块未闭合、JSON 未闭合、INCOMPLETE/AUTO_CONTINUE 状态 | 进入 Auto Continue shadow 样本 |
| `HA_CAPABILITY_SEARCH_THINKING_CONFLICT` | capability | search 请求伴随 thinking/current-input 冲突信号 | 检查 capability policy |
| `HA_ACCOUNT_RETRY_RECOVERED` | account_runtime | 空输出 / 429 后账号切换恢复 | 记录账号健康但不一定报错 |
| `HA_ACCOUNT_RETRY_EXHAUSTED` | account_runtime | retry / account switch 后仍失败 | 检查账号池和限流 |

## 7. CLI 形态

建议命令：

```bash
go run ./cmd/history-analyzer \
  --history data/chat_history.json \
  --response-history data/response_history \
  --out artifacts/history-analyzer/report.md \
  --json artifacts/history-analyzer/report.json
```

后续可以封装成：

```bash
./tests/scripts/run-history-analyzer.sh
```

## 8. Admin API 与 WebUI

第二阶段再接 Admin API：

- `GET /admin/history-analysis/reports`
- `GET /admin/history-analysis/reports/{id}`
- `POST /admin/history-analysis/run`
- `POST /admin/history-analysis/export-fixture-candidates`

WebUI 第一版只展示报告，不负责执行重分析。

## 9. 验收

- 至少覆盖 10 条合成历史样本。
- 每个 P0 规则都有单元测试。
- 输出 JSON schema 稳定。
- 报告默认脱敏。
- 运行失败不会影响主服务启动或请求链路。
