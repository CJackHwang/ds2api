# DeepSeek SSE 行为结构说明（第三方逆向版）

> 说明：本文基于当前仓库 `tests/raw_stream_samples/` 下全部 `upstream.stream.sse` 原始流样本整理而成，属于第三方逆向观察文档，不是官方协议。
> 当前 corpus 由 4 份原始流组成，覆盖搜索+引用、风控终态、Markdown 输出和空格敏感输出等行为。
> 补充：文末还会注明少量“当前实现已确认、但 corpus 尚未完整覆盖”的行为，例如长思考场景下的自动续写状态。

文档导航：[文档总索引](./README.md) / [测试指南](./TESTING.md) / [样本目录说明](../tests/raw_stream_samples/README.md)

## 1. 样本覆盖

下列样本共同构成了本文的观察基础：

| 样本 | 观察重点 |
| --- | --- |
| [guangzhou-weather-reasoner-search-20260404](../tests/raw_stream_samples/guangzhou-weather-reasoner-search-20260404/upstream.stream.sse) | 搜索+思考流程，包含 `reference:N` 引用标记与工具片段 |
| [content-filter-trigger-20260405-jwt3](../tests/raw_stream_samples/content-filter-trigger-20260405-jwt3/upstream.stream.sse) | `CONTENT_FILTER` 终态分支，包含拒答模板与 `ban_regenerate` |
| [markdown-format-example-20260405](../tests/raw_stream_samples/markdown-format-example-20260405/upstream.stream.sse) | Markdown 输出的早期样本，用于观察 token 级输出形态 |
| [markdown-format-example-20260405-spacefix](../tests/raw_stream_samples/markdown-format-example-20260405-spacefix/upstream.stream.sse) | Markdown 输出修正样本，用于验证空格 chunk 必须保留 |

当前 corpus 的整体特征是 `message` 帧占绝对多数，控制事件只占很小一部分，但它们决定了流的生命周期和最终状态。

## 2. 总体结构

DeepSeek 的这类输出可以分成两层看：

1. SSE 事件层。
2. JSON 载荷层。

事件层负责传输边界，载荷层负责业务状态。实现时不要把 HTTP chunk、SSE block 和业务 JSON 混为一体。

最常见的时序可以概括为：

```text
ready
update_session
message(初始化 envelope)
message(正文 / 片段 / 状态增量)
message(状态收口)
finish
update_session
title
close
```

`finish` 表示生成流结束，但不是唯一的终止信号；真正的语义终态通常还要结合 `response/status`、`quasi_status` 和 `close` 一起判断。

## 3. SSE 事件层

当前 corpus 中观察到的事件类型如下：

| 事件 | 作用 | 处理建议 |
| --- | --- | --- |
| `ready` | 传输层就绪，通常携带 `request_message_id`、`response_message_id`、`model_type` | 记录元数据即可，不参与正文拼接 |
| `update_session` | 会话时间戳或心跳更新 | 当作会话状态帧处理 |
| `message` | 主体载荷，绝大多数业务信息都在这里 | 必须按顺序解析并保序累积 |
| `finish` | 生成阶段结束 | 作为流结束标记之一 |
| `title` | 会话标题生成结果 | 元数据帧，不参与正文拼接 |
| `close` | 连接关闭信息 | 仅用于收尾与审计 |

说明：

- `message` 是默认事件名，SSE 中没有显式 `event:` 时也应按 `message` 处理。
- 目前样本里大量 `message` 帧没有独立的业务前缀，不能靠事件名区分正文和控制帧。
- 可能出现空 payload 的 `message` 帧；它们应被视为 no-op，但不能打乱事件顺序。

## 4. 载荷层形态

`message` 的 `data:` 部分不是单一 schema，而是多种结构混合。当前 corpus 里主要见到以下几种形态：

| 形态 | 典型结构 | 作用 |
| --- | --- | --- |
| 初始化 envelope | `{"v":{"response":{...}}}` | 给出会话初始状态、模型状态和片段容器 |
| 纯文本 token | `{"v":"..."}` | 直接输出可见文本 token |
| 路径补丁 | `{"p":"...","o":"APPEND|SET|BATCH","v":...}` | 对某个状态路径做增量更新 |
| 终态 batch | `{"v":[{"p":"status","v":"CONTENT_FILTER"}, ...]}` | 尾部状态收口，常见于风控终态 |

一个简化后的典型样式如下：

```json
{"v":"输出"}
{"p":"response/fragments/-1/content","o":"APPEND","v":"..."}
{"p":"response/fragments","o":"APPEND","v":[...]}
{"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":211},{"p":"quasi_status","v":"FINISHED"}]}
{"p":"response/status","o":"SET","v":"FINISHED"}
```

注意：

- `v` 可能是字符串、对象、数组、布尔值或数字。
- `o` 当前样本里主要见到 `APPEND`、`SET`、`BATCH`。
- `v` 为数组时，通常表示一个批量 patch 集合，而不是正文数组。

## 5. 初始化 envelope

每条流开头，常会先出现一个 `message` 帧，内容是完整的 `response` 初始状态。当前 corpus 中，这个 envelope 常见字段包括：

- `message_id`
- `parent_id`
- `model`
- `role`
- `thinking_enabled`
- `ban_edit`
- `ban_regenerate`
- `status`
- `incomplete_message`
- `accumulated_token_usage`
- `files`
- `feedback`
- `inserted_at`
- `search_enabled`
- `fragments`
- `conversation_mode`
- `has_pending_fragment`
- `auto_continue`
- `search_triggered`

这些字段更像会话状态和策略开关，不是正文内容。第三方实现应把它们保留在内部状态树里，而不是直接拼接到最终答案。

## 6. 路径结构

当前 corpus 里观察到的 `p` 路径可以归成几组：

### 6.1 片段级路径

- `response/fragments/-N/content`
- `response/fragments/-N/status`
- `response/fragments/-N/results`
- `response/fragments/-N/elapsed_secs`

这类路径表示某个片段对象的增量更新。`-N` 只是样本中的索引风格，不应被写死成固定数量。

### 6.2 片段容器路径

- `response/fragments`
- `fragments`

这两类路径通常承载 fragment 数组。前者更像响应树中的分支，后者更像终态批处理里的片段集合。

### 6.3 语义状态路径

- `response/status`
- `response/has_pending_fragment`
- `quasi_status`
- `status`
- `ban_regenerate`

这类路径决定流是否结束、是否被风控、是否还有待处理片段。它们不应作为正文输出。

尤其是 `response/status` / `status` 这类路径上的字符串值，应被视为控制信号而不是文本 token。当前已确认需要特殊对待的值包括：

- `FINISHED`：正常完成终态，应触发收口。
- `CONTENT_FILTER`：风控终态，应走拒答/模板分支。
- `WIP` / `INCOMPLETE` / `AUTO_CONTINUE`：未完成但可继续生成的中间状态，不应直接输出给客户端。

### 6.4 统计与进度路径

- `accumulated_token_usage`

这类路径用于使用量或进度统计，属于元数据。

### 6.5 非命名空间字段

在片段对象内部，还会看到 `content`、`references`、`result`、`queries`、`stage_id` 等字段。它们不一定带 `response/...` 前缀，但仍然是协议语义的一部分。

## 7. fragment 类型

当前 corpus 里已经观察到的 fragment 类型如下：

| 类型 | 作用 | 是否应直接渲染 |
| --- | --- | --- |
| `RESPONSE` | 正常回答片段 | 是，属于正文 |
| `THINK` | 推理或阶段提示 | 通常否，按产品策略决定是否展示 |
| `TOOL_SEARCH` | 搜索工具调用元数据 | 否 |
| `TOOL_OPEN` | 打开 / 抽取结果的工具元数据 | 否 |
| `TIP` | 提示 / 警告类片段，常带 `style: WARNING` | 视产品策略决定，通常作为附注 |
| `TEMPLATE_RESPONSE` | 风控拒答模板 | 是，但它属于终态 fallback，不是普通正文 |

观察到的典型片段字段：

- `id`
- `type`
- `content`
- `references`
- `stage_id`
- `status`
- `queries`
- `results`
- `result`
- `elapsed_secs`
- `style`
- `hide_on_wip`

第三方实现不要把 `fragment.type` 和 `p` 路径混为一谈。`type` 是语义分类，`p` 是状态树位置。

## 8. 终态行为

当前 corpus 里有两条很重要的终态分支。

### 8.1 正常完成

正常回答通常会出现如下收口顺序：

1. `response` 的 `BATCH` 更新 `accumulated_token_usage`。
2. `response` 的 `BATCH` 或单独 patch 更新 `quasi_status: FINISHED`。
3. `response/status` 置为 `FINISHED`。
4. `finish` 事件到来。
5. 之后可能还有 `update_session`、`title`、`close`。

### 8.2 风控终态

`content-filter-trigger-20260405-jwt3` 展示了另一种终态路径：

1. 先继续输出一段正常正文。
2. 出现提示类 fragment，例如 `TIP`。
3. 可能先把 `quasi_status` 提前收口到 `FINISHED`。
4. 之后出现一个终态 batch，把 `ban_regenerate` 设为 `true`，把 `status` 置为 `CONTENT_FILTER`，并附带 `TEMPLATE_RESPONSE`。
5. 最后再出现 `finish`，然后是收尾事件。

这个分支说明：

- `finish` 不等于正常结束。
- `CONTENT_FILTER` 是一个独立终态，不是普通异常。
- `TEMPLATE_RESPONSE` 不应被当作常规回答流的中间片段，它是终态 fallback。

一个简化的风控尾部可以写成：

```json
{"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":1269},{"p":"quasi_status","v":"FINISHED"}]}
{"v":[{"p":"ban_regenerate","v":true},{"p":"status","v":"CONTENT_FILTER"},{"p":"fragments","v":[{"id":38,"type":"TEMPLATE_RESPONSE","content":"..."}]},{"p":"quasi_status","v":"CONTENT_FILTER"}]}
{"event":"finish"}
```

### 8.3 自动续写中间态（实现补充）

这部分不是当前 corpus 的直接覆盖项，而是 2026-04-05 在长思考实测中观察到、且已在当前实现中兼容的行为：

1. 上游可能先把 `response/status` 或 envelope 内的 `response.status` 置为 `WIP` / `INCOMPLETE`。
2. 有时还会伴随 `auto_continue: true`。
3. 这表示当前轮输出尚未真正结束，客户端或代理层可以继续调用 continue 接口续写同一条回答。
4. 续写后的内容会承接之前的思考与正文，不应把前一轮状态值泄露成可见文本。

对第三方实现，建议把这一类状态统一当作“可继续的控制信号”：

- 可以据此决定是否继续拉取后续流。
- 不能把 `INCOMPLETE`、`WIP`、`AUTO_CONTINUE` 直接拼接到最终文本。
- `finish` 事件本身也不能单独说明回答已完全结束，仍要结合状态字段判断。

## 9. 文本重建规则

如果你的目标是把流重建成最终可见文本，必须遵守下面这些规则：

- 按接收顺序逐个追加 token。
- 不要对每个 `v` 做 `trim` 或 `TrimSpace`。
- 不要丢弃只包含空格的 chunk。
- 不要合并连续空格、换行或 Markdown 符号附近的空白。
- 不要把 `[reference:N]` 视为协议元数据，它在当前 corpus 里就是正文的一部分。
- 如果你要屏蔽引用标记，应当把它做成可配置的后处理，而不是在解析阶段硬删。
- `response/status` / `status` 路径上的状态字符串不应进入正文，即使它们不是终态。

这点对 Markdown、代码块、引用、表格都很关键。样本里已经证明，`#`、`-`、`>`、`|` 这类符号后面的空格必须原样保留，否则渲染结果会变形。

## 10. 推荐实现方式

对第三方开发者，建议把实现拆成三条线：

1. 原始事件线：保留 SSE block 顺序、事件名和完整 JSON 载荷。
2. 状态树线：维护 `response`、`fragments`、`status`、`quasi_status` 等结构。
3. 可见文本线：只从明确应渲染的 token / fragment 中拼接最终文本。

一个简单的处理顺序可以是：

```text
parse SSE block
  -> 识别 event
  -> 解析 JSON payload
  -> 更新状态树
  -> 识别 status / quasi_status / auto_continue 等控制信号
  -> 判定是否有可见文本
  -> 追加到输出缓冲
  -> 遇到 WIP / INCOMPLETE / AUTO_CONTINUE 时决定是否续写
  -> 遇到 FINISHED / CONTENT_FILTER / finish 时收口
```

实现时的兼容原则：

- 未知路径保留，不要报错中断。
- 未知 fragment.type 保留在日志里。
- 不要假设所有模型都一定输出 `thinking_content`，当前 corpus 的推理更多是通过 fragment 类型表达。
- 不要假设 `title` 一定存在，它只是后置元数据。

## 11. 本 corpus 证明了什么

当前样本足以证明以下行为：

- 搜索类模型会把工具调用、结果、引用和正文混在同一条 SSE 流里。
- 风控不会简单地“没有输出”，而是会在正常生成后切换到 `CONTENT_FILTER` 终态。
- Markdown 和代码输出对空格非常敏感，空格 chunk 不能吞。
- `message` 是主体承载层，`ready` / `update_session` / `finish` / `title` / `close` 是控制层。
- `fragment.type` 是可视化和工具链分层的关键，不应只靠 `p` 路径判断。

结合 2026-04-05 的长思考实测，还可以补充一条当前实现层面的结论：

- 长思考场景下，上游可能先给出 `INCOMPLETE` / `WIP` / `AUTO_CONTINUE` 状态，再通过 continue 链路续写；这些状态值本身不应作为正文输出。

## 12. 适用边界

本文是基于当前 corpus 的逆向说明，不是恒定协议。

- 新模型可能增加新的 `p` 路径。
- 新版本可能增加新的 fragment.type。
- `CONTENT_FILTER` 的终态模板内容可能变化。
- 自动续写相关状态（如 `INCOMPLETE` / `AUTO_CONTINUE`）当前主要来自实测与实现兼容逻辑，后续字段形态仍可能变化。
- 解析器应当对未知字段、未知路径、未知事件保持容忍。

如果你要把这份说明用于实际开发，建议同时保留原始流样本、回放脚本和回归测试，不要只依赖本文。
