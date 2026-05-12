# DS2API v2 开发路线总览

> Status: archived execution record. Future planning is governed by [`../../v2-prd.md`](../../v2-prd.md).

文档导航：[总览](../../../README.MD) / [文档索引](../../README.md) / [归档索引](./README.md) / [架构说明](../../ARCHITECTURE.md) / [PRD v1](./ds_2_api_prd.md) / [PRD 评审](./prd-review-and-improvements.md) / [Tool Parser 计划](./dev-plan-toolparser.md) / [Context Engine 计划](./dev-plan-context-engine.md) / [治理计划](./dev-plan-governance.md)

> 本文是 v2 阶段的**总入口与里程碑索引**。具体执行细节落在三份 `dev-plan-*.md`。

## 1. 路线主轴

PRD v1 提出 “双地基” 思路；v2 在此基础上根据现有代码成熟度重排：

| 主题 | 主轴 | 现状 | v2 定位 |
| --- | --- | --- | --- |
| Tool Parser / Stream | 输出正确性 | 已具备 DSML / canonical 解析、Node sieve、parity 测试 | **加固 + 可观测**，feature flag + shadow diff |
| Context Engine | 长任务连续性 | 暂无；`internal/promptcompat` 已承担请求归一 | **新增 `internal/contextengine/`**，渐进引入 |
| Governance | 安全 / 可观测 / 产品化 | 部分零散 | 集中沉淀（admin、密码、CORS、埋点、Provider、WebUI） |

## 2. 里程碑总览

按周里程碑组织，**串行推进**，避免多个里程碑同时改主链路：

| 里程碑 | 目标 | 跨主题协同 |
| --- | --- | --- |
| **M0（第 1 周）** | 现状盘点、问题证据清单、fixtures 基线、文档与分支基线 | 三主题各产出一份《差距矩阵》 |
| **M1（第 2–3 周）** | 加固与回归：parser 边界回归、context 接口与 segment 模型、安全治理 quick wins | 主链路只允许一个主题改动 |
| **M2（第 4–5 周）** | feature flag + shadow：parser shadow diff、context shadow 注入（仅日志）、可观测性指标接入 | 新能力默认 `off` |
| **M3（第 6 周）** | 联调与发布候选：Chat / Responses / Vercel parity、Claude Code / OpenCode E2E、文档同步、灰度发布 | 跨主题 E2E |
| **M4（第 7 周后，可选）** | Provider 抽象起步、WebUI TS 化排期、Context Debug UI | 长期路线，按需启动 |

> 周数为参考节奏，按实际投入与门禁通过情况浮动。

## 3. 主题入口

- **Tool Parser**：[`dev-plan-toolparser.md`](./dev-plan-toolparser.md) — 加固现有 `internal/toolcall` / `internal/toolstream` / Node sieve，引入 feature flag 与 shadow diff。
- **Context Engine**：[`dev-plan-context-engine.md`](./dev-plan-context-engine.md) — 新建 `internal/contextengine/`，落在 `promptcompat ↔ completionruntime` 之间。
- **Governance**：[`dev-plan-governance.md`](./dev-plan-governance.md) — 安全治理、可观测性、Provider 抽象、WebUI / 文档治理。

## 4. 跨主题硬规则

所有主题计划共用以下规则，不在各自文档中重复：

1. **PR Gate**（与 `AGENTS.md` 一致）：
   - `./scripts/lint.sh`
   - `./tests/scripts/check-refactor-line-gate.sh`
   - `./tests/scripts/run-unit-all.sh`
   - `npm run build --prefix webui`
   - 高风险改动追加 `./tests/scripts/run-live.sh`。
2. **协议适配边界**：所有共享业务逻辑必须落在归一化层；协议 surface 不持有业务行为。详见 `AGENTS.md` *Protocol Adapter Boundary*。
3. **文档同步**：
   - 改 prompt / history / file ref / completion payload → 同次 PR 更新 `docs/prompt-compatibility.md`。
   - 改 tool call 解析语义 → 同次 PR 更新 `docs/toolcall-semantics.md`。
   - 新增 / 调整模块结构 → 同次 PR 更新 `docs/ARCHITECTURE.md`。
4. **feature flag 三态**：所有新能力默认 `off`，再 `shadow`，再 `enforce`。配置入口统一在 `internal/config/`。
5. **回滚优先**：任何里程碑产出都必须可通过关闭 flag 回到旧路径。

## 5. 分支与集成策略

> **`main` 是受保护的集成分支，禁止任何人（含 Agent）直接在 `main` 上 commit / push。** 所有变更必须经过领头分支 + PR。

### 5.1 工作流

1. 从最新 `main` 切出领头分支后再开始任何编辑或写操作。
2. 一个分支只承载一个里程碑或一个逻辑改动；不要在同一分支累积无关 commit。
3. 提交前 `git rebase main` 并跑完 PR Gate。
4. PR 指向 `main`，**squash merge**，commit message 使用语义化前缀（`feat:` / `fix:` / `docs:` / `refactor:` / `chore:` / `perf:` / `style:`）。
5. 上一个里程碑合入 `main` 后再开下一个里程碑分支，避免 worktree 长期漂移。

### 5.2 分支命名

```text
feat/<milestone>-<theme>-<short-slug>     例：feat/m1-toolparser-confidence-rework
fix/<theme>-<short-slug>                  例：fix/toolstream-cdata-overflow
docs/<theme>-<short-slug>                 例：docs/v2-roadmap
chore/<short-slug>                        例：chore/lint-baseline
refactor/<theme>-<short-slug>             例：refactor/promptcompat-history-split
```

主题枚举：`toolparser`、`context`、`governance`、`webui`、`infra`、`docs`。
里程碑枚举：`m0`、`m1`、`m2`、`m3`、`m4`。

### 5.3 何时拆 PR

- 单个 PR 改动行数尽量控制在 `check-refactor-line-gate.sh` 限值之内；超过就拆分。
- 文档变更与代码变更可在同一 PR（推荐），但不要把多个里程碑混在一个 PR。

## 6. 风险与回滚

| 风险 | 应对 |
| --- | --- |
| 里程碑 drift（多分支并行） | 串行推进，上一里程碑合入后再开下一个 |
| 文档与代码不同步 | PR Gate 之外加 “文档同步 checklist” |
| feature flag 残留 | 每次发布候选时审计未使用 flag，及时清理 |
| 主链路被多人同时改 | 同一里程碑同一时间只允许一个主题动主链路 |
| 回归测试覆盖不足 | M0 必须先把 fixtures / parity 基线打稳 |

## 7. 与 PRD v1 的关系

- PRD v1（`docs/ds_2_api_prd.md`）仅作为规划输入保留，不再作为执行口径。
- 评审与差异点见 [`prd-review-and-improvements.md`](./prd-review-and-improvements.md)。
- 三份 `dev-plan-*.md` 是 v2 的执行权威；冲突时以 v2 plan 为准。
