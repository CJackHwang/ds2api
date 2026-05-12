# WebUI v2 配置与观测中心规划

文档导航：[文档索引](./README.md) / [未来主规划](./v2-prd.md) / [M4/M5 执行规划](./m4-development-plan.md) / [History Analyzer](./history-analyzer-design.md) / [Capability Router](./capability-router.md)

> Status: M4 design draft.
> WebUI v2 采用渐进增强，不做一次性重写。

## 1. 目标

把 WebUI 从“后台配置页”升级成：

```text
安全初始化入口
+ 配置中心
+ 运行状态仪表盘
+ 历史诊断面板
+ 排障工作台
```

## 2. 当前基础

| 已具备 | 说明 |
|---|---|
| Account 页面 | 账号增删改、测试、队列卡片 |
| Settings 页面 | runtime、current-input、security、feature flags |
| Chat History 页面 | 历史列表和详情 |
| API Tester | 基础请求调试 |
| Vercel 页面 | 同步和部署引导 |

## 3. 新增信息架构

建议新增一级入口 `Diagnostics`：

- Overview：健康摘要、最近异常、release readiness 状态。
- History Analysis：Analyzer 报告列表和 finding 详情。
- Parser：shadow diff、confidence、marker leak、fixtures 候选。
- Context：ContextPlan、trimmed、warnings、tool pair、reasoning summary。
- Auto Continue：candidate、continue count、skip reason、merge trace。
- Capability：模型能力矩阵、search/thinking/current-input warning。
- Account：429、retry、account switch、TTFT、queue、in-flight。

## 4. 页面优先级

| 阶段 | 页面 | 说明 |
|---|---|---|
| M4.1 | History Analysis | 展示离线/后台生成的报告 |
| M4.2 | Parser + Context | 展示 shadow report |
| M4.3 | Auto Continue | 展示 detector 和 merge trace |
| M4.4 | Capability Matrix | 展示模型能力和策略 warning |
| M4.5 | Release Readiness | 发布前 GO / NO-GO 面板 |

## 5. 交互原则

- 先只读展示，再考虑写操作。
- 所有高风险开关需要风险说明和二次确认。
- 不展示未脱敏 prompt、token、账号凭证。
- 报告详情必须能跳转到对应历史记录。
- 空状态显示“暂无数据”，不要用大段说明文字占据主界面。

## 6. Admin API 需求

| API | 用途 |
|---|---|
| `GET /admin/history-analysis/reports` | 报告列表 |
| `GET /admin/history-analysis/reports/{id}` | 报告详情 |
| `POST /admin/history-analysis/run` | 触发分析 |
| `GET /admin/release-readiness/latest` | 最新 readiness |
| `GET /admin/capability-router/matrix` | 能力矩阵 |
| `GET /admin/auto-continue/traces` | continuation trace |

## 7. 验收

- 新页面不阻塞现有设置和账号管理工作流。
- 移动端可读，但优先保证桌面排障效率。
- WebUI build 通过。
- 对应 Admin API 有鉴权和脱敏测试。
