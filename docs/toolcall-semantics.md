# Tool call parsing semantics（Go/Node 统一语义）

本文档描述当前代码中工具调用解析链路的**实际行为**（以 `internal/toolcall` 与 `internal/js/helpers/stream-tool-sieve` 为准）。

文档导航：[总览](../README.MD) / [架构说明](./ARCHITECTURE.md) / [测试指南](./TESTING.md)

## 1) 当前输出结构

`ParseToolCallsDetailed` / `parseToolCallsDetailed` 返回：

- `calls`：解析出的工具调用列表（`name` + `input`）。
- `sawToolCallSyntax`：检测到工具调用语法特征时为 `true`。
- `rejectedByPolicy`：当前实现固定为 `false`（预留字段）。
- `rejectedToolNames`：当前实现固定为空数组（预留字段）。

> 当前 `filterToolCallsDetailed` 仅做结构清洗，不做 allow-list 工具名硬拒绝。

## 2) 解析范围（重点）

当前版本的可执行解析以 **XML/Markup 家族**为主：

- `<tool_call>...</tool_call>`
- `<function_call>...</function_call>`
- `<invoke ...>...</invoke>`（含自闭合）
- `<tool_use>...</tool_use>`
- antml 变体（如 `antml:function_call` / `antml:argument`）

并支持在这些标记块内部解析：

- JSON 参数字符串
- 标签参数（`<parameter name="...">...`）
- key/value 风格子标签

## 3) 不应再假设的行为

以下说法在当前实现中已不成立：

1. “纯 JSON `tool_calls` 片段会被直接当作可执行工具调用解析”。
2. “存在 `toolcall.mode` / `toolcall.early_emit_confidence` 等可配置开关可以改变解析策略”。

当前策略在代码中固定为：

- 特征匹配开启（feature-match on）
- 高置信度早发开启（early emit on）
- policy 拒绝字段保留但未启用

## 4) 流式与防泄漏语义

在流式链路中（OpenAI / Claude / Gemini 统一内核）：

- 工具调用片段会被优先提取为结构化增量输出；
- 已识别的工具调用原始片段不会作为普通文本再次回流；
- fenced code block 中的示例内容按文本处理，不作为可执行工具调用。

## 5) 落地建议（按当前实现）

1. Prompt 里优先约束模型输出 XML/Markup 工具块。
2. 执行器侧继续做工具名白名单与参数 schema 校验（不要依赖 parser 代替安全策略）。
3. 需要兼容历史“纯 JSON tool_calls”模型输出时，请在上游模板层把输出规范化为 XML/Markup 风格再进入 DS2API。

## 6) 回归验证建议

可直接运行：

```bash
go test -v -run 'TestParseToolCalls|TestRepair' ./internal/toolcall/
node --test tests/node/stream-tool-sieve.test.js
```

重点覆盖：

- `<tool_call>` / `<function_call>` / `<invoke>` / `tool_use` / antml 变体
- 参数 JSON 修复与解析
- 流式增量下的工具调用提取与文本防泄漏
