# API -> 网页对话纯文本兼容主链路说明

文档导航：[总览](../README.MD) / [架构说明](./ARCHITECTURE.md) / [接口文档](../API.md) / [测试指南](./TESTING.md)

> 本文档是 DS2API“把 OpenAI / Claude / Gemini 风格 API 请求兼容成 DeepSeek 网页对话纯文本上下文”的专项说明。
> 这是项目最重要的兼容产物之一。凡是修改消息标准化、tool prompt 注入、tool history 保留、文件引用、history split、下游 completion payload 组装等行为，都必须同步更新本文档。

## 1. 核心结论

DS2API 当前的核心思路，不是把客户端传来的 `messages`、`tools`、`attachments` 原样转发给下游。

而是把这些高层 API 语义，统一压缩成 DeepSeek 网页对话更容易理解的三类输入：

1. `prompt`
   一个单字符串，里面带有角色标记、system 指令、历史消息、assistant reasoning 标签、历史 tool call XML 等。
2. `ref_file_ids`
   一个文件引用数组，承载附件、inline 上传文件，以及必要时被拆出去的历史文件。
3. 控制位
   例如 `thinking_enabled`、`search_enabled`、部分 passthrough 参数。

也就是说，项目最重要的兼容动作，是把“结构化 API 会话”翻译成“网页对话纯文本上下文 + 文件引用”。

## 2. 为什么这是核心产物

因为对下游来说，真正稳定的输入面不是 OpenAI/Claude/Gemini 的原生 schema，而是：

- 一段连续的对话 prompt
- 一组可引用文件
- 少量开关位

这也是为什么很多表面上看像“协议兼容”的代码，最终都会收敛到同一类逻辑：

- 先把不同协议的消息统一成内部消息序列
- 再把工具声明改写成 system prompt 文本
- 再把历史 tool call / tool result 改写成 prompt 可见内容
- 最后输出成 DeepSeek completion payload

## 3. 统一心智模型

当前主链路可以这样理解：

```text
客户端请求
  -> HTTP API surface（OpenAI / Claude / Gemini）
  -> promptcompat 统一消息标准化
  -> tool prompt 注入
  -> DeepSeek 风格 prompt 拼装
  -> 文件收集 / inline 上传 / history split（OpenAI 链路）
  -> completion payload
  -> 下游网页对话接口
```

对应的关键代码入口：

- OpenAI Chat / Responses：
  [internal/promptcompat/request_normalize.go](../internal/promptcompat/request_normalize.go)
- OpenAI prompt 组装：
  [internal/promptcompat/prompt_build.go](../internal/promptcompat/prompt_build.go)
- OpenAI 消息标准化：
  [internal/promptcompat/message_normalize.go](../internal/promptcompat/message_normalize.go)
- Claude 标准化：
  [internal/httpapi/claude/standard_request.go](../internal/httpapi/claude/standard_request.go)
- Claude 消息与 tool_use/tool_result 归一：
  [internal/httpapi/claude/handler_utils.go](../internal/httpapi/claude/handler_utils.go)
- Gemini 复用 OpenAI prompt builder：
  [internal/httpapi/gemini/convert_request.go](../internal/httpapi/gemini/convert_request.go)
- DeepSeek prompt 角色标记拼装：
  [internal/prompt/messages.go](../internal/prompt/messages.go)
- prompt 可见 tool history XML：
  [internal/prompt/tool_calls.go](../internal/prompt/tool_calls.go)
- completion payload：
  [internal/promptcompat/standard_request.go](../internal/promptcompat/standard_request.go)

## 4. 下游真正收到的东西

在“完成标准化后”，下游 completion payload 的核心形态是：

```json
{
  "chat_session_id": "session-id",
  "model_type": "default",
  "parent_message_id": null,
  "prompt": "<｜begin▁of▁sentence｜>...",
  "ref_file_ids": [
    "file-history",
    "file-systemprompt",
    "file-other-attachment"
  ],
  "thinking_enabled": true,
  "search_enabled": false
}
```

重点是：

- `prompt` 才是对话上下文主载体。
- `ref_file_ids` 只承载文件引用，不承载普通文本消息。
- `tools` 不会作为“原生工具 schema”直接下发给下游，而是被改写进 `prompt`。
- OpenAI Chat / Responses 原生走统一 OpenAI 标准化与 DeepSeek payload 组装；Claude / Gemini 会尽量复用 OpenAI prompt/tool 语义，其中 Gemini 直接复用 `promptcompat.BuildOpenAIPromptForAdapter`，Claude 消息接口在可代理场景会转换为 OpenAI chat 形态再执行。
- 客户端传入的 thinking / reasoning 开关会被归一到下游 `thinking_enabled`。Gemini `generationConfig.thinkingConfig.thinkingBudget` 会翻译成同一套 thinking 开关；关闭时即使上游返回 `response/thinking_content`，兼容层也不会把它当作可见正文输出。Claude surface 在流式请求且未显式声明 `thinking` 时，仍按 Anthropic 语义默认关闭；但在非流式代理场景，兼容层会内部开启一次下游 thinking，用于捕获“正文为空、工具调用落在 thinking 里”的情况，随后在回包前剥离用户不可见的 thinking block。
- 对 OpenAI Chat / Responses 的非流式收尾，如果最终可见正文为空，兼容层会优先尝试把思维链中的独立 `<tool_calls>...</tool_calls>` 结构当作真实工具调用解析出来。流式链路也会在收尾阶段做同样的 fallback 检测，但不会因为思维链内容去中途拦截或改写流式输出；thinking / reasoning 增量仍按原样先发，只有在结束收尾时才可能补发最终工具调用结果。只有正文为空且思维链里也没有可执行工具调用时，才继续按空回复错误处理。
- 对 OpenAI Chat / Responses 的非流式空正文场景，如果正文为空、未命中 content filter、且也没有可执行工具调用，兼容层会先按原始 `prompt` 追加一个 UTC 日期时间标签 `[retry_datetime_utc]...[/retry_datetime_utc]` 后向下游重试一次；不会额外注入补救提示词。只有重试后仍然没有可见正文时，才返回 `upstream_empty_output`。
- 对 OpenAI 历史拆分链路，如果最新 user 文本过长，兼容层会先把该轮完整文本上传成 `CURRENT_INPUT.txt`，再把更早历史上传成 `HISTORY.txt`。管理后台 `/admin/chat-history/{id}` 会持久化并展示这两次上传的文件名、字节数、file id 与最终 `ref_file_ids`，用于确认“是否真的上传成功且替换了正文内联”。
- 对 OpenAI 历史拆分链路，如果已经进入“需要压缩上下文”的区间，兼容层还会额外上传 `TASK_STATE.txt`。这个文件不是原始历史转储，而是当前任务连续性的短摘要：最新用户请求、当前文件引用、DSML 工具格式硬约束、以及本轮 `CURRENT_INPUT` / `HISTORY` 判定结果。它的目的不是替代原文，而是让模型在长对话里更稳定地保持任务目标和格式约束。

## 5. prompt 是怎么拼出来的

### 5.1 角色标记

最终 prompt 使用 DeepSeek 风格角色标记：

- `<｜begin▁of▁sentence｜>`
- `<｜System｜>`
- `<｜User｜>`
- `<｜Assistant｜>`
- `<｜Tool｜>`
- `<｜end▁of▁instructions｜>`
- `<｜end▁of▁sentence｜>`
- `<｜end▁of▁toolresults｜>`

实现位置：
[internal/prompt/messages.go](../internal/prompt/messages.go)

### 5.2 thinking continuity 说明

如果启用了 thinking，会在最前面额外插入一个 system block，提醒模型：

- 继续既有会话，不要重开
- earlier messages 是 binding context
- 不要把最终回答只留在 reasoning 里

这部分不是客户端原始消息，而是兼容层主动补进去的连续性契约。

### 5.3 相邻同角色消息会合并

在最终 `MessagesPrepareWithThinking` 中，相邻同 role 的消息会被合并成一个块，中间插入空行。

这意味着：

- prompt 中看到的是“合并后的 role block”
- 不是客户端传来的逐条 message 原样排列

## 6. tools 为什么是“文本注入”，不是原生下发

当前项目把工具能力视为“prompt 约束的一部分”。

具体做法：

1. 把每个 tool 的名称、描述、参数 schema 序列化成文本。
2. 拼成 `You have access to these tools:` 大段说明。
3. 再附上统一的 XML tool call 格式约束。
4. 把这整段内容并入 system prompt。

工具调用正例现在优先示范官方 DSML 风格：`<|DSML|tool_calls>` → `<|DSML|invoke name="...">` → `<|DSML|parameter name="..." string="true|false">`。
兼容层现在已真实支持 DSML wrapper：Go/Node 的非流式解析、流式 tool sieve、thinking fallback 都会先把 DSML 归一化到内部 XML parser，再执行同一套工具调用逻辑。
兼容层仍接受旧式纯 `<tool_calls>` wrapper，但提示词会优先要求模型输出官方 DSML 标签，并强调不能只输出 closing wrapper 而漏掉 opening tag。
正例中的工具名只会来自当前请求实际声明的工具；如果当前请求没有足够的已知工具形态，就省略对应的单工具、多工具或嵌套示例，避免把不可用工具名写进 prompt。
对执行类工具，脚本内容必须进入执行参数本身：`Bash` / `execute_command` 使用 `command`，`exec_command` 使用 `cmd`；不要把脚本示范成 `path` / `content` 文件写入参数。

OpenAI 路径实现：
[internal/promptcompat/tool_prompt.go](../internal/promptcompat/tool_prompt.go)

Claude 路径实现：
[internal/httpapi/claude/handler_utils.go](../internal/httpapi/claude/handler_utils.go)

统一工具调用格式模板：
[internal/toolcall/tool_prompt.go](../internal/toolcall/tool_prompt.go)

这也是项目“网页对话纯文本兼容”的关键设计：

- tools 对下游来说，本质上是 prompt 内规则
- 不是 native tool schema transport

## 7. assistant 的 tool_calls / reasoning 如何保留

### 7.1 reasoning 保留方式

assistant 的 reasoning 会变成一个显式标签块：

```text
[reasoning_content]
...
[/reasoning_content]
```

然后再接可见回答正文。

### 7.2 历史 tool_calls 保留方式

assistant 历史 `tool_calls` 不会保留成 OpenAI 原生 JSON，而会转成 prompt 可见的 DSML：

```xml
<|DSML|tool_calls>
  <|DSML|invoke name="read_file">
    <|DSML|parameter name="path" string="true">src/main.go</|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>
```

兼容层会优先向模型展示官方 DSML 形态；旧 XML `<tool_calls>` 只保留为兼容输入，不再作为历史 transcript 主格式继续喂回模型。
例外是 parser 仍会对一个非常窄的模型失误做修复：如果 assistant 输出了 `<invoke ...>` ... `</tool_calls>`，但漏掉最前面的 opening `<tool_calls>`，解析阶段会补回 wrapper 后再尝试识别。

这件事很重要，因为它决定了：

- 历史工具调用在 prompt 中是“可见文本历史”
- 不是“隐藏结构化元数据”

实现位置：
[internal/prompt/tool_calls.go](../internal/prompt/tool_calls.go)

### 7.3 tool result 保留方式

tool / function role 的结果会作为 `<｜Tool｜>...<｜end▁of▁toolresults｜>` 进入 prompt。

如果 tool content 为空，当前会补成字符串 `"null"`，避免整个 tool turn 丢失。

## 8. files、附件、systemprompt 文件的实际语义

这里要明确区分两类东西：

1. 文本型 system prompt
   例如 OpenAI `developer` / `system` / Responses `instructions` / Claude top-level `system`
   这类会进入 `prompt`。
2. 文件型 systemprompt
   例如通过附件、`input_file`、base64、data URL 上传的文件
   这类不会直接内联进 `prompt`，而是进入 `ref_file_ids`。

OpenAI 文件相关实现：

- inline/base64/data URL 上传：
  [internal/httpapi/openai/files/file_inline_upload.go](../internal/httpapi/openai/files/file_inline_upload.go)
- 文件 ID 收集：
  [internal/promptcompat/file_refs.go](../internal/promptcompat/file_refs.go)

结论：

- “systemprompt 文字”在 prompt 里
- “systemprompt 文件”通常只在 `ref_file_ids` 里

除非调用方自己把文件内容展开后再塞进 system/developer 文本，否则文件内容不会自动出现在 prompt 正文。

## 9. 多轮历史为什么不会一直完整内联在 prompt

history split 现在全局强制开启；旧配置中的 `history_split.enabled=false` 会被忽略。默认从第 2 个 user turn 起就可能触发，仍可通过 `history_split.trigger_after_turns` 调整触发阈值。

相关实现：

- 配置访问器：
  [internal/config/store_accessors.go](../internal/config/store_accessors.go)
- 历史拆分：
  [internal/httpapi/openai/history/history_split.go](../internal/httpapi/openai/history/history_split.go)

触发后行为：

1. 旧历史消息被切出去。
2. 旧历史会被重新序列化成一个文本文件。
3. 真正上传的文件名固定是 `HISTORY.txt`。
4. 文件内容内部会使用 `IGNORE` 这层包装名来闭合 DeepSeek 官网原生文件标记。
5. 该文件上传后，其 `file_id` 会排在 `ref_file_ids` 最前面。
6. live prompt 只保留：
   - system / developer
   - 最新 user turn 起的上下文

如果最新这一轮 `user` 还是纯文本且本身过长，兼容层还会再额外上传一个 `CURRENT_INPUT.txt`：

1. 该文件内容就是最后一轮 user 的完整原文。
2. live prompt 里的最后一轮 user 文本会被替换成一段简短提示，要求模型改为读取 `CURRENT_INPUT.txt`。
3. `ref_file_ids` 会同时包含：
   - `HISTORY.txt` 对应的 file id
   - `CURRENT_INPUT.txt` 对应的 file id
4. 这样 history split 不仅拆“过去的历史”，也能避免“当前这一轮超长正文”继续撑爆 live prompt。

历史文件内容不是普通自由文本，而是用同一套角色标记再次序列化出的 transcript：

```text
[uploaded filename]: HISTORY.txt
[file content end]

<｜begin▁of▁sentence｜><｜User｜>...<｜Assistant｜>...<｜Tool｜>...

[file name]: IGNORE
[file content begin]
```

所以“完整上下文”在当前实现里，其实通常分散在两处：

- `prompt` 里的 live context
- `ref_file_ids` 指向的 history transcript file

## 10. 各协议入口的差异

### 10.1 OpenAI Chat / Responses

特点：

- `developer` 会映射到 `system`
- Responses `instructions` 会 prepend 为 system message
- `tools` 会注入 system prompt
- `attachments` / `input_file` / inline 文件会进入 `ref_file_ids`
- history split 主要在这条链路里生效

### 10.2 Claude Messages

特点：

- top-level `system` 优先作为系统提示
- `tool_use` / `tool_result` 会被转换成统一的 assistant/tool 历史语义
- `tools` 同样会被并进 system prompt
- 常规执行通过 `internal/httpapi/claude/handler_messages.go` 转到 OpenAI chat 路径，模型 alias 会先解析成 DeepSeek 原生模型
- 当前代码里没有像 OpenAI 那样完整的 `ref_file_ids` 附件链路

### 10.3 Gemini

特点：

- `systemInstruction`、`contents.parts`、`functionCall`、`functionResponse` 会先归一
- tools 会转成 OpenAI 风格 function schema
- prompt 构建复用 OpenAI 的 `promptcompat.BuildOpenAIPromptForAdapter`
- 未识别的非文本 part 会被安全序列化进 prompt，并对二进制/疑似 base64 内容做省略或截断处理

也就是说，Gemini 在“最终 prompt 语义”上，尽量和 OpenAI 保持一致。

## 11. 一份贴近真实的最终上下文示意

假设用户发来一个多轮请求：

- 有 system/developer 文本
- 有 tools
- 有一个文件型 systemprompt 附件
- 有历史 assistant tool call / tool result
- history split 已触发

那么最终上下文更接近：

```json
{
  "prompt": "<｜begin▁of▁sentence｜><｜System｜>continuity instructions...\\n\\n原 system / developer\\n\\nYou have access to these tools: ...<｜end▁of▁instructions｜><｜User｜>最新问题<｜Assistant｜>",
  "ref_file_ids": [
    "file-history-ignore",
    "file-systemprompt",
    "file-other-attachment"
  ],
  "thinking_enabled": true,
  "search_enabled": false
}
```

这正是“API 转网页对话纯文本”的核心成果：

- 大部分结构化语义被压进 `prompt`
- 文件保持文件
- 历史必要时拆文件

## 12. 修改时必须同步本文档的场景

只要触碰以下任一类行为，就必须在同一提交或同一 PR 中更新本文档：

- 角色映射变更
- system / developer / instructions 合并规则变更
- assistant reasoning 保留格式变更
- assistant 历史 `tool_calls` 的 XML 呈现方式变更
- tool result 注入方式变更
- tool prompt 模板或 tool_choice 约束变更
- inline 文件上传 / 文件引用收集规则变更
- history split 触发条件、上传格式、`IGNORE` 包装格式变更
- completion payload 字段语义变更
- Claude / Gemini 对这套统一语义的复用关系变更

优先检查这些文件：

- `internal/promptcompat/request_normalize.go`
- `internal/promptcompat/prompt_build.go`
- `internal/promptcompat/message_normalize.go`
- `internal/promptcompat/tool_prompt.go`
- `internal/httpapi/openai/files/file_inline_upload.go`
- `internal/promptcompat/file_refs.go`
- `internal/httpapi/openai/history/history_split.go`
- `internal/promptcompat/responses_input_normalize.go`
- `internal/httpapi/claude/standard_request.go`
- `internal/httpapi/claude/handler_utils.go`
- `internal/httpapi/gemini/convert_request.go`
- `internal/httpapi/gemini/convert_messages.go`
- `internal/httpapi/gemini/convert_tools.go`
- `internal/prompt/messages.go`
- `internal/prompt/tool_calls.go`
- `internal/promptcompat/standard_request.go`

## 13. 建议的最小验证

改动这条链路后，至少补齐或检查这些测试：

- `go test ./internal/prompt/...`
- `go test ./internal/httpapi/openai/...`
- `go test ./internal/httpapi/claude/...`
- `go test ./internal/httpapi/gemini/...`
- `go test ./internal/util/...`

如果改的是 tool call 相关兼容语义，还应同时检查：

- `go test ./internal/toolcall/...`
- `node --test tests/node/stream-tool-sieve.test.js`

## 14. 文档同步约定

本文档是这条兼容链路的专项说明。

如果外部接口行为也变了，还应同步检查：

- [API.md](../API.md)
- [API.en.md](../API.en.md)
- [docs/toolcall-semantics.md](./toolcall-semantics.md)

原则是：

- 内部主链路变化，至少更新本文档
- 外部可见契约变化，再同步更新 API 文档
