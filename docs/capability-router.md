# DeepSeek Capability Router 设计草案

文档导航：[文档索引](./README.md) / [未来主规划](./v2-prd.md) / [M4/M5 执行规划](./m4-development-plan.md) / [WebUI v2](./webui-v2-observability.md)

> Status: M4 design draft.
> 本文只讨论 DeepSeek 专用能力路由，不引入多 Provider 抽象。

## 1. 目标

把 search、thinking、vision、nothinking、current-input 文件化、session split 的关系显式化，避免能力冲突散落在模型名后缀、prompt 拼接和各 handler 中。

## 2. 当前基础

| 已具备 | 说明 |
|---|---|
| `internal/config/models.go` | 模型 alias、模型列表、thinking/search/model_type 基础映射 |
| `promptcompat.StandardRequest` | 已包含 requested/resolved/response model、thinking、search、tool choice、ref files |
| Upload model type | 文件上传已能根据模型透传 `default` / `expert` / `vision` |
| current-input 文件化 | 已有 DS2API_HISTORY / DS2API_TOOLS 上传路径 |
| observe metrics | 可增加能力路由 trace 字段 |

## 3. Profile 模型

```go
type ModelCapabilityProfile struct {
    Alias                    string
    ResolvedModel            string
    ModelType                string // default | expert | vision
    SupportsSearch           bool
    SupportsThinking         bool
    SupportsVision           bool
    SupportsCurrentInputFile bool
    DefaultThinking          bool
    IsNoThinkingVariant      bool
    SearchThinkingPolicy     string // allow | warn | force_no_thinking | block
    CurrentInputSearchPolicy string // allow | warn | disable_current_input | block
    Notes                    []string
}
```

## 4. Router 模式

建议新增：

```json
{
  "capability_router": {
    "mode": "shadow"
  }
}
```

| 模式 | 行为 |
|---|---|
| `off` | 不生成 route plan |
| `shadow` | 生成 capability plan 和 warning，不改变请求 |
| `enforce` | 根据 policy 自动降级或拒绝冲突请求 |

第一阶段只实现 `shadow`。

## 5. Policy 示例

| 冲突 | shadow 行为 | enforce 候选行为 |
|---|---|---|
| search + thinking 冲突 | 记录 warning | 可按模型 profile 关闭 thinking |
| nothinking 仍出现 reasoning | 记录模型违约 | 后续可强清 reasoning 或提示风险 |
| vision 请求未带 vision model_type | 记录 warning | 自动使用 vision upload type |
| current-input 与 search 冲突 | 记录 warning | 可禁用 current-input 或要求用户确认 |
| unknown alias | 记录 profile missing | 返回清晰错误或 fallback |

## 6. Trace 字段

| 字段 | 说明 |
|---|---|
| `capability_model_alias` | 请求模型 |
| `capability_resolved_model` | 解析后模型 |
| `capability_profile` | profile 名称或 hash |
| `capability_search_enabled` | 最终 search |
| `capability_thinking_enabled` | 最终 thinking |
| `capability_current_input_enabled` | 是否触发 current-input |
| `capability_warnings` | 冲突 warning 数量或 codes |

## 7. WebUI Matrix

WebUI 展示：

- 模型 alias。
- search / thinking / vision / current-input 支持情况。
- 默认策略。
- 风险说明。
- 最近 24h 相关异常数。

## 8. 验收

- 所有公开模型 alias 都有 profile。
- shadow 模式不改变现有响应。
- search/thinking/current-input 的冲突能被 History Analyzer 归类。
- Capability Router 不依赖多 Provider，也不感知 OpenAI / Claude / Gemini 协议外形。
