# ds2api DeepSeek 专用智能 Agent 网关方向型 PRD

> Status: future roadmap source of truth.
> 本文是 2026-05 起 ds2api 后续方向的主要规划文档。旧 v2 里程碑文档仅作为已执行阶段记录归档；如有冲突，以本文为准。
>
> 适用仓库：`Activer007/ds2api`  
> 参考上游：`CJackHwang/ds2api`  
> 文档定位：方向型 PRD，供开发团队、Codex、Claude Code、Subagent 拆解后续任务使用  
> 版本：v1.0  
> 重点：不做过细接口设计，聚焦产品定位、架构方向、核心能力和阶段路线

---

## 1. 一句话结论

`ds2api` 下一阶段不应继续扩展成“大而全”的多模型 Provider 网关，而应聚焦成为 **DeepSeek 专用智能 Agent API 引擎**。

核心目标是：

```text
让 DeepSeek 在 Codex、Claude Code、OpenCode、Trae、OpenAI SDK 等 Agent 场景中更稳定、更连续、更可观测、更易部署。
```

也就是说，后续重点不是“再支持更多模型”，而是让 DeepSeek 在长任务、工具调用、上下文管理、自动续写、搜索能力、账号调度和 WebUI 运维上更像一个成熟 API 平台。

---

## 2. 产品定位

### 2.1 当前定位

当前 fork 已经不是简单 API 转换器，而是一个 DeepSeek 专用多协议适配网关雏形。

它对外兼容：

- OpenAI Chat / Responses；
- Claude Messages；
- Gemini generateContent；
- Ollama 兼容入口；
- WebUI 管理后台；
- Vercel Node 流式桥接。

它对内主要服务：

- DeepSeek Web 会话；
- PoW；
- Session；
- 文件上传；
- 搜索 / 思考 / 视觉能力；
- Tool Call 解析；
- current-input 上下文文件化；
- 多账号调度。

### 2.2 后续定位

推荐正式定位为：

```text
DeepSeek 专用智能 Agent API 引擎
```

含义如下：

| 维度 | 定位 |
|---|---|
| 对外 | 保留 OpenAI / Claude / Gemini 等 API 形态，方便客户端接入 |
| 对内 | 只聚焦 DeepSeek，不扩展多个上游 Provider |
| 核心 | 强化 Agent 长任务、上下文、工具调用、自动续写、搜索、账号稳定性 |
| 产品化 | WebUI 从后台升级为配置中心、诊断中心、观测中心 |
| 用户 | Codex、Claude Code、OpenCode、Trae、AI IDE、Agent 框架用户 |

### 2.3 明确不做

| 不做 | 原因 |
|---|---|
| 不做多 Provider 抽象 | 与 DeepSeek 专用路线冲突，容易分散主线 |
| 不接入 Kimi / Qwen / GLM / Claude / OpenAI 上游 | 当前目标不是通用 LLM Gateway |
| 不做 SaaS 多租户和计费 | 现阶段优先解决稳定性、可观测性和部署体验 |
| 不一次性重写 WebUI | 采用渐进式增强，先做配置、安全、诊断、观测 |
| 不默认强开新 Parser / Context Engine | 必须先 shadow、分析、审计，再 enforce |

---

## 3. 当前核心问题

### 3.1 长输出容易中断

DeepSeek Web UI 中可以手动 Continue，但 API 客户端往往只得到截断响应。对于生成长代码、长报告、复杂分析和 Agent 多步任务，这是明显短板。

### 3.2 Agent 上下文体验不够连续

在 Codex、Claude Code、OpenCode 等场景中，用户期望模型持续理解任务状态、文件状态和工具调用结果。当前仅靠 prompt 拼接和 current-input 文件化还不够，需要更明确的 Context State Engine。

### 3.3 Tool Call 仍是高风险模块

Tool Call 的稳定性决定 Agent 能不能闭环。任何控制 token 泄漏、半截 chunk 误发、DSML/XML 漂移解析失败、Go/Node 行为不一致，都可能让 Agent loop 断掉。

### 3.4 Search / Thinking / Session Split 存在隐性耦合

DeepSeek 的搜索、思考、视觉、nothinking、current-input 文件化、session split 之间存在耦合。不能只靠模型名后缀和 prompt 拼接处理，需要能力路由层。

### 3.5 目前仍偏“人工排障”

项目已经保存了会话历史、final prompt、reasoning、content、error、usage、elapsed 等信息，但还缺少自动分析工具。后续应该把会话历史升级为质量诊断数据源，而不是只作为查看记录。

---

## 4. 核心建设方向

## 4.1 DeepSeek Runtime Core

### 方向

将 DeepSeek 会话、补偿、续写、搜索、文件、账号调度从散落逻辑中逐步收束出来，形成 DeepSeek 专用运行时核心。

建议抽象为：

```text
DeepSeek Runtime Core
  Session
  Completion
  Continuation
  Capability
  FileContext
  AccountGuard
  Trace
```

### 目标

- 会话生命周期更清晰；
- stream / non-stream 行为更一致；
- 账号切换、文件重上传、空输出重试逻辑更可靠；
- search / thinking / vision 能力可被路由和解释；
- 故障排查可以从 trace 看到完整链路。

### 原则

不要把它做成通用 Provider 抽象，而是做成 DeepSeek 专用运行时边界。

---

## 4.2 Auto Continue + Stream Merger

### 方向

构建自动续写能力，让 ds2api 在 DeepSeek 输出被截断时，自动使用同一 session 继续生成，并把多段输出安全合并成一次 API 响应。

### 解决的问题

- 长代码生成被截断；
- SSE stream 中途断；
- 用户需要手动提示“请继续”；
- Agent 客户端无法自动处理 Web UI 的 Continue 行为。

### 初期范围

第一版不要贪大，建议只做：

| 范围 | 策略 |
|---|---|
| OpenAI Chat non-stream | 优先支持 |
| OpenAI Chat stream | 优先支持 |
| Responses / Claude / Gemini | 后续扩展 |
| Tool Call continuation | 暂缓，先处理纯文本长输出 |
| 默认行为 | 默认关闭，可灰度 |

### 判断是否成功

- 长代码和长文档输出不再明显截断；
- 流式响应结构不被破坏；
- 续写次数、原因、失败原因可观测；
- 能从历史会话中自动识别 Auto Continue 候选样本。

---

## 4.3 Context State Engine

### 方向

将 Context Engine 从“prompt 编译计划”继续升级为“任务状态管理能力”。

当前已经具备 ContextPlan、token budget、tool pair 校验、reasoning summary 等基础，后续应进一步关注跨轮任务连续性。

### 重点能力

| 能力 | 作用 |
|---|---|
| Task Memory | 记录任务目标、阶段、已完成事项、待完成事项 |
| Decision Log | 记录关键决策，避免模型反复推理 |
| File Snapshot | 记录文件 digest、是否已读、是否仍在上下文 |
| Tool Pair Protection | 防止 tool_call / tool_result 断链 |
| Context Diff | 解释本轮保留、裁剪、摘要、复用原因 |
| Project Isolation | 按 API Key / Project / Client 隔离上下文状态 |

### 策略

- 普通调用默认 shadow；
- Agent 客户端可配置 enforce；
- 不直接注入完整 thinking，优先注入 reasoning summary / decision log；
- 当前用户请求、system、工具协议约束必须高优先级保护；
- 文件上下文用 digest 和 snapshot 管理，减少重复读取。

---

## 4.4 Tool Parser v2 稳定化

### 方向

Tool Parser 不应继续靠零散正则补丁推进，而应走证据化路线：样本、shadow、diff、confidence、Go/Node parity、再 enforce。

### 重点工作

- 建立 marker leak golden corpus；
- 覆盖跨 chunk 工具结果泄漏；
- 覆盖 DSML/XML 漂移；
- 覆盖 Markdown code block / inline code false positive；
- 保持 Go 主链路和 Vercel Node stream 行为一致；
- 在 WebUI 展示 parser shadow diff 和风险样本。

### 判断是否成功

- 可见输出中不再出现 DeepSeek 控制 token；
- tool_call 不被当作正文；
- 普通 XML、Markdown 示例、代码块不会被误杀；
- shadow 数据能支持是否进入 enforce 的判断。

---

## 4.5 DeepSeek Capability Router

### 方向

建立 DeepSeek 能力路由层，统一管理 search、thinking、vision、nothinking、current-input 文件化、session split 之间的关系。

### 解决的问题

- search 模型无法联网；
- thinking 干扰 search；
- nothinking 模型仍出现 reasoning；
- current-input 文件化影响搜索；
- 模型 alias 映射后能力边界不清楚。

### 重点能力

| 能力 | 说明 |
|---|---|
| Model Capability Profile | 每个模型明确 search / thinking / vision / current-input 能力 |
| Policy Router | 根据模型和请求自动选择策略 |
| Risk Warning | 对已知冲突给出警告或自动降级 |
| WebUI Matrix | 在 WebUI 显示模型能力矩阵 |
| Trace | search 失败时能解释是模型能力、thinking、session split 还是上游问题 |

---

## 4.6 WebUI v2 配置与观测中心

### 方向

WebUI 不应只是后台，而应成为：

```text
安全初始化入口
+ 配置中心
+ 运行状态仪表盘
+ 历史诊断面板
+ 排障工作台
```

### 重点建设

| 模块 | 方向 |
|---|---|
| 首次初始化 | 未设置 admin 密码时必须进入 bootstrap 流程 |
| 配置中心 | 每个配置展示说明、推荐值、风险等级、热更新能力 |
| Feature Flags | parser/context/auto_continue 支持 off/shadow/enforce 管理 |
| DeepSeek Trace | 展示 session、PoW、retry、continue、search、account switch |
| Parser 面板 | 展示 diff、confidence、suppressed token、异常样本 |
| Context 面板 | 展示 ContextPlan、裁剪、摘要、file reuse |
| Account 面板 | 展示 in-flight、queue、429、冷却、成功率、延迟 |
| History Analysis | 展示自动分析出的异常会话和建议动作 |

---

## 4.7 History Analyzer 会话历史智能诊断

### 方向

将已保存的会话历史从“查看记录”升级为“质量诊断数据源”。

当前历史中已经保存了请求入口、模型、是否流式、消息、history_text、final_prompt、reasoning_content、content、error、status_code、elapsed_ms、finish_reason、usage 等信息。这些字段足以支撑第一版规则化分析。

### 目标

History Analyzer 要回答：

```text
这次请求是否正常？
如果不正常，属于哪类问题？
应该补 Parser 样本，还是改 Context？
是否是 Auto Continue 候选？
是否是 Search / Thinking 冲突？
是否是账号或性能问题？
```

### 重点分析方向

| 方向 | 检测内容 |
|---|---|
| Tool Call | marker 泄漏、tool_call 被当正文、false positive、Go/Node 差异 |
| Context | 上下文孤立、history 文件引用异常、tools 文件缺失、reasoning 膨胀、tool pair 断链 |
| Auto Continue | 输出截断、代码块未闭合、JSON 未闭合、SSE 中断、continue 信号 |
| Search / Thinking | search 无联网结果、thinking 干扰、nothinking 违规、能力路由异常 |
| Account / Runtime | 429、空输出、账号切换后恢复、耗时异常、stream 路径异常 |

### 产品形态

建议分三步：

1. **CLI 离线分析**：生成 Markdown / JSON 报告；
2. **Admin API**：供 WebUI 查询分析结果；
3. **WebUI 诊断面板**：展示异常会话、问题分布、推荐动作。

### 原则

- 第一版先做规则，不急着让 LLM 读历史；
- 默认脱敏；
- 不影响主请求链路；
- 每个高风险问题都要能转化成开发任务或测试样本。

### 价值

History Analyzer 将成为 ds2api 的“黑匣子分析仪”：

```text
用户遇到问题
  -> 历史自动扫描
  -> 问题自动归类
  -> 样本自动沉淀
  -> 形成 PR 方向
  -> 下一轮迭代更准
```

---

## 5. 推荐阶段路线

## M4-A：History Analyzer + Release Readiness

### 目标

先让已有数据变成诊断能力，为 Parser、Context、Auto Continue 的后续推进提供证据。

### 工作方向

- 建立 History Analyzer 基础规则；
- 扫描会话历史，输出异常报告；
- 补充 Tool Parser marker leak / false positive 样本；
- 增加 Parser shadow report；
- 增加 Context shadow report；
- 输出 release readiness 报告。

### 成功标志

- 能自动识别高风险会话；
- 能指出问题属于 Tool、Context、Continue、Search 还是 Account；
- 能输出建议加入 fixtures 的样本；
- 能辅助判断 parser/context 是否适合进入 shadow 或 enforce。

---

## M4-B：Auto Continue MVP

### 目标

解决长输出截断和 SSE 中断问题。

### 工作方向

- 增加 auto_continue 配置；
- 增加 continuation detector；
- 支持同 session Continue；
- 支持 OpenAI Chat stream / non-stream 合并；
- 结合 History Analyzer 自动沉淀长输出截断样本。

### 成功标志

- 长代码生成不再明显截断；
- 输出合并不破坏 SSE；
- continue 次数和原因可观测；
- 默认关闭，可灰度。

---

## M4-C：Capability Router

### 目标

解决 DeepSeek search / thinking / vision / nothinking / current-input 文件化之间的能力冲突。

### 工作方向

- 建立模型能力 profile；
- 制定 search / thinking 策略；
- 在 WebUI 展示模型能力矩阵；
- 结合历史分析识别 search 失败样本。

### 成功标志

- search 模型联网能力更稳定；
- thinking 冲突可被解释或自动降级；
- nothinking 模型行为更符合预期。

---

## M4-D：WebUI v2 配置与观测中心

### 目标

降低部署、配置和排障难度。

### 工作方向

- 首次登录设置 admin 密码；
- 配置项 schema 和风险说明；
- Feature Flag 面板；
- Parser / Context / Auto Continue / Account / Search 观测页；
- History Analysis 诊断页。

### 成功标志

- 普通用户能看懂配置；
- 出问题能快速定位；
- 高风险配置有提示；
- 历史异常能在 WebUI 中看到。

---

## M5：Agent 长任务增强

### 目标

让 ds2api 更适合 Codex、Claude Code、OpenCode、Trae 等长任务 Agent。

### 工作方向

- Context Engine Agent profile；
- Task Memory；
- Decision Log；
- File Snapshot；
- Project Isolation；
- Agent E2E 测试集。

### 成功标志

- 多轮任务不再像重新开始；
- 重复 read_file 明显减少；
- tool_call / tool_result 不断链；
- Agent 客户端稳定完成较长任务。

---

### 执行文档索引

后续开发以本文为产品方向，以以下文档作为执行依据：

| 文档 | 用途 |
|---|---|
| [`m4-development-plan.md`](./m4-development-plan.md) | M4/M5 里程碑、任务拆分、PR 顺序和门禁 |
| [`history-analyzer-design.md`](./history-analyzer-design.md) | History Analyzer 规则、报告结构、CLI/Admin/WebUI 演进 |
| [`auto-continue-design.md`](./auto-continue-design.md) | Auto Continue detector、配置、stream merge 和 trace 设计 |
| [`capability-router.md`](./capability-router.md) | DeepSeek 模型能力 profile、策略路由、WebUI matrix |
| [`release-readiness.md`](./release-readiness.md) | 发布候选报告、feature flag 晋级证据和 GO/NO-GO 口径 |
| [`webui-v2-observability.md`](./webui-v2-observability.md) | WebUI v2 诊断中心、观测页和 Admin API 需求 |

这些执行文档应随代码进展持续更新；当实现改变用户可见行为时，同步更新 `prompt-compatibility.md`、`toolcall-semantics.md`、`ARCHITECTURE*.md` 或 `API*.md`。

---

## 6. 优先级建议

| 优先级 | 方向 | 原因 |
|---|---|---|
| P0 | History Analyzer | 先有自动诊断，后续改造才有证据 |
| P0 | Tool Parser 稳定化 | Agent 工具调用稳定性的底座 |
| P0 | Auto Continue MVP | 直接解决长输出中断体感问题 |
| P0 | Context Engine shadow 审计 | 为 Agent enforce 做准备 |
| P1 | Capability Router | 解决 search / thinking 隐性冲突 |
| P1 | WebUI Bootstrap | 生产部署安全底线 |
| P1 | WebUI 配置中心 | 降低使用门槛 |
| P1 | Account Guard | 降低账号异常和高频调用风险 |
| P2 | Replay Lab | 提升 parser/context 调试效率 |
| P2 | Release Trust | SBOM、checksum、签名、发布可信度 |

---

## 7. 开发组织建议

### 7.1 总体策略

采用“主控工程师 + Subagent 并行”的方式，但主链路必须由主控工程师串行集成。

### 7.2 分工建议

| 角色 | 负责方向 |
|---|---|
| Runtime Agent | Auto Continue、DeepSeek Runtime、stream merge |
| Parser Agent | Tool Parser corpus、shadow diff、Go/Node parity |
| Context Agent | Context Engine、ContextPlan、Agent profile |
| History Analyzer Agent | 会话历史分析、报告、规则引擎 |
| WebUI Agent | 配置中心、诊断面板、观测页 |
| Test Agent | fixtures、E2E、run-live、兼容矩阵 |
| Docs Agent | PRD、部署文档、排障手册、release report |

### 7.3 分支建议

```text
main
  <- m4/history-analyzer-release-readiness
  <- m4/auto-continue
  <- m4/capability-router
  <- m4/webui-v2
  <- m5/agent-context
```

---

## 8. 判断方向正确的标准

后续每个阶段不只看“功能是否完成”，而要看是否改善以下指标：

| 指标 | 说明 |
|---|---|
| 长输出完成率 | 长代码、长报告是否减少截断 |
| Tool Call 成功率 | 工具调用是否稳定进入结构化输出 |
| Marker Leak 数量 | 可见输出中控制 token 是否消失 |
| Context 连续性 | 多轮任务是否不再像重新开始 |
| 重复读文件次数 | Agent 是否减少无意义 read_file |
| Search 成功率 | search 模型是否稳定联网 |
| 账号异常率 | 429、空输出、切换账号是否可观测 |
| 排障耗时 | 从问题出现到定位原因是否变快 |
| Shadow 晋级依据 | parser/context 是否有数据支撑 enforce |

---

## 9. 最终建议

当前最推荐的路线是：

```text
第一步：Release Readiness baseline
第二步：History Analyzer CLI / report
第三步：Tool Parser 与 Context shadow 审计
第四步：Auto Continue MVP
第五步：Capability Router profile
第六步：WebUI v2 配置与诊断中心
第七步：Agent 长任务增强
```

其中 History Analyzer 应该提前做，因为它能把后续所有改造从“凭感觉”变成“有证据”。

---

## 10. 总结

`ds2api` 后续最有潜力的方向，不是成为另一个多模型代理网关，而是成为 DeepSeek 的智能运行时外骨骼。

它应该重点解决：

```text
长输出不中断
上下文不失忆
Tool Call 不泄漏
Search / Thinking 不冲突
账号调度可解释
历史问题自动诊断
配置和观测足够清晰
```

这条路线更聚焦，也更容易形成差异化。真正的护城河不是“支持多少 Provider”，而是 **让 DeepSeek 在 Agent 工程场景中跑得稳、看得清、修得快**。
