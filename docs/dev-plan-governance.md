# 治理与产品化开发计划：安全 / 可观测 / Provider / WebUI

文档导航：[总览](../README.MD) / [文档索引](./README.md) / [开发路线总览](./dev-roadmap.md) / [PRD 评审](./prd-review-and-improvements.md) / [架构说明](./ARCHITECTURE.md) / [部署指南](./DEPLOY.md)

> 本文是 v2 阶段 Governance 主题的执行计划，覆盖 Tool Parser / Context Engine 之外的“地基性”治理能力：
> 安全治理、可观测性、Provider 抽象（长期）、WebUI / 文档治理。
> 跨主题硬规则（PR Gate / 文档同步 / 分支命名）见 [`dev-roadmap.md`](./dev-roadmap.md)。

## 1. 主题与现状

### 1.1 安全治理

| 维度 | 现状（需 M0 复核） | 入口 |
| --- | --- | --- |
| Admin 默认 key / fail-closed | 需要复核默认值与缺失配置时的行为 | `internal/auth/admin.go`、`internal/config/codec.go` |
| 密码哈希算法 | 当前疑似 sha256，待确认 | `internal/auth/admin.go` |
| query key 兼容 | 仍存在，需评估关停成本 | `internal/auth/`、`internal/server/router.go` |
| CORS allowlist | 现状待复核 | `internal/server/router.go` |
| 日志脱敏 | 现状待复核（token / email / mobile / 文件内容） | `internal/util/`、`internal/devcapture/`、`internal/responsehistory/` |

### 1.2 可观测性

- 现有：`internal/devcapture`、`internal/rawsample`、`internal/responsehistory`、`internal/chathistory`，主要服务调试与历史归档。
- 缺失：请求级指标（TTFT / 上游 TTFT / retry / account switch）与解析 / context 编译事件指标。
- 未对接外部 metrics 系统；先以结构化日志为统一出口即可。

### 1.3 Provider 抽象（长期）

- 现状：`internal/deepseek/client/protocol/transport` 紧耦合 DeepSeek 网页 completion。
- 长期目标：抽出 Provider interface，让 DeepSeek 网页 / 官方 / 其他上游可替换。
- 本期不实现，仅做接口预留与目录占位。

### 1.4 WebUI / 文档

- WebUI 是 React + Vite + Tailwind，未 TS 化；本期不重构。
- 文档体量已较大；本期只做 v2 文档矩阵与索引完善。

## 2. 里程碑

### 2.1 M0：证据复核 + 埋点盘点（约 1 周，与其他主题并行）

任务：

- 安全维度逐条复核（见 §1.1），把每条结论以 issue 形式记录：
  - 是否真的存在默认 Admin key？
  - 密码 hash 实际算法？
  - query key 当前的鉴权路径？
  - CORS 默认行为？
  - 日志中是否有真实可观测的敏感字段？
- 可观测性维度：把现有埋点（devcapture / rawsample / responsehistory / chathistory）做一次盘点，输出《埋点与日志清单》到 `docs/dev-plan-governance.md` 附录。
- 与 Tool Parser / Context Engine 计划对齐新增指标命名，避免冲突。

DoD：

- 安全证据清单 + 埋点清单进入本文档附录。
- 不在本里程碑做修复。

分支：`docs/governance-baseline`。

### 2.2 M1：安全 quick wins（约 1–2 周）

> 仅做不破坏现有部署的最小修复。重大变更（如密码 hash 升级）放 M2。

任务：

- Admin key fail-closed：缺失或默认值时拒绝启动并打印明确错误（不静默使用默认值）。
- 日志脱敏：补 token / email / mobile / 文件内容的字段级脱敏；新增结构化日志字段时统一过过滤函数。
- query key：新增配置开关，默认仍兼容；文档与发布说明里加 “建议关闭” 提示。
- CORS：明确 allowlist 配置入口；文档补充示例。

DoD：

- 不影响已有部署，但提供更安全的默认开关。
- `docs/DEPLOY.md` 同步更新对应配置说明。

分支：`fix/governance-admin-failclosed`、`feat/m1-governance-log-redaction`、`feat/m1-governance-cors-allowlist`。

### 2.3 M2：密码 hash 升级 + 可观测性 v1（约 1–2 周）

任务：

- 密码 hash：从 sha256 升级到 argon2id（或 bcrypt，按依赖成本择一）：
  - 兼容旧 hash：登录时旧 hash 验证通过后自动重写为新 hash。
  - 文档说明迁移路径。
- 结构化日志指标 v1（输出到日志，不强求接 metrics 系统）：
  - `request_id`、`route`、`model_alias`、`account_id`（脱敏）、`ttft_ms`、`upstream_ttft_ms`、`retry_count`、`account_switch_count`。
  - 与 Tool Parser M2 / Context Engine M2 的指标共用一套 logger 字段约定。
- 文档：`docs/DEPLOY.md` 增加 “观测与日志” 小节。

DoD：

- 密码迁移路径有自动化测试覆盖。
- 关键指标字段在主链路日志中可见。
- 文档同步。

分支：`feat/m2-governance-password-hash`、`feat/m2-governance-observability`。

### 2.4 M3：Provider 抽象起步（可选，约 1 周）

任务：

- 设计 `internal/provider/` 接口草案（不实现具体替换）。
- 把 `internal/deepseek/client` 的对外调用按 Provider 接口梳理一遍，标注哪些行为属于 Provider、哪些属于业务层。
- 仅出设计稿（`docs/provider-abstraction.md`）+ 占位 `doc.go`，不动主链路。

DoD：

- 设计稿可作为下一阶段实现起点。
- 现有 DeepSeek 调用零回归。

分支：`docs/provider-abstraction-design`、`feat/m3-provider-skeleton`。

### 2.5 M4+：长期路线（不在本期承诺）

- WebUI TypeScript 化：分批迁移，先 `webui/src/utils` 与 `webui/src/features/settings`。
- Context Debug UI：Admin 页面展示 ContextPlan（依赖 Context Engine M2 的只读接口）。
- 多租户 / 计费 / 审计：仅作为占位，不在本期排期。
- 文档拆分：`README` 减肥，按主题（deploy / api / security / toolcall / context / troubleshooting）继续拆。

## 3. 与其他主题的协同

- 任何新增日志字段必须先在本文档附录登记，避免命名冲突。
- 任何 feature flag 必须复用 `internal/config/` 的同一模式（参考 Tool Parser / Context Engine 的 `*.engine` / `*.v2` 命名）。
- 涉及鉴权 / CORS / Admin 行为变更时，必须在 PR description 标注 “影响部署兼容性”，发布说明同步。

## 4. 风险与回滚

| 风险 | 应对 |
| --- | --- |
| Admin fail-closed 影响存量部署 | 提供过渡期：先打 warning，再下个版本 fail-closed |
| 密码 hash 升级阻塞老用户登录 | 登录时透明迁移；保留旧 hash 至少一个版本 |
| 日志脱敏漏字段 | 在 `internal/util/` 集中过滤函数 + 单元测试覆盖典型字段 |
| 指标命名冲突 | M0 埋点清单作为唯一事实来源 |

## 5. 验收

- §1 各维度结论已写入本文档附录（M0 输出）。
- M1 / M2 任务通过 PR Gate 合入；`docs/DEPLOY.md` 同步。
- Provider 设计稿存在（M3 完成后）。
- 长期路线在本文档登记，不强求本期完成。

## 附录 A：安全证据清单（M0 待填）

> 在 M0 完成后填写。每行格式：
> `维度 | 复现入口 | 现状结论 | 排期里程碑`

## 附录 B：埋点与日志清单（M0 待填）

> 在 M0 完成后填写。每行格式：
> `字段 / 事件 | 产生位置 | 现状 | v2 命名建议`
