# DS2API 文档导航 | Documentation Index

语言 / Language: [中文](README.md) | [English](README.md#english)

## 中文

为减少重复维护，本仓库文档按“入口文档 + 专题文档”拆分。建议从下列顺序阅读：

1. [项目总览（README）](../README.MD)
2. [架构与目录说明](./ARCHITECTURE.md)
3. [接口文档（API）](../API.md)
4. [部署指南](./DEPLOY.md)
5. [测试指南](./TESTING.md)
6. [开发者速查](./DEVELOPMENT.md)
7. [贡献指南](./CONTRIBUTING.md)

### 专题文档

- [DS2API 项目价值说明](./project-value.md)
- [API -> 网页对话纯文本兼容主链路说明](./prompt-compatibility.md)
- [Tool Calling 统一语义](./toolcall-semantics.md)
- [DeepSeek SSE 行为结构说明（逆向观察）](./DeepSeekSSE行为结构说明-2026-04-05.md)

### v2 开发规划

- [PRD v1（已归档为规划输入）](./ds_2_api_prd.md)
- [PRD 评审与改进建议](./prd-review-and-improvements.md)
- [v2 开发路线总览（含分支与集成策略）](./dev-roadmap.md)
- [Tool Parser v2 计划：加固 + 可观测](./dev-plan-toolparser.md)
- [Context Engine 计划：渐进引入](./dev-plan-context-engine.md)
- [治理与产品化计划](./dev-plan-governance.md)

### 文档维护约定

- 文档更新必须以实际代码实现为依据：总路由装配看 `internal/server/router.go`，协议/resource 路由看 `internal/httpapi/**/handler*.go` 与 `internal/httpapi/admin/handler.go`，配置默认值看 `internal/config/*`，模型/alias 看 `internal/config/models.go`，prompt 兼容链路看 `docs/prompt-compatibility.md` 列出的代码入口。
- `README.MD` / `README.en.md`：面向首次接触用户，保留“是什么 + 怎么快速跑起来”。
- `docs/ARCHITECTURE*.md`：面向开发者，集中维护项目结构、模块职责与调用链。
- `API*.md`：面向客户端接入者，聚焦接口行为、鉴权和示例。
- `docs/prompt-compatibility.md`：面向维护者，集中维护“API -> 网页对话纯文本上下文”的统一兼容语义；相关行为修改时必须同步更新。
- 其他 `docs/*.md`：主题化说明，避免在多个文档重复粘贴同一段内容。

---

## English

To reduce maintenance drift, docs are split into an “entry doc + topical docs” layout.

Recommended reading order:

1. [Project overview (README)](../README.en.md)
2. [Architecture and project layout](./ARCHITECTURE.en.md)
3. [API reference](../API.en.md)
4. [Deployment guide](./DEPLOY.en.md)
5. [Testing guide](./TESTING.md)
6. [Developer quick reference](./DEVELOPMENT.md)
7. [Contributing guide](./CONTRIBUTING.en.md)

### Topical docs

- [DS2API project value note](./project-value.md)
- [API -> pure-text web-chat compatibility pipeline](./prompt-compatibility.md)
- [Tool-calling unified semantics](./toolcall-semantics.md)
- [DeepSeek SSE behavior notes (reverse-engineered)](./DeepSeekSSE行为结构说明-2026-04-05.md)

### v2 development planning (Chinese only)

- [PRD v1 (archived as planning input)](./ds_2_api_prd.md)
- [PRD review & improvement notes](./prd-review-and-improvements.md)
- [v2 development roadmap (branching & integration policy)](./dev-roadmap.md)
- [Tool Parser v2 plan: harden + observability](./dev-plan-toolparser.md)
- [Context Engine plan: incremental rollout](./dev-plan-context-engine.md)
- [Governance & productization plan](./dev-plan-governance.md)

### Maintenance conventions

- Documentation updates must be grounded in the actual implementation: root routing lives in `internal/server/router.go`, protocol/resource routes live in `internal/httpapi/**/handler*.go` and `internal/httpapi/admin/handler.go`, config defaults in `internal/config/*`, models/aliases in `internal/config/models.go`, and the prompt compatibility pipeline in the code entrypoints listed by `docs/prompt-compatibility.md`.
- `README.MD` / `README.en.md`: onboarding-oriented (“what + quick start”).
- `docs/ARCHITECTURE*.md`: developer-oriented source of truth for module boundaries and execution flow.
- `API*.md`: integration-oriented behavior/contracts.
- `docs/prompt-compatibility.md`: maintainer-oriented source of truth for the “API -> pure-text web-chat context” compatibility flow; update it whenever related behavior changes.
- Other `docs/*.md`: focused topics, avoid copy-pasting the same section into multiple files.
