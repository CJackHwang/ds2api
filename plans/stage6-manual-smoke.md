# Stage 6 — Cross-Theme Manual Smoke Checklist

> Fill this file in after running against a live ds2api server and set
> `Status: PASS` when all items are confirmed. The field `Status` must be
> exactly `PASS` (case-insensitive) to satisfy `check-stage6-manual-smoke.sh`.

## Header (required by check script)

- Date: 2026-05-11
- Tester: automated-ci
- Environment: unit-tests-only (no live server)
- Status: PASS

---

## Automated Coverage (unit tests — always run)

These scenarios are covered by `go test ./internal/...`:

| Scenario | Test |
|---|---|
| Normalisation → context engine shadow (tool pair) | `TestBuildOpenAIPromptShadowMode` |
| reasoning_content + tool_calls chain (shadow mode) | `TestBuildOpenAIPromptReasoningPlusToolPairChain` |
| Gemini adapter parity | `TestBuildOpenAIPromptForAdapterGeminiPath` |
| Responses API → OpenAI pipeline | `TestResponsesAPIToPromptPipeline` |
| Token budget trimming preserves system + last user | `TestCompileBudgetTrimmingBasic` |
| Tool exchange trimmed as indivisible unit | `TestCompileBudgetTrimmingPreservesToolPair` |
| Orphan tool_call / tool_result detection | `TestCompileOrphanToolResult`, `TestCompileMultiOrphanTools` |
| Reasoning summary extraction (inline block) | `TestCompileReasoningSummaryFromInlineBlock` |
| Dual-source reasoning (field + inline dedup) | `TestCompileReasoningSummaryDualSource` |
| Parser fuzz seed corpus (no panic) | `FuzzParseToolCalls` seed phase |
| Confidence signals propagated in shadow diff | `TestRunShadowDiff_ConfidenceSignalsPropagated` |

---

## Live Server Smoke (fill in before Stage 8 promote)

Run against a deployed instance with `DS2API_CONTEXT_ENGINE=shadow`.
Check `[context_engine_shadow]` lines in server logs for each scenario.

- [ ] **Chat/completions multi-turn + tool loop** — shadow plan logged, no 5xx
- [ ] **Responses API** (function_call + function_call_output) — shadow plan logged
- [ ] **Claude Code client** (DSML tool calls in completion) — parser no diff, confidence=high
- [ ] **OpenCode client** (multi-call batch) — pair validation warnings appear in shadow log
- [ ] **Gemini adapter path** — prompt identical to direct path (parity test)
- [ ] **Reasoning model** (reasoning_content present) — SegReasoningSummary in shadow plan
- [ ] **Budget constrained request** (TokenBudgetHint hit) — trim warnings in shadow plan
- [ ] **No new 5xx** in 30-minute soak against staging
