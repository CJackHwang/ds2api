# DS2API 文档导航 | Documentation Index

语言 / Language: [中文](README.md) | [English](README.md#english)

## 中文

为减少重复维护，本仓库文档按“入口文档 + 专题文档”拆分。建议从下列顺序阅读：

1. [项目总览（README）](../README.MD)
2. [架构与目录说明](./ARCHITECTURE.md)
3. [接口文档（API）](../API.md)
4. [部署指南](./DEPLOY.md)
5. [测试指南](./TESTING.md)
6. [贡献指南](./CONTRIBUTING.md)

### 专题文档

- [Tool Calling 统一语义](./toolcall-semantics.md)
- [DeepSeek SSE 行为结构说明（逆向观察）](./DeepSeekSSE行为结构说明-2026-04-05.md)

### 文档维护约定

- `README.MD` / `README.en.md`：面向首次接触用户，保留“是什么 + 怎么快速跑起来”。
- `docs/ARCHITECTURE*.md`：面向开发者，集中维护项目结构、模块职责与调用链。
- `API*.md`：面向客户端接入者，聚焦接口行为、鉴权和示例。
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
6. [Contributing guide](./CONTRIBUTING.en.md)

### Topical docs

- [Tool-calling unified semantics](./toolcall-semantics.md)
- [DeepSeek SSE behavior notes (reverse-engineered)](./DeepSeekSSE行为结构说明-2026-04-05.md)

### Maintenance conventions

- `README.MD` / `README.en.md`: onboarding-oriented (“what + quick start”).
- `docs/ARCHITECTURE*.md`: developer-oriented source of truth for module boundaries and execution flow.
- `API*.md`: integration-oriented behavior/contracts.
- Other `docs/*.md`: focused topics, avoid copy-pasting the same section into multiple files.
