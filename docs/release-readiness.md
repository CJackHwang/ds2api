# Release Readiness 报告规范

文档导航：[文档索引](./README.md) / [未来主规划](./v2-prd.md) / [M4/M5 执行规划](./m4-development-plan.md) / [History Analyzer](./history-analyzer-design.md)

> Status: M4 design draft.
> 本文定义发布前如何把测试、shadow 数据、历史诊断和手工检查合成一份 release readiness 报告。

## 1. 目标

Release Readiness 不是替代 PR Gate，而是回答：

- 当前版本是否可发？
- 哪些高风险功能仍只能保持 `off` / `shadow`？
- 哪些异常来自 parser、context、continue、capability、account？
- 是否有足够证据把某个功能推进到 `enforce`？

## 2. 数据来源

| 来源 | 要求 |
|---|---|
| PR Gate | `lint.sh`、`check-refactor-line-gate.sh`、`run-unit-all.sh`、WebUI build |
| Live Gate | 高风险改动运行 `run-live.sh`，产物脱敏 |
| History Analyzer | 输出异常统计和高风险样本 |
| Parser Shadow Report | diff 率、confidence 分布、marker leak |
| Context Shadow Report | warnings、trimmed、tool pair、budget |
| Auto Continue Shadow | candidate 数、跳过原因、预计收益 |
| Capability Router Shadow | search/thinking/current-input 冲突统计 |
| Account Metrics | 429、retry、account switch、TTFT、total_ms |

## 3. M4.0 交付边界

M4.0 只建立 release readiness baseline，不改变任何主请求链路行为。

做：

- 固化报告模板、决策口径和 feature flag 晋级规则。
- 提供后续脚本生成报告初稿所需的数据字段。
- 允许 History Analyzer、Parser Shadow Report、Context Shadow Report、Auto Continue Shadow、Capability Router Shadow 在缺失时标记为 `pending`。
- 让 release 决策可以清楚说明证据来源和缺口。

不做：

- 不实现 History Analyzer 规则。
- 不自动切换 feature flag。
- 不把 parser、context、auto continue 或 capability router 推到 `enforce`。
- 不读取或输出未脱敏 prompt、token、账号凭证和完整真实请求。
- 不把 live test 失败或缺凭证隐藏成通过结果。

## 4. 报告模板

```md
# Release Readiness Report

Version / Branch:
Generated at:
Scope:
Decision owner:

## Gate Summary

| Gate | Result | Evidence | Notes |
|---|---|---|---|
| lint | PASS/FAIL/UNKNOWN | link or local log | |
| refactor line gate | PASS/FAIL/UNKNOWN | link or local log | |
| unit all | PASS/FAIL/UNKNOWN | link or local log | |
| webui build | PASS/FAIL/UNKNOWN | link or local log | |
| live | PASS/FAIL/SKIP/UNKNOWN | link or reason | |

## Feature Flag Readiness

| Feature | Current | Target | Decision | Evidence | Missing Evidence |
|---|---|---|---|---|---|
| parser_v2 | off/shadow/enforce | off/shadow/enforce | hold/promote/disable | | |
| context_engine | off/shadow/enforce | off/shadow/enforce | hold/promote/disable | | |
| auto_continue | off/shadow/enforce | off/shadow/enforce | hold/promote/disable | | |
| capability_router | off/shadow/enforce | off/shadow/enforce | hold/promote/disable | | |

## Analyzer Findings

| Category | Critical | High | Warning | Top rule |
|---|---|---|---|---|
| tool | 0 | 0 | 0 | |
| context | 0 | 0 | 0 | |
| continue | 0 | 0 | 0 | |
| capability | 0 | 0 | 0 | |
| account_runtime | 0 | 0 | 0 | |

## Shadow Evidence

| Source | Status | Samples | Summary |
|---|---|---|---|
| history analyzer | PASS/FAIL/PENDING | count | |
| parser shadow | PASS/FAIL/PENDING | count | |
| context shadow | PASS/FAIL/PENDING | count | |
| auto continue shadow | PASS/FAIL/PENDING | count | |
| capability router shadow | PASS/FAIL/PENDING | count | |

## Release Decision

Decision: GO / NO-GO / GO-WITH-FLAGS-OFF
Reason:
Required follow-ups:
```

## 5. 决策口径

| Decision | 含义 | 必要条件 |
|---|---|---|
| `GO` | 可以按目标配置发布 | PR Gate 通过；无未解释 critical finding；目标 feature flag 有对应证据 |
| `GO-WITH-FLAGS-OFF` | 可以发布，但高风险能力保持 `off` 或 `shadow` | 主链路改动低风险；高风险能力未默认开启；缺失证据已列为 follow-up |
| `NO-GO` | 不建议发布 | PR Gate 失败；存在未解释 critical finding；或高风险能力缺少回滚路径 |

默认优先选择 `GO-WITH-FLAGS-OFF`，除非本次发布明确需要启用高风险能力。

## 6. 晋级规则

默认保守：

- 没有 shadow 数据，不允许直接 `enforce`。
- 有 critical analyzer finding，不允许 release，除非明确不影响本次变更范围。
- Auto Continue 没有 live smoke，不允许 stream enforce。
- Parser / Context diff 无人工审阅，不允许默认开启。

### 6.1 Feature Flag 晋级矩阵

| Feature | `off -> shadow` 证据 | `shadow -> enforce` 证据 | 强制保持或回退条件 |
|---|---|---|---|
| `parser_v2` | shadow 入口已就绪；报告能统计 diff、confidence、marker leak；不改变对外响应 | diff 经人工审阅；marker leak 和 false positive 均有 fixtures；Go/Node parity 可解释 | 出现未解释 marker leak、tool call 丢失、跨 chunk 误判 |
| `context_engine` | ContextPlan 和 warning 可记录；tool pair 风险可被报告引用；shadow 不改变请求 | 多轮任务 shadow 稳定；tool_call/tool_result 不断链；reasoning summary 不挤压当前请求 | tool pair orphan、当前请求被错误裁剪、current-input hash 异常 |
| `auto_continue` | detector shadow 能记录 candidate、skip reason、trace；默认不续写 | non-stream smoke 通过；有 `max_continue_count`、`max_total_ms`、`max_extra_tokens`；失败可回退 | tool call、JSON mode、structured output、stream continuation 缺 live smoke |
| `capability_router` | 所有公开 alias 有 profile；shadow 只产生 warning；冲突可被 History Analyzer 归类 | policy 经样本验证；冲突处理不破坏 search/thinking/current-input 语义 | unknown alias、search/thinking 冲突不可解释、vision/current-input 策略不完整 |

### 6.2 数据缺失处理

缺少某类证据时，报告必须显式标记：

- `PENDING`：数据源尚未实现或没有可用样本。
- `SKIP`：本次不适用，并写明原因，例如没有 live credentials。
- `UNKNOWN`：命令或数据源未运行，不能作为通过证据。

`PENDING` 和 `UNKNOWN` 不能支持 feature flag 晋级到 `enforce`。

## 7. Phase Closure Review

每个 M4.0 Phase 完成后，必须对照以下问题做偏差检查：

1. 是否仍符合 `docs/v2-prd.md` 的 DeepSeek 专用 Agent API 引擎方向？
2. 是否没有改动主请求链路？
3. 是否没有提前实现 M4.1/M4.2 的分析规则？
4. 是否所有高风险能力仍默认 `off` 或只允许 `shadow`？
5. 是否报告、模型、CLI 的职责边界清楚？
6. 是否没有输出未脱敏 artifacts？
7. 是否满足本仓库 PR Gate 和 stacked PR 分支纪律？

如发现偏差，必须在进入下一 Phase 前提交修复 PR 或修复 commit。

## 8. 验收

- 报告可以由脚本生成初稿。
- 人工只补决策和证据链接。
- 报告不包含未脱敏 artifacts。
- 每次 release 可追溯到一份报告。
- 缺失 History Analyzer 或 shadow report 时，报告仍可生成，并清楚标记 `PENDING`。

## 9. 本地生成器

M4.0 提供轻量本地生成器，只负责组装 readiness baseline，不执行 History Analyzer 规则，也不自动运行 PR Gate。

```bash
go run ./cmd/release-readiness \
  --branch current \
  --out artifacts/release-readiness/report.md \
  --json artifacts/release-readiness/report.json \
  --lint-result pass \
  --refactor-result pass \
  --unit-result pass \
  --webui-build-result pass \
  --live-result skip \
  --live-skip-reason "no credentials"
```

也可以使用脚本封装：

```bash
DS2API_READINESS_LINT_RESULT=pass \
DS2API_READINESS_REFACTOR_RESULT=pass \
DS2API_READINESS_UNIT_RESULT=pass \
DS2API_READINESS_WEBUI_BUILD_RESULT=pass \
tests/scripts/run-release-readiness.sh
```

未来输入预留：

- `--history-analyzer-json`
- `--parser-shadow-json`
- `--context-shadow-json`
- `--auto-continue-json`
- `--capability-router-json`

当这些输入缺失时，报告会把对应来源标记为 `pending`，不能作为晋级到 `enforce` 的证据。
