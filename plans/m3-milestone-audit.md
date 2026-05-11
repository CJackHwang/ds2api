# M3 Milestone Audit

> 收口审计时间：2026-05-11  
> 审计范围：M3 全部 10 个 Stage（Stage 0–9）  
> PR Gate 结论：lint 0 issue / unit 144 pass 0 fail / line-gate clean  

---

## 1. Stage 完成清单

| Stage | 主题 | 分支 | 提交摘要 | 状态 |
|-------|------|------|---------|------|
| S0 | 基线对齐 — 合入 M2 分支 | `main`（已合并） | M2 PR #12 合入 | ✅ |
| S1 | Context Engine — group-based tool pair validation | `feat/m3-context-tool-pair` | `feat(context-engine): M3 Stage 1` | ✅ |
| S2 | Context Engine — token budget trimming | `feat/m3-context-budget` | `feat(context-engine): M3 Stage 2` | ✅ |
| S3 | Context Engine — reasoning summary + doc sync | `feat/m3-context-reasoning-summary` | `feat(context-engine): M3 Stage 3` + 3 correctness fixes | ✅ |
| S4 | Tool Parser — fuzz/benchmark smoke | `feat/m3-toolparser-fuzz-bench` | `feat(toolparser): M3 Stage 4` | ✅ |
| S5 | Tool Parser — shadow diff confidence threshold | `feat/m3-toolparser-shadow-convergence` | `feat(toolparser): M3 Stage 5` | ✅ |
| S6 | 跨主题 E2E pipeline tests + smoke checklist | `feat/m3-crosstheme-e2e` | `feat(crosstheme): M3 Stage 6` | ✅ |
| S7 | Governance — Provider abstraction design doc | `docs/governance-provider-abstraction` | `docs(governance): M3 Stage 7` | ✅ |
| S8 | 默认值晋级 enforce checklist + DEPLOY.md | `chore/m3-enforce-checklist` | `docs(governance): M3 Stage 8` | ✅ |
| S9 | 收口审计（本文档） | `chore/m3-milestone-audit` | — | ✅ |

---

## 2. 新增代码/测试清单

### 2.1 internal/contextengine/

| 文件 | 变更 | 关联 Stage |
|------|------|-----------|
| `compile.go` | `validateToolPairs`, `applyBudgetTrimming`, `extractReasoningBlock`, `summarizeReasoning`, `buildSegments` 扩展 | S1–S3 |
| `compile_test.go` | 新增 8 个 test 函数（tool pair / budget / reasoning dual-source 等） | S1–S3 |

### 2.2 internal/toolcall/

| 文件 | 变更 | 关联 Stage |
|------|------|-----------|
| `fuzz_test.go` | `FuzzParseToolCalls` + `FuzzParseAssistantToolCallsDetailed`（13/15 seeds） | S4 |
| `bench_test.go` | 7 个 benchmark（DSML/XML/multi/large/no-call/mixed/Detailed） | S4 |
| `shadow.go` | `ParseConfidence` type + `ClassifyConfidence` + `ShadowDiffRecord` 置信信号扩展 | S5 |
| `shadow_test.go` | 10 个新 test（ClassifyConfidence 各分支 + 信号传播 + String） | S5 |

### 2.3 internal/promptcompat/

| 文件 | 变更 | 关联 Stage |
|------|------|-----------|
| `prompt_build_test.go` | 4 个跨主题 E2E 测试（shadow/reasoning/Gemini/Responses API） | S6 |

### 2.4 internal/provider/

| 文件 | 变更 | 关联 Stage |
|------|------|-----------|
| `provider.go` | `Provider` / `CompletionRequest` / `CompletionChunk` / `ToolCallChunk` 接口定义（仅占位） | S7 |

### 2.5 docs/

| 文件 | 变更 |
|------|------|
| `docs/prompt-compatibility.md` | Context Engine M3 Stages 1-3 行为描述 |
| `docs/dev-plan-governance.md` | §1.3 Provider 接口设计（§1.3.1–1.3.4） |
| `docs/dev-plan-context-engine.md` | 附录 B：发布候选 checklist（off→shadow / shadow→enforce） |
| `docs/dev-plan-toolparser.md` | 附录 A：发布候选 checklist（off→shadow / shadow→enforce） |
| `docs/DEPLOY.md` | §八 Feature Flags（`DS2API_CONTEXT_ENGINE` / `DS2API_PARSER_V2`） |

### 2.6 plans/

| 文件 | 用途 |
|------|------|
| `plans/stage6-manual-smoke.md` | Stage 6 手工 smoke 检查单（automated-CI 已通过） |
| `plans/m3-milestone-audit.md` | 本收口审计文档 |

---

## 3. 待完成（非 M3 范围，留 M4）

| 项目 | 说明 | 目标里程碑 |
|------|------|-----------|
| `DS2API_CONTEXT_ENGINE=shadow` staging 部署 + 24h 流量收集 | 发布候选 B1 最后一项 | M4 Pre-release |
| `DS2API_PARSER_V2=shadow` staging 部署 + 72h diff 数据 | 发布候选 A2 数据门 | M4 Pre-release |
| `internal/provider/` 具体实现（deepseek-web / gemini 适配器） | Provider 接口落地 | M4 |
| `run-live.sh` 端到端 enforce 验证 | 发布候选 B2/A2 最后一项 | M4 |
| WebUI 集成 context engine mode 显示 | UX 可观测性 | M4 |

---

## 4. 不变量审计

以下 AGENTS.md 规则在 M3 所有变更中均已遵守：

- **Protocol Adapter Boundary**：`contextengine.Compile` / `RunShadowDiff` / `ClassifyConfidence` 均不感知 OpenAI/Claude/Gemini 协议形状；`internal/provider/provider.go` 接口只接受归一化后的 `Prompt string`。
- **文档同步**：所有影响 prompt 归一化行为的变更均同步到 `docs/prompt-compatibility.md`；Provider 设计同步到 `docs/dev-plan-governance.md`；feature flag 操作同步到 `docs/DEPLOY.md`。
- **feature flag 三态**：所有新能力默认 `off`；`shadow` 可观测；`enforce` 有 checklist 门禁。
- **go lint + gofmt**：所有修改文件通过 `./scripts/lint.sh`（golangci-lint v2.11.4，0 issues）。
- **回归测试**：unit 144 pass 0 fail；fuzz seed corpus 无 panic；benchmark 基线已记录。
- **分支纪律**：每个 Stage 独立分支，命名符合规范（feat/m3-* / docs/* / chore/*）；未直接推送 main。
