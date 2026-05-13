# Tool call parsing semantics（Go/Node 统一语义）

本文档描述当前代码中的**实际行为**，以 `internal/toolcall`、`internal/toolstream` 与 `internal/js/helpers/stream-tool-sieve` 为准。

文档导航：[总览](../README.MD) / [架构说明](./ARCHITECTURE.md) / [测试指南](./TESTING.md)

## 1) 当前可执行格式

当前版本推荐模型输出半角管道符 DSML 外壳：

```xml
<|DSML|tool_calls>
  <|DSML|invoke name="read_file">
    <|DSML|parameter name="path"><![CDATA[README.MD]]></|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>
```

兼容层仍接受旧式 canonical XML：

```xml
<tool_calls>
  <invoke name="read_file">
    <parameter name="path"><![CDATA[README.MD]]></parameter>
  </invoke>
</tool_calls>
```

这不是原生 DSML 全链路实现。DSML 主要用于让模型有意识地输出协议标识，隔离普通 XML 语义；进入 parser 前会按固定本地标签名归一化成 `<tool_calls>` / `<invoke>` / `<parameter>`，内部仍以现有 XML 解析语义为准。

约束：

- 必须有 `<|DSML|tool_calls>...</|DSML|tool_calls>` 或 `<tool_calls>...</tool_calls>` wrapper
- 每个调用必须在 `<|DSML|invoke name="...">...</|DSML|invoke>` 或 `<invoke name="...">...</invoke>` 内
- 工具名必须放在 `invoke` 的 `name` 属性
- 参数必须使用 `<|DSML|parameter name="...">...</|DSML|parameter>` 或 `<parameter name="...">...</parameter>`
- 同一个工具块内不要混用 DSML 标签和旧 XML 工具标签；混搭会被视为非法工具块

兼容修复：

- 如果模型漏掉 opening wrapper，但后面仍输出了一个或多个 invoke 并以 closing wrapper 收尾，Go 解析链路会在解析前补回缺失的 opening wrapper。
- 在进入现有 DSML rewrite / XML parse 之前，Go / Node 都会先做一次非常窄的 candidate-span canonicalization：只处理已经被 scanner 识别为工具标签壳的 wrapper / `invoke` / `parameter` / `name` / `CDATA` / `DSML` 及其结构分隔符；这里会移除零宽 / BOM / 控制类干扰字符，并把 `<`、`>`、`/`、`|`、`=`、引号、Unicode 空白、常见 dash / underscore 变体这类工具语法外壳符号折回 ASCII 语义。
- Go / Node 解析层不再枚举每一种 DSML typo。它以固定本地标签名 `tool_calls` / `invoke` / `parameter` 为准，把标签名前的任意协议前缀壳视为可容忍噪声，并继续兼容半角管道符、全角感叹号 `！`、顿号 `、`、空白、重复 leading `<`、可视控制符 `␂`、原始 STX `\x02`、非 ASCII 分隔符、CJK 尖括号 `〈` / `〉`、弯引号属性值、PascalCase 本地名等漂移。例如 `<DSML|tool_calls>`、`<<|DSML|tool_calls>`、`<|DSML tool_calls>`、`<DSMLtool_calls>`、`<DSmartToolCalls>`、`<<DSML|DSML|tool_calls>`、`<DSML␂tool_calls>`、`<proto💥tool_calls>`、`<DSM|tool_calls>...〈/DSM|tool_calls〉`、`<！DSML！tool_calls>...<！/DSML！tool_calls>`、`<、DSML、tool_calls>...<、/DSML、tool_calls>` 都会归一化；相似但非固定标签名（如 `tool_calls_extra` / `ToolCallsExtra`）仍按普通文本处理。
- 这个 candidate-span canonicalization 不会对普通 prose、参数正文、CDATA 内容或嵌套的非工具 XML 做广义 Unicode 归一化。也就是说，参数里的示例 `<invοke>`、普通聊天文本里的 confusable 单词、或其他非工具壳 XML 片段都保持原样；只有真正落在工具标签壳上的 whitelist 关键字和结构符号会被折叠。
- 如果模型在固定工具标签名后多输出一个非结构性分隔符，例如 `<|DSML|tool_calls|` / `<|DSML|invoke|` / `<|DSML|parameter|` / `<DSMLtool_calls※>`，或在带属性标签的结束符前多输出一个尾部分隔符（如 `<DSM|parameter name="command"|>`），兼容层会把这个尾部分隔符当作异常标签终止符并补齐或归一化；如果后面已经有 `>` / `〉`，也会消费这个多余分隔符后再归一化。结构性字符如 `<` / `>` / `/` / `=` / 引号、空白和 ASCII 字母数字不会被当作这类分隔符。
- “缺失 opening wrapper”的修复只会在 wrapper-confidence 足够高时触发：scanner 必须已经识别出白名单工具壳结构（wrapper / invoke / parameter / `name=` 等），且剩余失败看起来只是壳层结构问题。相似但不在白名单内的 near-miss 标签名，或缺少足够 wrapper 证据的 malformed 片段，仍会按普通文本透传。
- 这是一个针对常见模型失误的窄修复，不改变推荐输出格式；prompt 仍要求模型直接输出完整 DSML 外壳。
- 裸 `<invoke ...>` / `<parameter ...>` 不会被当成“已支持的工具语法”；只有 `tool_calls` wrapper 或可修复的缺失 opening wrapper 才会进入工具调用路径。

## 2) 非兼容内容

任何不满足上述 DSML / canonical XML 形态的内容，都会保留为普通文本，不会执行。一个例外是上一节提到的“缺失 opening wrapper、但 closing wrapper 仍存在”的窄修复场景。

当前 parser 不把 allow-list 当作硬安全边界：即使传入了已声明工具名列表，XML 里出现未声明工具名时也会尽量解析并交给上层协议输出；真正的执行侧仍必须自行校验工具名和参数。

## 3) 流式与防泄漏行为

在流式链路中（Go / Node 一致）：

- DSML `<|DSML|tool_calls>` wrapper、短横线形式（如 `<dsml-tool-calls>` / `<dsml-invoke>` / `<dsml-parameter>`）、基于固定本地标签名的 DSML 噪声容错形态、尾部非结构性分隔符形态（如 `<|DSML|tool_calls|` / `<DSMLtool_calls※>`）和 canonical `<tool_calls>` wrapper 都会进入结构化捕获
- 如果流里直接从 invoke 开始，但后面补上了 closing wrapper，Go 流式筛分也会按缺失 opening wrapper 的修复路径尝试恢复
- 已识别成功的工具调用不会再次回流到普通文本
- 不符合新格式的块不会执行，并继续按原样文本透传
- 如果一个 confusable / 漂移过的工具壳在 candidate-span canonicalization + repair 后仍能形成有效工具调用，wrapper 后面的 suffix prose 会继续按普通文本输出；如果 canonicalization 后仍不满足 wrapper-confidence 或 XML 语义，整块就作为普通文本释放，不会半吞半漏。
- fenced code block（反引号 `` ``` `` 和波浪线 `~~~`）以及 Markdown inline code span（例如 `` `<tool_calls>...</tool_calls>` ``）中的 XML 示例始终按普通文本处理
- 支持嵌套围栏（如 4 反引号嵌套 3 反引号）和 CDATA 内围栏保护
- 对 `command` / `content` 等长文本参数，CDATA 内部如果包含 Markdown fenced DSML / XML 示例，即使示例里出现 `]]></parameter>` / `</tool_calls>` 这类看起来像外层结束标签的片段，也会继续按参数原文保留，直到真正位于围栏外的外层结束标签
- CDATA 开头也按扫描式识别，除了标准 `<![CDATA[`，还会接受 `<！[CDATA[`、`<、[CDATA[` 这类分隔符漂移，并统一还原为原文字段内容。
- 如果模型把 `<![CDATA[` 打开后却没有闭合，流式扫描阶段仍会保守地继续缓冲，不会误把 CDATA 里的示例 XML 当成真实工具调用；在最终 parse / flush 恢复阶段，会对这类 loose CDATA 做窄修复，尽量保住外层已完整包裹的真实工具调用
- 当文本中 mention 了某种标签名（如 `<dsml|tool_calls>` 或 Markdown inline code 里的 `<|DSML|tool_calls>`）而后面紧跟真正工具调用时，sieve 会跳过不可解析的 mention 候选并继续匹配后续真实工具块；行内 code span 中即使出现完整 `<tool_calls>...</tool_calls>` 示例也不会执行，不会因 mention 导致工具调用丢失，也不会截断 mention 后的正文
- Go 侧 SSE 读取不再使用 `bufio.Scanner` 的固定 token 上限；单个 `data:` 行中包含很长的写文件参数时，非流式收集、流式解析与 auto-continue 透传都应保留完整行，再交给 tool parser 处理

另外，`<parameter>` 的值如果本身是合法 JSON 字面量，也会按结构化值解析，而不是一律保留为字符串。例如 `123`、`true`、`null`、`[1,2]`、`{"a":1}` 都会还原成对应的 number / boolean / null / array / object。
结构化 XML 参数也会还原为 JSON 结构：如果参数体只包含一个或多个 `<item>...</item>` 子节点，会输出数组；嵌套对象里的 item-only 字段也同样按数组处理。例如 `<parameter name="questions"><item><question>...</question></item></parameter>` 会输出 `{"questions":[{"question":"..."}]}`，而不是 `{"questions":{"item":...}}`。
如果模型误把完整结构化 XML fragment 放进 CDATA，Go / Node 会先保护明显的原文字段（如 `content` / `command` / `prompt` / `old_string` / `new_string`），其余参数会尝试把 CDATA 内的完整 XML fragment 还原成 object / array；常见的 `<br>` 分隔符会按换行归一化后再解析。但如果 CDATA 只是单个平面的 XML/HTML 标签，例如 `<b>urgent</b>` 这种行内标记，兼容层会把它保留为原始字符串，而不会强行升成 object / array；只有明显表示结构的 CDATA 片段，例如多兄弟节点、嵌套子节点或 `item` 列表，才会触发结构化恢复。

## 4) 输出结构

`ParseToolCallsDetailed` / `parseToolCallsDetailed` 返回：

- `calls`：解析出的工具调用列表（`name` + `input`）
- `sawToolCallSyntax`：检测到 DSML / canonical wrapper，或命中“缺失 opening wrapper 但可修复”的形态时会为 `true`；裸 `invoke` 不计入该标记
- `rejectedByPolicy`：当前固定为 `false`
- `rejectedToolNames`：当前固定为空数组

解析层不会因为参数值为空而丢弃工具调用。若模型输出了显式空字符串或纯空白参数，它们会按空字符串进入结构化 `tool_calls`；是否拒绝缺参或空命令应由后续工具执行侧 / 客户端 schema 校验决定。Prompt 层仍会要求模型不要主动输出空参数。

完整的 DSML / XML wrapper 只有在成功解析出有效 `invoke name`，并且参数节点（如存在）符合 `parameter` 语义后，才会变成结构化工具调用；真正的零参数工具调用仍然有效。如果 wrapper 完整但内部不是可执行工具调用形态（例如使用 `<param>`、缺少有效 `invoke name`、或其他 malformed XML 工具壳），流式 sieve 会把原始 wrapper 作为普通文本释放，不会吞掉内容，也不会生成空的工具调用。

## 5) 落地建议

1. Prompt 里只示范 DSML 外壳语法。
2. 上游客户端应直接输出完整 DSML 外壳；DS2API 兼容旧式 canonical XML，并只对“closing tag 在、opening tag 漏掉”的常见失误做窄修复，不会泛化接受其他旧格式。
3. 模型只有在知道本次调用所需参数值时才应输出工具调用；不要输出 placeholder、空字符串或纯空白参数。对 `Bash` / `execute_command`，实际命令必须在 `command` 参数里。
4. 不要依赖 parser 做安全控制；执行器侧仍应做工具名和参数校验。

## 6) 回归验证

可直接运行：

```bash
go test -v -run 'TestParseToolCalls|TestProcessToolSieve' ./internal/toolcall ./internal/toolstream ./internal/httpapi/openai/...
./tests/scripts/run-unit-node.sh
```

重点覆盖：

- DSML `<|DSML|tool_calls>` wrapper 正常解析
- legacy canonical `<tool_calls>` wrapper 正常解析
- 固定本地标签名的 DSML 噪声容错形态（如 `<DSML|tool_calls>`、`<<|DSML|tool_calls>`、`<|DSML tool_calls>`、`<DSMLtool_calls>`、`<DSmartToolCalls>`、`<<DSML|DSML|tool_calls>`、`<DSM|tool_calls>...〈/DSM|tool_calls〉`、`<！DSML！tool_calls>...<！/DSML！tool_calls>`）正常解析
- 混搭标签（DSML wrapper + canonical inner）归一化后正常解析
- 波浪线围栏 `~~~` 内的示例不执行
- 嵌套围栏（4 反引号嵌套 3 反引号）内的示例不执行
- Markdown 行内 code span 内的完整工具调用示例不执行
- 文本 mention 标签名后紧跟真正工具调用的场景（含同一 wrapper 变体）
- 空参数结构化保留，malformed executable-looking XML wrapper 作为文本释放
- 非兼容内容按普通文本透传
- 代码块示例不执行
