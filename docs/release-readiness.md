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

## 3. 报告模板

```md
# Release Readiness Report

Version / Branch:
Generated at:
Scope:

## Gate Summary

| Gate | Result | Evidence |
|---|---|---|
| lint | PASS/FAIL | link |
| refactor line gate | PASS/FAIL | link |
| unit all | PASS/FAIL | link |
| webui build | PASS/FAIL | link |
| live | PASS/FAIL/SKIP | reason |

## Feature Flag Readiness

| Feature | Current | Target | Decision | Reason |
|---|---|---|---|---|
| parser_v2 | off/shadow/enforce | shadow/enforce | hold/promote | evidence |
| context_engine | off/shadow/enforce | shadow/enforce | hold/promote | evidence |
| auto_continue | off/shadow/enforce | shadow/enforce | hold/promote | evidence |
| capability_router | off/shadow/enforce | shadow/enforce | hold/promote | evidence |

## Analyzer Findings

| Category | Critical | High | Warning | Top rule |
|---|---|---|---|---|

## Release Decision

Decision: GO / NO-GO / GO-WITH-FLAGS-OFF
Required follow-ups:
```

## 4. 晋级规则

默认保守：

- 没有 shadow 数据，不允许直接 `enforce`。
- 有 critical analyzer finding，不允许 release，除非明确不影响本次变更范围。
- Auto Continue 没有 live smoke，不允许 stream enforce。
- Parser / Context diff 无人工审阅，不允许默认开启。

## 5. 验收

- 报告可以由脚本生成初稿。
- 人工只补决策和证据链接。
- 报告不包含未脱敏 artifacts。
- 每次 release 可追溯到一份报告。
