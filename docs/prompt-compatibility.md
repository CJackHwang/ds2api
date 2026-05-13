# API -> 网页对话纯文本兼容主链路说明

文档导航：[总览](../README.MD) / [架构说明](./ARCHITECTURE.md) / [接口文档](../API.md) / [测试指南](./TESTING.md)

> 本文档是 DS2API“把 OpenAI / Claude / Gemini 风格 API 请求兼容成 DeepSeek 网页对话纯文本上下文”的专项说明。
> 这是项目最重要的兼容产物之一。凡是修改消息标准化、tool prompt 注入、tool history 保留、文件引用、current input file、下游 completion payload 组装等行为，都必须同步更新本文档。

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
  -> 文件收集 / inline 上传（OpenAI 文件链路）
  -> current input file（completion runtime 全局入口）
  -> completion payload
  -> 下游网页对话接口
  -> assistantturn 输出语义归一（Go 非流式 + 流式收尾）
  -> 各协议 renderer（OpenAI / Responses / Claude / Gemini）
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
- 最新 user 思考格式注入：
  [internal/promptcompat/thinking_injection.go](../internal/promptcompat/thinking_injection.go)
- completion payload：
  [internal/promptcompat/standard_request.go](../internal/promptcompat/standard_request.go)
- Go 输出侧 assistant turn：
  [internal/assistantturn/turn.go](../internal/assistantturn/turn.go)
- Go completion runtime：
  [internal/completionruntime/nonstream.go](../internal/completionruntime/nonstream.go)

## 4. 下游真正收到的东西

在“完成标准化后”，下游 completion payload 的核心形态是：

```json
{
  "chat_session_id": "session-id",
  "model_type": "default",
  "parent_message_id": null,
  "prompt": "<|begin▁of▁sentence|>...",
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
- 对外返回给客户端的 `prompt_tokens` / `input_tokens` / `promptTokenCount` 不再按“最后一条消息”或字符粗估近似返回，而是基于**完整上下文 prompt**做 tokenizer 计数；为了避免上下文实际超限但客户端误以为还能塞下，请求侧上下文 token 会额外保守上浮一点，宁可略大也不低估。
- 当前 `/v1/chat/completions` 业务路径仍是“每次请求新建一个远端 `chat_session_id`，并默认发送 `parent_message_id: null`”；因此 DS2API 对外默认表现为“新会话 + prompt 拼历史”，而不是复用 DeepSeek 原生会话树。
- 但 DeepSeek 远端本身支持同一 `chat_session_id` 的跨轮次持续对话。2026-04-27 已用项目内现有 DeepSeek client 做过一次不改业务代码的双轮实测：同一 `chat_session_id` 下，第 1 轮返回 `request_message_id=1` / `response_message_id=2` / 文本 `SESSION_TEST_ONE`；第 2 轮重新获取一次 PoW，并发送 `parent_message_id=2` 后，成功返回 `request_message_id=3` / `response_message_id=4` / 文本 `SESSION_TEST_TWO`。这说明“同远端会话持续聊天”能力存在，且每轮需要携带正确的 parent/message 链接信息，同时重新获取对应轮次可用的 PoW。
- OpenAI Chat / Responses 原生走统一 OpenAI 标准化与 DeepSeek payload 组装；Claude / Gemini 会尽量复用 OpenAI prompt/tool 语义，其中 Gemini 直接复用 `promptcompat.BuildOpenAIPromptForAdapter`。Go 主服务新增 `completionruntime` 启动层，统一执行 DeepSeek session/PoW/call；输出侧新增 `assistantturn` 语义层：非流式 OpenAI Chat / Responses / Claude / Gemini 会把 DeepSeek SSE 收集结果先归一成同一份 assistant turn，再分别渲染成各协议原生外形；流式 OpenAI Chat / Responses / Claude / Gemini 继续保持各协议实时 SSE framing，但最终收尾的 tool fallback、schema 归一、usage、empty-output / content-filter 错误语义同样由 `assistantturn` 判定。Claude / Gemini 的常规 Go 主路径不再依赖内部 `httptest` 转发到 OpenAI handler；`translatorcliproxy` 仅保留用于 Vercel bridge、后端缺失 fallback 和回归测试，不作为主业务协议转换中心。
- Vercel Node 流式路径本轮不迁移，仍使用现有 Node bridge / stream-tool-sieve 实现；后续若变更 Node 流式语义，需要按 `assistantturn` 的 Go canonical 输出语义同步对齐。
- 客户端传入的 thinking / reasoning 开关会被归一到下游 `thinking_enabled`。Gemini `generationConfig.thinkingConfig.thinkingBudget` 会翻译成同一套 thinking 开关；关闭时即使上游返回 `response/thinking_content`，兼容层也不会把它当作可见正文输出。若最终解析出的模型名带 `-nothinking` 后缀，则会无条件强制关闭 thinking，优先级高于请求体中的 `thinking` / `reasoning` / `reasoning_effort`。未显式关闭时，各 surface 会按解析后的 DeepSeek 模型默认能力开启 thinking，并用各自协议的原生形态暴露：OpenAI Chat 为 `reasoning_content`，OpenAI Responses 为 `response.reasoning.delta` / `reasoning` content，Claude 为 `thinking` block / `thinking_delta`，Gemini 为 `thought: true` part。
- 对 OpenAI Chat / Responses 的非流式收尾，如果最终可见正文为空，兼容层会优先尝试把思维链中的独立 DSML / XML 工具块当作真实工具调用解析出来。流式链路也会在收尾阶段做同样的 fallback 检测，但不会因为思维链内容去中途拦截或改写流式输出；真正的工具识别始终基于原始上游文本，而不是基于“已经做过可见输出清洗”的版本。最终可见层会剥离已经成功解析成工具调用的完整 leaked DSML / XML `tool_calls` wrapper；如果遇到完整 wrapper 但内部形态不符合可执行工具调用语义（例如 `<param>` 这类 malformed XML 工具壳），流式 sieve 会把该块作为普通文本释放，而不是吞掉或伪造成工具调用。补发结果会作为本轮 assistant 的结构化 `tool_calls` / `function_call` 输出返回，而不是塞进 `content` 文本；如果客户端没有开启 thinking / reasoning，思维链只用于检测，不会作为 `reasoning_content` 或可见正文暴露。只有正文为空且思维链里也没有可执行工具调用时，才继续按空回复错误处理。
- OpenAI Chat / Responses、Claude Messages、Gemini generateContent 的空回复错误处理之前会默认做一次内部补偿重试：第一次上游完整结束后，如果最终可见正文为空、没有解析到工具调用、也没有已经向客户端流式发出工具调用，并且终止原因不是 `content_filter`，兼容层会复用同一个 `chat_session_id`、账号、token 与工具策略，把原始 completion `prompt` 追加固定后缀 `Previous reply had no visible output. Please regenerate the visible final answer or tool call now.` 后重新提交一次。Go 主路径的非流式重试由 `completionruntime.ExecuteNonStreamWithRetry` 统一处理；流式重试由 `completionruntime.ExecuteStreamWithRetry` 统一处理，各协议 runtime 只负责消费/渲染本协议 SSE framing。重试遵循 DeepSeek 多轮对话协议：从第一次上游 SSE 流中提取 `response_message_id`，并在重试 payload 中设置 `parent_message_id` 为该值，使重试成为同一会话的后续轮次而非断裂的根消息；同时重新获取一次 PoW（若 PoW 获取失败则回退到原始 PoW）。该同账号重试不会重新标准化消息、不会新建 session，也不会向流式客户端插入重试标记；第二次 thinking / reasoning 会按正常增量直接接到第一次之后，并继续使用 overlap trim 去重。若同账号补偿重试后即将返回 429 `upstream_empty_output`，并且当前是托管账号模式，runtime 会在返回 429 前切换到下一个可用账号，新建 `chat_session_id`，使用原始 completion payload 再做一次 fresh retry；该切号重试不携带空回复 prompt 后缀，也不设置上一账号的 `parent_message_id`。如果 current input file 已触发，切号前会在新账号上重新上传同一份 `DS2API_HISTORY.txt`（以及需要时的 `DS2API_TOOLS.txt`），并用新账号可见的 file_id 替换自动生成的旧 file_id；客户端原本传入的其他文件引用保持不变。如果没有可切换账号，或切号后的 fresh retry 仍没有可见正文或工具调用，则继续按原错误返回：无任何输出为 503 `upstream_unavailable`，有 reasoning 但没有可见正文或工具调用为 429 `upstream_empty_output`。若任一尝试触发空 `content_filter`，不做补偿重试并保持 `content_filter` 错误。Vercel Node 流式路径通过 Go 内部 prepare / pow / switch 端点获取初始 payload、重试 PoW 和切号 fresh retry payload，因此同样会重新上传 current-input 自动文件并替换为新账号 file_id。

- 非流式 OpenAI Chat / Responses、Claude Messages、Gemini generateContent 在最终可见正文渲染阶段，会把 DeepSeek 搜索返回中的 `[citation:N]` / `[reference:N]` 标记替换成对应 Markdown 链接。`citation` 标记按一基序号解析；`reference` 标记只有在同一段正文中出现 `[reference:0]`（允许冒号后有空格）时才按零基序号映射，并且不会影响同段正文里的 `citation` 标记。
- 流式输出仍默认隐藏 `[citation:N]` / `[reference:N]` 这类上游内部标记，避免分片输出中泄漏尚未完成映射的引用占位符。

## 5. prompt 是怎么拼出来的

OpenAI Chat / Responses 在标准化后、current input file 之前，会默认执行 `thinking_injection` 增强。它参考 DeepSeek V4 “把控制指令放在 user 消息末尾更稳定”的用法，在最新 user message 后追加思考增强提示词。当前内置默认提示词以 `Reasoning Effort: Absolute maximum with no shortcuts permitted.` 开头，并继续要求模型充分分解问题、覆盖潜在路径与边界条件、把完整推演过程显式写出。该开关默认启用，可通过 `thinking_injection.enabled=false` 关闭；也可以通过 `thinking_injection.prompt` 自定义提示词，留空时使用内置默认提示词。

这段增强属于 prompt 可见上下文：

- 普通请求会直接出现在最终 `prompt` 的最新 user block 末尾。
- 如果触发 current input file，它会进入完整上下文文件中。

另外，`MessagesPrepareWithThinking` 还会在最终 prompt 的最前面预置一段固定的 system 级“输出完整性约束（Output integrity guard）”：

- 如果上游上下文、工具输出或解析后的文本出现乱码、损坏、部分解析、重复或其他畸形片段，不要模仿、不要回显，只输出给用户的正确内容。
- 这段约束位于普通 system / tool prompt 之前，因此是当前最终 prompt 里的最高优先级前置指令。

### 5.1 角色标记

最终 prompt 使用 DeepSeek 风格角色标记：

- `<|begin▁of▁sentence|>`
- `<|System|>`
- `<|User|>`
- `<|Assistant|>`
- `<|Tool|>`
- `<|end▁of▁instructions|>`
- `<|end▁of▁sentence|>`
- `<|end▁of▁toolresults|>`

实现位置：
[internal/prompt/messages.go](../internal/prompt/messages.go)

### 5.2 相邻同角色消息会合并

在最终 `MessagesPrepareWithThinking` 中，相邻同 role 的消息会被合并成一个块，中间插入空行。

这意味着：

- prompt 中看到的是“合并后的 role block”
- 不是客户端传来的逐条 message 原样排列

## 6. tools 为什么是“文本注入”，不是原生下发

当前项目把工具能力视为“prompt 约束的一部分”。

具体做法：

1. 把每个 tool 的名称、描述、参数 schema 序列化成文本。
2. 拼成 `You have access to these tools:` 大段说明。
3. 再附上统一的 DSML tool call 外壳格式约束。
4. 普通直传请求会把“工具描述 + 格式约束”一起并入 system prompt；如果 `current_input_file` 触发，则工具描述/schema 会单独上传成 `DS2API_TOOLS.txt`，live prompt 和 system tool 格式提示都会明确要求模型把 `DS2API_TOOLS.txt` 当作可调用工具和参数 schema 的权威来源。

工具调用正例现在优先示范半角管道符 DSML 风格：`<|DSML|tool_calls>` → `<|DSML|invoke name="...">` → `<|DSML|parameter name="...">`。
兼容层仍接受旧式纯 `<tool_calls>` wrapper，并会容错若干 DSML 标签变体，包括短横线形式 `<dsml-tool-calls>` / `<dsml-invoke>` / `<dsml-parameter>`、下划线形式 `<dsml_tool_calls>` / `<dsml_invoke>` / `<dsml_parameter>`，以及其他前缀分隔形态如 `<vendor|tool_calls>` / `<vendor_tool_calls>` / `<vendor - tool_calls>`；标签壳扫描还会把全角 ASCII 漂移归一化，例如 `<ｄＳＭＬ|tool_calls>` 与全角 `＞` 结束符，也会容错 CJK 尖括号、全角感叹号或顿号分隔符、弯引号属性值、PascalCase 本地名和属性尾部分隔符漂移，例如 `<DSM|parameter name="command"|>...〈/DSM|parameter〉`、`<！DSML！invoke name=“Bash”>`、`<、DSML、tool_calls>`、`<DSmartToolCalls>`、`<DSMLtool_calls※>`。更一般地，Go / Node tag 扫描以固定本地标签名 `tool_calls` / `invoke` / `parameter` 为准，标签名前或标签名后的非结构性协议分隔符都会在解析入口剥离，例如 `<DSML␂tool_calls>`、`<proto💥tool_calls>` 这类控制符或非 ASCII 分隔符漂移也会归一化回现有 XML 标签后继续走同一套 parser；结构性字符如 `<` / `>` / `/` / `=` / 引号、空白和 ASCII 字母数字不会被当作这类分隔符。进入现有 DSML rewrite / XML parse 之前，Go / Node 还会先对“已经识别成工具标签壳的 candidate span”做一次窄 canonicalization：只折叠 wrapper / `invoke` / `parameter` / `name` / `CDATA` / `DSML` 及其壳层分隔符里的 confusable 字符，清理零宽 / BOM / 控制类干扰，并把引号、空白、dash / underscore 变体等统一回可解析的工具语法。这个阶段不会广义改写普通正文、参数内容、Markdown 行内 code span、CDATA 里的示例文本或其他非工具 XML。CDATA 开头也使用同一类扫描式容错，`<![CDATA[` / `<！[CDATA[` / `<、[CDATA[` 都会作为参数原文容器处理。但提示词会优先要求模型输出官方 DSML 标签，并强调不能只输出 closing wrapper 而漏掉 opening tag。需要注意：这是“兼容 DSML 外壳，内部仍以 XML 解析语义为准”，不是原生 DSML 全链路实现。解析器会先截获非 Markdown 代码上下文中的疑似工具 wrapper，完整解析失败或工具语义无效时再按普通文本放行。
数组参数使用 `<item>...</item>` 子节点表示；当某个参数体只包含 item 子节点时，Go / Node 解析器会把它还原成数组，避免 `questions` / `options` 这类 schema 中要求 array 的参数被误解析成 `{ "item": ... }` 对象。除此之外，解析器还会回收一些更松散的列表写法，例如 JSON array 字面量或逗号分隔的 JSON 项序列，只要它们足够明确；但 `<item>` 仍然是首选形态。若模型把完整结构化 XML fragment 误包进 CDATA，兼容层会在保护 `content` / `command` 等原文字段的前提下，尝试把非原文字段中的 CDATA XML fragment 还原成 object / array。不过，如果 CDATA 只是单个平面的 XML/HTML 标签，例如 `<b>urgent</b>` 这种行内标记，兼容层会保留原始字符串，不会强行升成 object / array；只有明显表示结构的 CDATA 片段，例如多兄弟节点、嵌套子节点或 `item` 列表，才会触发结构化恢复。对 `command` / `content` 等长文本参数，CDATA 内部的 Markdown fenced DSML / XML 示例会作为原文保护；示例里的 `]]></parameter>` 或 `</tool_calls>` 不会截断外层工具调用，解析器会继续等待围栏外真正的参数 / wrapper 结束标签。
Go 侧读取 DeepSeek SSE 时不再依赖 `bufio.Scanner` 的固定 2MiB 单行上限；当写文件类工具把很长的 `content` 放在单个 `data:` 行里返回时，非流式收集、流式解析和 auto-continue 透传都会保留完整行，再进入同一套工具解析与序列化流程。
在 assistant 最终回包阶段，如果某个 tool 参数在声明 schema 中明确是 `string`，兼容层会在把解析后的 `tool_calls` / `function_call` 重新序列化成 OpenAI / Responses / Claude 可见参数前，递归把该路径上的 number / bool / object / array 统一转成字符串；其中 object / array 会压成紧凑 JSON 字符串。这个保护只对 schema 明确声明为 string 的路径生效，不会改写本来就是 `number` / `boolean` / `object` / `array` 的参数。这样可以兼容 DeepSeek 输出了结构化片段、但上游客户端工具 schema 又严格要求字符串参数的场景（例如 `content`、`prompt`、`path`、`taskId` 等）。
工具 schema 的权威来源始终是**当前请求实际携带的 schema**，而不是同名工具在其他 runtime（Claude Code / OpenCode / Codex 等）里的默认印象。兼容层现在会同时兼容 OpenAI 风格 `function.parameters`、直接工具对象上的 `parameters` / `input_schema`、以及 camelCase 的 `inputSchema` / `schema`，并在最终输出阶段按这份请求内 schema 决定是保留 array/object，还是仅对明确声明为 `string` 的路径做字符串化。该规则同样适用于 Claude 的流式收尾和 Vercel Node 流式 tool-call formatter，避免不同 runtime 因 schema shape 差异而出现同名工具参数类型漂移。
正例中的工具名只会来自当前请求实际声明的工具；如果当前请求没有足够的已知工具形态，就省略对应的单工具、多工具或嵌套示例，避免把不可用工具名写进 prompt。
对执行类工具，脚本内容必须进入执行参数本身：`Bash` / `execute_command` 使用 `command`，`exec_command` 使用 `cmd`；不要把脚本示范成 `path` / `content` 文件写入参数。
工具提示词也会明确要求模型按本次调用实际需要填写参数，禁止输出 placeholder、空字符串或纯空白参数；如果必填参数未知，应先追问用户或正常文字回复，而不是输出空工具壳。对 `Bash` / `execute_command` 这类 shell 工具，命令或脚本必须写入 `command` 参数。解析层仍会把空字符串参数结构化返回；是否拒绝空 `command` 由后续工具执行侧 / 客户端 schema 校验决定。
如果当前请求声明了 `Read` / `read_file` 这类读取工具，兼容层会额外注入一条 read-tool cache guard：当读取结果只表示“文件未变更 / 已在历史中 / 请引用先前上下文 / 没有正文内容”时，模型必须把它视为内容不可用，不能反复调用同一个无正文读取；应改为请求完整正文读取能力，或向用户说明需要重新提供文件内容。这个约束只缓解客户端缓存返回空内容导致的死循环，DS2API 不会也无法凭空恢复客户端本地文件正文。

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

对最终返回给客户端的 assistant 轮次，reasoning 不会因为本轮输出了工具调用而被丢弃。OpenAI Chat 会在同一个 assistant message 上同时返回 `reasoning_content` 和 `tool_calls`；OpenAI Responses 会先返回一个包含 `reasoning` content 的 assistant message item，再返回后续 `function_call` item；Claude / Gemini 也会在各自原生 thinking / thought 结构后继续返回 tool_use / functionCall。

对进入后续 prompt / `DS2API_HISTORY.txt` 的历史轮次，兼容层也会把同一轮工具调用前的 reasoning 绑定到 assistant tool call 历史上。OpenAI Chat 原生 `reasoning_content + tool_calls` 会直接保留；OpenAI Responses 若以 `reasoning` message item 后接 `function_call` item 的形式回放历史，会在归一化时合并为同一个 assistant 历史块；Claude 的 `thinking` block 会绑定到后续 `tool_use`；Gemini 的 `thought: true` part 会绑定到后续 `functionCall`。最终 prompt 中的顺序固定为 `[reasoning_content]...[/reasoning_content]`，再接 DSML tool call 外壳。

### 7.2 历史 tool_calls 保留方式

assistant 历史 `tool_calls` 不会保留成 OpenAI 原生 JSON，而会转成 prompt 可见的 DSML 外壳：

```xml
<|DSML|tool_calls>
  <|DSML|invoke name="read_file">
    <|DSML|parameter name="path"><![CDATA[src/main.go]]></|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>
```

如果客户端历史里没有结构化 `tool_calls` 字段、却把一个可独立解析的 assistant 工具块放进了普通 `content`，兼容层会在写入后续 prompt 前先按工具调用解析它，再重渲染为规范 DSML 历史外壳。这样可以避免一次 malformed 工具块未被结构化保存后，作为普通 assistant 文本回灌，继续污染后续模型的 few-shot 工具格式。

解析层同时兼容旧式纯 XML 形态：`<tool_calls>` / `<invoke>` / `<parameter>`。两者都会先归一到现有 XML 解析语义；其他旧格式都会作为普通文本保留，不会作为可执行调用语法。
例外是 parser 会对一个非常窄的模型失误做修复：如果 assistant 输出了 `<invoke ...>` ... `</tool_calls>`（或 DSML 对应标签），但漏掉最前面的 opening wrapper，解析阶段会在 wrapper-confidence 足够高时补回 wrapper 后再尝试识别。这里的 wrapper-confidence 指 scanner 已经识别出白名单工具壳结构，剩余失败只像壳层结构漂移，而不是语义上接近但不在白名单内的 near-miss 标签名。修复成功时，wrapper 后面的 suffix prose 会继续保留在可见文本里；修复失败时，该块仍按普通文本处理。

这件事很重要，因为它决定了：

- 历史工具调用在 prompt 中是“可见文本历史”
- 不是“隐藏结构化元数据”

实现位置：
[internal/prompt/tool_calls.go](../internal/prompt/tool_calls.go)

### 7.3 tool result 保留方式

tool / function role 的结果会作为 `<|Tool|>...<|end▁of▁toolresults|>` 进入 prompt。

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

OpenAI 的文件上传现在不再是“只传文件本体”的通用路径，而是会先根据请求里的 `model` 解析出 DeepSeek 的上传类型，并把它透传到上传接口的 `x-model-type`。当前可见的上传类型就是 `default` / `expert` / `vision`，其中 vision 请求上传图片时必须带上 `vision`，否则下游容易退回到仅文本或 OCR 语义。这个模型类型会同时用于：

- `/v1/files` 这类独立文件上传入口
- Chat / Responses 的 inline 图片、附件上传
- current input file 触发时生成的 `DS2API_HISTORY.txt` 上下文文件

也就是说，文件上传和完成请求的 `model_type` 现在是一致的：完成 payload 里仍然是 `model_type`，上传文件则会在 DeepSeek 上传阶段携带同样的模型类型信息。

结论：

- “systemprompt 文字”在 prompt 里
- “systemprompt 文件”通常只在 `ref_file_ids` 里

除非调用方自己把文件内容展开后再塞进 system/developer 文本，否则文件内容不会自动出现在 prompt 正文。

## 9. 多轮历史为什么不会一直完整内联在 prompt

兼容层现在只保留 `current_input_file` 这一种拆分方式；旧的 `history_split` 配置字段已移除，读取旧配置时会忽略它且不会再写回。

- `current_input_file` 默认开启；它在统一 completion runtime 入口全局生效，用于把“完整上下文”合并进 `DS2API_HISTORY.txt` 上下文文件。当最新 user turn 的纯文本长度达到 `current_input_file.min_chars`（默认 `0`）时，runtime 会上传一个文件名为 `DS2API_HISTORY.txt` 的上下文文件。文件内容会先经过各协议入口的标准化，再序列化成按轮次编号的 `DS2API_HISTORY.txt` 风格 transcript，带有 `# DS2API_HISTORY.txt` 标题和 `=== N. ROLE ===` 分段；如果当前请求声明了可用工具，还会把工具名称、描述和参数 schema 单独上传成 `DS2API_TOOLS.txt`，带有 `# DS2API_TOOLS.txt` 标题。live prompt 中则会给出一个 continuation 语气的 user 消息，引导模型从 `DS2API_HISTORY.txt` 的最新状态继续推进，并在有工具文件时明确可用工具 schema 位于 `DS2API_TOOLS.txt`；system prompt 也会在统一 DSML 工具格式约束前说明 `DS2API_TOOLS.txt` 是可调用工具和 schema 的权威来源，同时保留本轮工具选择策略，避免把任务拉回起点。
- 如果 `current_input_file.enabled=false`，请求会直接透传，不上传任何拆分上下文文件。
- 即使触发 `current_input_file` 后 live prompt 被缩短，对客户端回包里的上下文 token 统计，仍会沿用**拆分前的完整 prompt 语义**做计数，而不是按缩短后的占位 prompt 计算；否则会把真实上下文显著算小。

相关实现：

- 配置访问器：
  [internal/config/store_accessors.go](../internal/config/store_accessors.go)
- 当前输入转文件：
  [internal/httpapi/openai/history/current_input_file.go](../internal/httpapi/openai/history/current_input_file.go)
- 全局 completion runtime 应用点：
  [internal/completionruntime/nonstream.go](../internal/completionruntime/nonstream.go)

当前输入转文件启用并触发时，上传的历史文件真实文件名是 `DS2API_HISTORY.txt`，文件内容是完整 `messages` 上下文；它会使用 OpenAI-compatible 的消息/transcript 序列化规则和 DeepSeek 角色标记，再按轮次编号成 `DS2API_HISTORY.txt` 风格的 transcript（不再注入文件边界标签）：

```text
[uploaded filename]: DS2API_HISTORY.txt
# DS2API_HISTORY.txt
Prior conversation history and tool progress.

=== 1. SYSTEM ===
...

=== 2. USER ===
...

=== 3. ASSISTANT ===
...

=== 4. TOOL ===
...
```

如果当前请求带有工具，runtime 同时上传 `DS2API_TOOLS.txt`：

```text
[uploaded filename]: DS2API_TOOLS.txt
# DS2API_TOOLS.txt
Available tool descriptions and parameter schemas for this request.

You have access to these tools:

Tool: ...
Description: ...
Parameters: ...
```

开启后，请求的 live prompt 不再直接内联完整上下文，也不再内联大段工具 schema；它保留一个 user role 的短提示，提示模型基于已提供上下文直接回答最新请求，并在有工具时引用 `DS2API_TOOLS.txt`。上传后的 `DS2API_HISTORY.txt` file_id 会排在 `ref_file_ids` 最前；如果存在 `DS2API_TOOLS.txt`，它的 file_id 紧随其后；客户端已有的其他 file_id 保持在后面。上下文 token 统计会包含上传的历史文件、工具文件和 live prompt。自动生成的 current-input 文件引用会被记录为 runtime 状态；如果托管账号模式切号 fresh retry，runtime 会重新上传这些自动文件，而不是把上一账号的 file_id 交给新账号。

## 10. 各协议入口的差异

### 10.1 OpenAI Chat / Responses

特点：

- `developer` 会映射到 `system`
- Responses `instructions` 会 prepend 为 system message
- 普通直传时 `tools` 会注入 system prompt；`current_input_file` 触发时工具描述/schema 会拆成 `DS2API_TOOLS.txt`，system prompt 保留格式/策略规则并明确要求模型从 `DS2API_TOOLS.txt` 获取可调用工具和 schema
- `attachments` / `input_file` / inline 文件会进入 `ref_file_ids`
- current input file 在统一 completion runtime 入口全局生效

### 10.2 Claude Messages

特点：

- top-level `system` 优先作为系统提示
- `tool_use` / `tool_result` 会被转换成统一的 assistant/tool 历史语义
- 普通直传时 `tools` 同样会被并进 system prompt；`current_input_file` 触发时会沿用统一的 `DS2API_TOOLS.txt` 拆分上传路径
- 常规执行通过 `internal/httpapi/claude/handler_messages.go` 转到 OpenAI chat 路径，模型 alias 会先解析成 DeepSeek 原生模型
- 当前代码里没有像 OpenAI 那样完整的 `ref_file_ids` 附件链路

### 10.3 Gemini

特点：

- `systemInstruction`、`contents.parts`、`functionCall`、`functionResponse` 会先归一
- tools 会转成 OpenAI 风格 function schema
- prompt 构建复用 OpenAI 的 `promptcompat.BuildOpenAIPromptForAdapter`，`current_input_file` 触发时也会使用统一的 `DS2API_TOOLS.txt` 拆分上传路径
- 未识别的非文本 part 会被安全序列化进 prompt，并对二进制/疑似 base64 内容做省略或截断处理

也就是说，Gemini 在“最终 prompt 语义”上，尽量和 OpenAI 保持一致。

## 11. 一份贴近真实的最终上下文示意

假设用户发来一个多轮请求：

- 有 system/developer 文本
- 有 tools
- 有一个文件型 systemprompt 附件
- 有历史 assistant tool call / tool result
- current input file 已触发

那么最终上下文更接近：

```json
{
  "prompt": "<|begin▁of▁sentence|><|System|>原 system / developer\n\nTOOL CALL FORMAT — FOLLOW EXACTLY: ...<|end▁of▁instructions|><|User|>Continue from the latest state in the attached DS2API_HISTORY.txt context. Treat it as the current working state and answer the latest user request directly. Available tool descriptions and parameter schemas are attached in DS2API_TOOLS.txt; use only those tools and follow the tool-call format rules in this prompt.<|Assistant|>",
  "ref_file_ids": [
    "file-ds2api-history",
    "file-ds2api-tools",
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
- 需要时把完整上下文拆进 `DS2API_HISTORY.txt` 上下文文件，并按轮次编号成 transcript

## 12. 修改时必须同步本文档的场景

只要触碰以下任一类行为，就必须在同一提交或同一 PR 中更新本文档：

- 角色映射变更
- system / developer / instructions 合并规则变更
- assistant reasoning 保留格式变更
- assistant 历史 `tool_calls` 的 XML 呈现方式变更
- tool result 注入方式变更
- tool prompt 模板或 tool_choice 约束变更
- inline 文件上传 / 文件引用收集规则变更
- current input file 触发条件、上传格式、`DS2API_HISTORY.txt` transcript 结构变更
- 旧 `history_split` 字段忽略/清理行为变更
- completion payload 字段语义变更
- Claude / Gemini 对这套统一语义的复用关系变更

优先检查这些文件：

- `internal/promptcompat/request_normalize.go`
- `internal/promptcompat/prompt_build.go`
- `internal/promptcompat/message_normalize.go`
- `internal/promptcompat/tool_prompt.go`
- `internal/httpapi/openai/files/file_inline_upload.go`
- `internal/promptcompat/file_refs.go`
- `internal/httpapi/openai/history/current_input_file.go`
- `internal/completionruntime/nonstream.go`
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
- `go test ./internal/toolstream/...`
- `./tests/scripts/run-unit-node.sh`

## 14. 文档同步约定

本文档是这条兼容链路的专项说明。

如果外部接口行为也变了，还应同步检查：

- [API.md](../API.md)
- [API.en.md](../API.en.md)
- [docs/toolcall-semantics.md](./toolcall-semantics.md)

原则是：

- 内部主链路变化，至少更新本文档
- 外部可见契约变化，再同步更新 API 文档
