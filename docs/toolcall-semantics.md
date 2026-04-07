# Tool call parsing semantics（Go/Node 统一语义）

本文档描述当前代码中 `ParseToolCallsDetailed` / `parseToolCallsDetailed` 的**实际行为**，用于对齐 Go 与 Node Runtime。

文档导航：[总览](../README.MD) / [架构说明](./ARCHITECTURE.md) / [测试指南](./TESTING.md)

## 1) 输出结构（当前实现）

- `calls`：解析得到的工具调用列表（`name` + `input`）。
- `sawToolCallSyntax`：检测到工具调用语法特征时为 `true`（例如 `tool_calls`、`<tool_call>`、`<function_call>`、`<invoke>`、`function.name:`）。
- `rejectedByPolicy`：当前实现固定为 `false`（预留字段，尚未启用 allow-list 拒绝）。
- `rejectedToolNames`：当前实现固定为空数组（预留字段）。

> 说明：`filterToolCallsDetailed` 当前仅做结构清洗，不做工具名策略拒绝。

## 2) 解析管线

1. **示例保护**：若判定为 fenced code block 示例上下文，则跳过执行型解析。
2. **候选片段构建**：从完整文本中构建候选（原文、围绕 `tool_calls` 的 JSON 片段、首尾大括号切片等）。
3. **按序尝试解析（命中即停）**：
   - 对“明显 JSON 工具载荷候选”（以 `{`/`[` 开头且包含 `tool_calls`/`\"function\"`）先走 JSON 解析，避免 JSON 字符串内偶发 XML 片段误命中；
   - 其余候选优先 XML 解析（`<tool_call>` / `<function_call>` / `<invoke>` / `tool_use` / `antml:function_call` 等）；
   - JSON 解析（`{"tool_calls": [...]}`、列表、单对象）；
   - Markup 解析；
   - Text-KV 回退（如 `function.name:` + `function.arguments:`）。
4. **兜底**：候选全部失败后，再对全文做 XML / Text-KV 回退。

## 3) XML 能力边界（当前）

当前已支持输入端的“多 XML/标记风格”解析，包括但不限于：

- `<tool_call><tool_name>...</tool_name><parameters>...</parameters></tool_call>`
- `<function_call>tool</function_call><function parameter name="x">...</function parameter>`
- `<invoke name="tool"><parameter name="x">...</parameter></invoke>`
- `antml:function_call` / `antml:argument` / `antml:parameters`
- `tool_use` 家族标签

但**输出端仍统一转换为 OpenAI 兼容 JSON 事件/对象**（`message.tool_calls`、`delta.tool_calls`、`response.function_call_arguments.*`）。

## 4) 关于“是否可以封装成 XML 再喂给模型”

结论：**可以做，而且当前解析器已经能兼容 XML 作为输入格式之一**，但代码里并没有 `toolcall.prefer_xml_output` 这个开关。现有可调配置只有：

- `toolcall.mode`：`feature_match` / `off`
- `toolcall.early_emit_confidence`：`high` / `low` / `off`

推荐思路仍然是“输入兼容层 + 输出按客户端协议渲染”：

1. **Prompt 约束层**：如果你要尝试 XML-first，可以在系统提示词里约束模型输出规范 XML tool block（例如 `<tool_calls><tool_call>...</tool_call></tool_calls>`）。
2. **解析兼容层**：继续在 parser 中同时接受 JSON / XML / ANTML / invoke / text-kv。
3. **协议归一层**：无论模型输出什么格式，统一落到内部 `ParsedToolCall`。
4. **对外渲染层**：根据客户端请求协议渲染（OpenAI / Claude / Gemini 各自格式）。

这样可以同时获得：

- 减少模型端 JSON 转义/引号错误；
- 不破坏现有 SDK / 客户端生态；
- 逐步灰度（按模型、按租户、按请求开关）。

## 5) 落地建议（低风险迭代）

- 继续使用现有的 `toolcall.mode=feature_match` 和 `toolcall.early_emit_confidence=high` 作为默认策略。
- 如果要试 XML-first，把它放在 prompt 层或上游模板层，不要假设代码里已有专门的 XML 输出开关。
- 增加观测指标：
  - `toolcall_parse_source`（json/xml/markup/textkv）；
  - `toolcall_parse_success_rate`；
  - `toolcall_malformed_rate`；
  - `toolcall_repair_rate`。
- 先在 `responses` 链路灰度，再扩展 `chat.completions`。

## 6) 兼容性提醒

- 上游模型若输出混合文本 + XML，仍可能出现“半结构化”噪声，需要依赖现有 sieve 增量消费策略。
- XML 不等于安全：仍需做 tool 名、参数 schema、执行权限的服务端校验。
