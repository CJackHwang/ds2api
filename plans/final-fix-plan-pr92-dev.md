# PR92 合并后最终修复计划（dev 分支）

> 目标：在不扩大行为风险的前提下，完成 DeepSeek tool-call 解析链路的稳定化，并通过 CI 门槛测试（含你给的 Actions 门槛）。

## 0. 审核结论（现状）

基于当前 `dev` 代码和本地门禁执行结果，已确认以下现状：

1. **Go 单测整体通过**（`go test ./...` 通过）。
2. **门槛脚本 `check-refactor-line-gate.sh` 当前失败**：
   - `internal/util/toolcalls_parse.go` 行数 **373 > 300**，被结构门禁阻断。
3. **Node 单测存在 2 个失败**（`./tests/scripts/run-unit-all.sh`）：
   - strict mode 场景下，stream sieve 对“带前后 prose 的 tool JSON”行为与测试预期不一致。
4. **Windows 路径反斜杠修复仍有潜在语义污染风险**：
   - `repairInvalidJSONBackslashes` 保留 `\n/\t/\r` 为合法转义，可能把 `C:\new\tmp` 还原成换行/Tab，而不是字面路径。
5. **Go / JS 双实现存在继续漂移风险**：
   - 两端都有 sieve/parse，但严格模式细节和回归用例未完全对齐。

---

## 1. 必过项（先过门槛）

### P0-1. 拆分 `internal/util/toolcalls_parse.go`，通过行数门禁

**目的**：先恢复 CI 结构门禁可通过，避免后续修复无法合并。

**建议拆分方案（最小风险）**：

- `internal/util/toolcalls_parse_core.go`
  - `ParseToolCalls*`、`ParseStandaloneToolCalls*`、`filterToolCallsDetailed`
- `internal/util/toolcalls_parse_payload.go`
  - `parseToolCallsPayload`、`parseToolCallList`、`parseToolCallItem`、`parseToolCallInput`
- `internal/util/toolcalls_repair.go`
  - `repairInvalidJSONBackslashes`、`RepairLooseJSON`、相关正则

**约束**：

- 不改变导出函数签名。
- 拆分后每个文件尽量 <300 行（留 10~15% 缓冲）。
- 仅移动函数，不做语义变更（先过门禁）。

**验收命令**：

```bash
./tests/scripts/check-refactor-line-gate.sh
```

---

### P0-2. 明确并修复 strict mode 行为（Go/JS 一致）

**问题**：Node 两个失败用例说明当前行为已变，但预期未同步，或者实现偏离“strict mode 不拦截混合 prose + tool JSON”的策略。

**两种可选策略（二选一，必须统一）**：

- **策略 A（推荐）**：恢复 strict 语义
  - 只要工具 JSON 前后出现自然语言，视为普通文本，不触发 tool_call 事件。
  - 优点：安全，避免误调用。
- **策略 B**：接受新语义并更新规范
  - 保留当前可解析即拦截的行为。
  - 必须同步更新 Go/JS 测试、README/TESTING 的 strict 定义。

**执行建议**：优先策略 A（更符合“宁可漏触发，不误触发”）。

**验收命令**：

```bash
node --test tests/node/stream-tool-sieve.test.js
./tests/scripts/run-unit-node.sh
```

---

## 2. 正确性修复（功能风险项）

### P1-1. 修复 Windows 路径 `\n/\t/\r` 污染问题

**问题本质**：当前 repair 把 `\n`、`\t`、`\r` 当“应保留的合法转义”，对文件路径场景会造成语义污染。

**目标行为**：

- 在“tool args JSON 字符串上下文”里，路径类字符串应优先保留字面反斜杠。
- 对真实多行文本仍可保持 `\n` 语义（避免过修复）。

**建议实现（低风险）**：

1. 新增“上下文感知修复”函数（例如仅在 `"path"` / `"file"` / `"cwd"` 等键附近启用强修复）。
2. 或采用两阶段反序列化：
   - 先普通 decode；
   - 若命中路径字段且包含控制字符，再回退“反斜杠字面化”重试。
3. 保留 `_raw` 回退路径，避免 silent corruption。

**新增测试（必须）**：

- `{"path":"C:\new\tmp"}` -> 最终 path 含字面 `\\`，不是换行/tab。
- 混合内容：`{"content":"line1\nline2","path":"D:\tmp\a.txt"}` 两字段语义分别正确。
- 非路径文本参数不被过度转义。

**验收命令**：

```bash
go test -v -run TestRepairInvalidJSONBackslashes ./internal/util/
go test -v -run TestParseToolCallsWithInvalidBackslashes ./internal/util/
```

---

### P1-2. `RepairLooseJSON` 深层嵌套能力与边界防护

**问题**：当前正则仅稳定覆盖“单层嵌套对象”；复杂深层对象可能修复失败或误修。

**计划**：

- 将“列表对象补 `[]`”从单纯 regex 升级为轻量扫描器（括号计数 + 状态机）。
- 保留 regex 路径作为 fallback（兼容旧逻辑）。
- 增加最大扫描长度与失败回退，避免 OOM/卡死。

**验收命令**：

```bash
go test -v -run TestRepairLooseJSONWithNestedObjects ./internal/util/
```

---

## 3. 一致性与回归防线

### P2-1. Go/JS parity 套件补齐

**目标**：同一批 fixture 同时跑 Go 与 Node，防止再漂移。

**动作**：

- 新增或扩展 parity fixtures：
  - mixed prose + tool JSON
  - `function.name:` / `[TOOL_CALL_HISTORY]`
  - Windows path + loose JSON
- 在 CI 中把 parity 结果作为 hard gate（至少关键 fixture）。

---

### P2-2. 文档与策略收敛

**目标**：让实现、测试、文档三方一致。

**动作**：

- 在 `README.MD` / `TESTING.md` 补充：
  - strict mode 明确定义
  - 已知修复策略与退化策略（`_raw` 回退）
  - 推荐调试命令矩阵

---

## 4. 建议执行顺序（一天内可落地版本）

1. **先做 P0-1**：文件拆分过 line gate。  
2. **再做 P0-2**：确定 strict 语义并修 Node 失败。  
3. **做 P1-1**：路径转义污染修复 + 回归测试。  
4. **做 P1-2**：loose JSON 深层修复能力增强（可拆次提交）。  
5. **最后 P2**：parity 与文档收口。

---

## 5. 最终验收清单（建议按此顺序跑）

```bash
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-go.sh
./tests/scripts/run-unit-node.sh
./tests/scripts/run-unit-all.sh
npm ci --prefix webui && npm run build --prefix webui
```

若你要和 Actions 完全同构，再补：

```bash
./tests/scripts/run-unit-all.sh
./tests/scripts/check-refactor-line-gate.sh
```

（与当前质量门禁工作流一致即可。）

---

## 6. 风险提示

- 如果优先“放宽解析”以提高命中率，会增加误触发工具风险；建议默认偏保守。
- `repair` 类逻辑必须避免 silent data corruption：宁可 `_raw` 回退，也不要悄悄改坏路径/参数。
- Go/JS 双实现在热修后最容易再次分叉，parity fixture 是最低成本长期保险。
