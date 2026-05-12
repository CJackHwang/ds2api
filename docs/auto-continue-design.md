# Auto Continue + Stream Merger 设计草案

文档导航：[文档索引](./README.md) / [未来主规划](./v2-prd.md) / [M4/M5 执行规划](./m4-development-plan.md) / [History Analyzer](./history-analyzer-design.md)

> Status: M4 design draft.
> 第一版只解决纯文本长输出截断，不处理 tool-call continuation。

## 1. 目标

当 DeepSeek Web 输出被截断或进入可继续状态时，ds2api 自动使用同一 session 继续生成，并把多段输出合并成一次 API 响应。

## 2. 当前基础

| 已具备 | 说明 |
|---|---|
| `internal/completionruntime` | 已统一 completion 启动、空输出 retry、账号切换重试、current-input 文件重传 |
| `internal/sse` / `internal/stream` | 已有 SSE 解析和流式消费 |
| `internal/assistantturn` | 已有输出语义归一、tool fallback、usage 处理 |
| `responsehistory` / `chathistory` | 可记录 continuation 前后的上下文与输出 |
| raw stream samples | 已有 continue / longtext 类样本，可扩展为 golden corpus |

## 3. 配置

建议新增：

```json
{
  "auto_continue": {
    "mode": "off",
    "max_continue_count": 1,
    "max_total_ms": 120000,
    "max_extra_tokens": 12000,
    "pure_text_only": true
  }
}
```

环境变量：

- `DS2API_AUTO_CONTINUE=off|shadow|enforce`
- `DS2API_AUTO_CONTINUE_MAX_COUNT`
- `DS2API_AUTO_CONTINUE_MAX_TOTAL_MS`

模式语义：

| 模式 | 行为 |
|---|---|
| `off` | 不检测、不续写 |
| `shadow` | 检测 continuation 候选并记录 trace，不续写 |
| `enforce` | 满足安全条件时实际续写 |

## 4. Detector

候选信号：

- DeepSeek SSE 状态：`INCOMPLETE` / `AUTO_CONTINUE` / pending fragment。
- 连接异常：stream 非正常结束且没有完整 finish。
- 结构未闭合：Markdown fence、JSON/object/array、XML/HTML 标签、代码块。
- 文本语义：以明显半句结束、常见 “继续” 状态提示。

跳过条件：

- 本轮已经检测到 tool call。
- 请求为 JSON mode / structured output。
- 响应已经进入协议级 error。
- 已超过 `max_continue_count` / `max_total_ms`。
- session 或账号不可复用。

## 5. Merge 策略

non-stream：

1. 收集第一段 assistant turn。
2. detector 判断是否需要 continue。
3. 使用同一 session 调用 DeepSeek continue。
4. 将续写文本追加到同一个 assistant turn。
5. usage、trace、history 记录 continuation 明细。

stream：

1. 第一版只在确认不会破坏 SSE framing 时启用。
2. 每段继续生成必须保持同一个 OpenAI completion id。
3. continuation 边界不向客户端暴露成额外 assistant 消息。
4. 若续写失败，已经发送的内容保持有效，并在 trace/history 中记录失败原因。

## 6. Trace 字段

建议写入 `observe` / history：

| 字段 | 说明 |
|---|---|
| `auto_continue_mode` | off / shadow / enforce |
| `auto_continue_candidate` | 是否命中候选 |
| `auto_continue_reason` | detector 命中的主原因 |
| `auto_continue_count` | 实际续写次数 |
| `auto_continue_skipped_reason` | 跳过原因 |
| `auto_continue_merge_strategy` | append_text / stream_append |
| `auto_continue_failed_reason` | 续写失败原因 |

## 7. 测试矩阵

| 场景 | 期望 |
|---|---|
| 长 Markdown fence 未闭合 | shadow 命中；enforce 续写 |
| JSON 未闭合 | shadow 命中；第一版 enforce 可跳过 |
| 普通完整回答 | 不续写 |
| tool call 输出 | 跳过 |
| 账号 429 | 不无限 retry，记录失败 |
| stream 中断 | shadow 命中，stream enforce 需单独开关 |

## 8. 验收

- `off` 与现有行为一致。
- `shadow` 有报告且不改变响应。
- `enforce` 有硬上限和回滚路径。
- OpenAI Chat non-stream 先落地，stream 后落地。
- Claude / Gemini / Responses 等协议后续扩展，不进入 MVP。
