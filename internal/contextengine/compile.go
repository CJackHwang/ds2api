package contextengine

import (
	"encoding/json"
	"fmt"
	"strings"

	"ds2api/internal/util"

	"github.com/google/uuid"
)

const (
	// reasoningTailChars is the number of tail characters to preserve when
	// summarising a large reasoning block.
	reasoningTailChars = 500
	// reasoningSummaryThreshold: reasoning blocks shorter than this are kept
	// verbatim; longer blocks are replaced with "[N chars omitted]\n<tail>".
	reasoningSummaryThreshold = reasoningTailChars * 2
)

// extractReasoningBlock parses the promptcompat-formatted labeled block
// "[reasoning_content]\n{text}\n[/reasoning_content]" out of content.
// Returns the raw reasoning text and the remaining visible content.
func extractReasoningBlock(content string) (reasoning, remaining string) {
	const openTag = "[reasoning_content]\n"
	const closeTag = "\n[/reasoning_content]"
	start := strings.Index(content, openTag)
	if start == -1 {
		return "", content
	}
	afterOpen := content[start+len(openTag):]
	end := strings.Index(afterOpen, closeTag)
	if end == -1 {
		return "", content
	}
	reasoning = afterOpen[:end]
	remaining = strings.TrimSpace(content[:start] + strings.TrimPrefix(afterOpen[end+len(closeTag):], "\n\n"))
	return reasoning, remaining
}

// summarizeReasoning compresses a reasoning string if it exceeds
// reasoningSummaryThreshold. Short strings are returned unchanged.
func summarizeReasoning(text string) string {
	runes := []rune(text)
	if len(runes) <= reasoningSummaryThreshold {
		return text
	}
	omitted := len(runes) - reasoningTailChars
	tail := string(runes[len(runes)-reasoningTailChars:])
	return fmt.Sprintf("[%d chars omitted]\n%s", omitted, tail)
}

// CompileInput is the input to Compile.
type CompileInput struct {
	// Messages is a slice of normalised OpenAI-style message maps
	// (as produced by promptcompat.NormalizeOpenAIMessagesForPrompt).
	Messages []map[string]any
	// TokenBudgetHint is a soft upper-bound for total segment token cost.
	// 0 means no budget constraint (counting still happens).
	TokenBudgetHint int
	// FileDigests maps file_id → SHA-256 digest for dedup (may be nil for M1).
	FileDigests map[string]string
}

// Compile maps a normalised message sequence to a ContextPlan.
//
// M3 behaviour:
//   - Every message becomes one or two ContextSegments (assistant with
//     tool_calls yields both SegAssistant + SegToolCall).
//   - Group-based pair validation: a SegToolCall with no following
//     SegToolResult is trimmed as "orphan_tool_call"; a SegToolResult with no
//     preceding SegToolCall in the same exchange is trimmed as
//     "orphan_tool_result"; a call-count mismatch emits a soft Warning.
//   - Token budget is computed (Used) and priority trimming is applied (M3 Stage 2).
func Compile(input CompileInput) (ContextPlan, error) {
	planID := "plan_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	included := make([]ContextSegment, 0, len(input.Messages))
	trimmed := make([]TrimmedSegment, 0)
	warnings := make([]string, 0)

	// First pass: build segments.
	segs := buildSegments(input.Messages)

	// Second pass: group-based tool pair validation.
	orphanCallIdxs, orphanResultIdxs, pairWarnings := validateToolPairs(segs)
	warnings = append(warnings, pairWarnings...)

	// Build trimmed list and orphan ID set.
	orphanIDs := make(map[string]struct{}, len(orphanCallIdxs)+len(orphanResultIdxs))
	for _, idx := range orphanCallIdxs {
		seg := segs[idx]
		trimmed = append(trimmed, TrimmedSegment{
			ID:     seg.ID,
			Type:   seg.Type,
			Reason: "orphan_tool_call",
		})
		orphanIDs[seg.ID] = struct{}{}
	}
	for _, idx := range orphanResultIdxs {
		seg := segs[idx]
		trimmed = append(trimmed, TrimmedSegment{
			ID:     seg.ID,
			Type:   seg.Type,
			Reason: "orphan_tool_result",
		})
		orphanIDs[seg.ID] = struct{}{}
	}

	// Third pass: include non-orphan segments, accumulate token cost.
	totalTokens := 0
	for _, seg := range segs {
		if _, isOrphan := orphanIDs[seg.ID]; isOrphan {
			continue
		}
		included = append(included, seg)
		totalTokens += seg.TokenCost
	}

	// Fourth pass: priority budget trimming (M3 Stage 2).
	// Only runs when a budget hint is set and the total exceeds it.
	var budgetWarnings []string
	if input.TokenBudgetHint > 0 && totalTokens > input.TokenBudgetHint {
		included, trimmed, budgetWarnings = applyBudgetTrimming(included, trimmed, input.TokenBudgetHint)
		warnings = append(warnings, budgetWarnings...)
		// Recompute used tokens after trimming.
		totalTokens = 0
		for _, seg := range included {
			totalTokens += seg.TokenCost
		}
	}

	overflow := input.TokenBudgetHint > 0 && totalTokens > input.TokenBudgetHint

	return ContextPlan{
		PlanID:           planID,
		SegmentsIncluded: included,
		SegmentsTrimmed:  trimmed,
		TokenBudget: TokenBudgetReport{
			Budget:   input.TokenBudgetHint,
			Used:     totalTokens,
			Overflow: overflow,
		},
		Warnings: warnings,
	}, nil
}

// buildSegments converts a normalised message slice to a flat ContextSegment list.
func buildSegments(messages []map[string]any) []ContextSegment {
	segs := make([]ContextSegment, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		if role == "" {
			// Skip messages with missing or non-string roles
			continue
		}
		content := strings.TrimSpace(asString(msg["content"]))

		switch role {
		case "system":
			if content != "" {
				segs = append(segs, makeSegment(SegSystem, "request", content))
			}
		case "user":
			if content != "" {
				segs = append(segs, makeSegment(SegUser, "request", content))
			}
		case "assistant":
			// Always extract the inline reasoning block so the labeled text is
			// stripped from visible content (avoids double-counting when both
			// the field and the inline block are present).
			inlineReasoning, visibleContent := extractReasoningBlock(content)
			// Prefer the explicit reasoning_content field; fall back to inline block.
			rawReasoning := strings.TrimSpace(asString(msg["reasoning_content"]))
			if rawReasoning == "" {
				rawReasoning = inlineReasoning
			}
			// Emit SegReasoningSummary if reasoning is present.
			if rawReasoning != "" {
				summary := summarizeReasoning(rawReasoning)
				rseg := makeSegment(SegReasoningSummary, "request", summary)
				rawReasoningLen := len([]rune(rawReasoning))
				if rawReasoningLen > reasoningSummaryThreshold {
					rseg.Metadata = map[string]any{
						"original_reasoning_len": rawReasoningLen,
						"summarized":             true,
					}
				}
				segs = append(segs, rseg)
			}
			// Emit the visible text content segment (may be empty after reasoning extraction).
			if visibleContent != "" {
				segs = append(segs, makeSegment(SegAssistant, "request", visibleContent))
			}
			// If the assistant message carries tool_calls, emit a SegToolCall
			// and store the call count in Metadata for pair validation.
			if toolCalls := msg["tool_calls"]; toolCalls != nil {
				tcContent, callCount := marshalToolCallsWithCount(toolCalls)
				if tcContent != "" {
					seg := makeSegment(SegToolCall, "tool", tcContent)
					if callCount > 0 {
						seg.Metadata = map[string]any{"call_count": callCount}
					}
					segs = append(segs, seg)
				}
			}
		case "tool", "function":
			if content != "" {
				segs = append(segs, makeSegment(SegToolResult, "tool", content))
			}
		}
	}
	return segs
}

// validateToolPairs performs group-based tool pair validation on a segment
// slice. It returns the indices of orphaned SegToolCall and SegToolResult
// segments, plus any soft warnings (e.g. call-count mismatch).
//
// Pairing rules:
//   - A SegToolCall segment is paired with all immediately-following
//     SegToolResult segments. A call with zero results is orphaned.
//   - A SegToolResult that appears outside any call-initiated exchange
//     (i.e. no preceding SegToolCall in the same run) is orphaned.
//   - When the number of SegToolResult segments in a group differs from the
//     stored call_count (> 1), a soft warning is emitted but nothing is
//     trimmed (partial results are still useful to the model).
func validateToolPairs(segs []ContextSegment) (orphanCalls, orphanResults []int, warnings []string) {
	type group struct {
		callIdx    int
		callCount  int
		resultIdxs []int
	}

	groups := make([]group, 0)

	i := 0
	for i < len(segs) {
		switch segs[i].Type {
		case SegToolCall:
			g := group{callIdx: i, callCount: segCallCount(segs[i])}
			i++
			for i < len(segs) && segs[i].Type == SegToolResult {
				g.resultIdxs = append(g.resultIdxs, i)
				i++
			}
			groups = append(groups, g)
		case SegToolResult:
			// SegToolResult with no preceding SegToolCall in this exchange.
			orphanResults = append(orphanResults, i)
			i++
		default:
			i++
		}
	}

	for _, g := range groups {
		callSeg := segs[g.callIdx]
		if len(g.resultIdxs) == 0 {
			orphanCalls = append(orphanCalls, g.callIdx)
			warnings = append(warnings, fmt.Sprintf(
				"segment %s: orphan tool_call — no matching tool_result follows",
				callSeg.ID,
			))
		} else if g.callCount > 1 && len(g.resultIdxs) != g.callCount {
			warnings = append(warnings, fmt.Sprintf(
				"segment %s: tool_call count mismatch — expected %d result(s), got %d",
				callSeg.ID, g.callCount, len(g.resultIdxs),
			))
		}
	}

	for _, idx := range orphanResults {
		warnings = append(warnings, fmt.Sprintf(
			"segment %s: orphan tool_result — no preceding tool_call in this exchange",
			segs[idx].ID,
		))
	}

	return orphanCalls, orphanResults, warnings
}

// applyBudgetTrimming trims segments from oldest to newest until the total
// token cost is within budget. Trimming rules:
//
//   - SegSystem is pinned — never trimmed.
//   - The last SegUser in the slice is pinned — the current user request must survive.
//   - A SegToolCall and all immediately-following SegToolResult(s) are treated as
//     an indivisible unit; they are trimmed together (including any SegAssistant
//     segment that immediately precedes the SegToolCall in the same exchange).
//   - All other segments (SegAssistant, SegUser) are trimmable individually.
//
// Returns the updated included/trimmed slices and any diagnostic warnings.
func applyBudgetTrimming(
	included []ContextSegment,
	alreadyTrimmed []TrimmedSegment,
	budget int,
) (newIncluded []ContextSegment, newTrimmed []TrimmedSegment, warnings []string) {
	newTrimmed = alreadyTrimmed

	// Compute total tokens.
	total := 0
	for _, seg := range included {
		total += seg.TokenCost
	}
	if total <= budget {
		return included, newTrimmed, nil
	}

	// Find the last SegUser index (pinned).
	lastUserIdx := -1
	for i := len(included) - 1; i >= 0; i-- {
		if included[i].Type == SegUser {
			lastUserIdx = i
			break
		}
	}

	// Build indivisible trim units. Each unit is a slice of indices into included.
	// Tool exchanges: (optional SegAssistant peer) + SegToolCall + SegToolResult* → one unit.
	// Everything else: individual segment.
	type unit struct {
		idxs   []int
		cost   int
		pinned bool
	}

	units := make([]unit, 0, len(included))
	i := 0
	for i < len(included) {
		seg := included[i]
		isPinned := seg.Type == SegSystem || i == lastUserIdx

		// Detect the start of a tool exchange: look ahead for SegToolCall
		// after an optional SegAssistant text segment.
		if seg.Type == SegAssistant && i+1 < len(included) && included[i+1].Type == SegToolCall {
			// Group: SegAssistant + SegToolCall + SegToolResult*
			u := unit{idxs: []int{i, i + 1}, cost: seg.TokenCost + included[i+1].TokenCost}
			j := i + 2
			for j < len(included) && included[j].Type == SegToolResult {
				u.idxs = append(u.idxs, j)
				u.cost += included[j].TokenCost
				j++
			}
			// Pinned if any component is pinned (shouldn't happen for tool
			// exchanges but be safe).
			for _, idx := range u.idxs {
				if included[idx].Type == SegSystem || idx == lastUserIdx {
					u.pinned = true
					break
				}
			}
			units = append(units, u)
			i = j
			continue
		}

		if seg.Type == SegToolCall {
			// Bare SegToolCall (no preceding SegAssistant in this position).
			u := unit{idxs: []int{i}, cost: seg.TokenCost}
			j := i + 1
			for j < len(included) && included[j].Type == SegToolResult {
				u.idxs = append(u.idxs, j)
				u.cost += included[j].TokenCost
				j++
			}
			for _, idx := range u.idxs {
				if included[idx].Type == SegSystem || idx == lastUserIdx {
					u.pinned = true
					break
				}
			}
			units = append(units, u)
			i = j
			continue
		}

		units = append(units, unit{idxs: []int{i}, cost: seg.TokenCost, pinned: isPinned})
		i++
	}

	// Greedily trim oldest non-pinned units until within budget.
	trimSet := make(map[string]struct{})
	for _, u := range units {
		if total <= budget {
			break
		}
		if u.pinned {
			continue
		}
		for _, idx := range u.idxs {
			seg := included[idx]
			trimSet[seg.ID] = struct{}{}
			newTrimmed = append(newTrimmed, TrimmedSegment{ID: seg.ID, Type: seg.Type, Reason: "token_budget"})
			warnings = append(warnings, fmt.Sprintf(
				"segment %s (%s, %d tokens): trimmed for token budget", seg.ID, seg.Type, seg.TokenCost,
			))
		}
		total -= u.cost
	}

	// Rebuild included from non-trimmed segments preserving order.
	newIncluded = make([]ContextSegment, 0, len(included))
	for _, seg := range included {
		if _, drop := trimSet[seg.ID]; !drop {
			newIncluded = append(newIncluded, seg)
		}
	}
	return newIncluded, newTrimmed, warnings
}

// segCallCount returns the expected number of tool_call results from the
// Metadata stored by buildSegments, defaulting to 1.
func segCallCount(seg ContextSegment) int {
	if seg.Metadata == nil {
		return 1
	}
	if v, ok := seg.Metadata["call_count"]; ok {
		if n, ok := v.(int); ok && n > 0 {
			return n
		}
	}
	return 1
}

func makeSegment(segType SegmentType, source, content string) ContextSegment {
	id := "seg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	return ContextSegment{
		ID:        id,
		Type:      segType,
		Source:    source,
		Priority:  defaultPriority(segType),
		TokenCost: util.EstimateTokens(content),
		Digest:    SHA256Digest(content),
		Content:   content,
	}
}

func defaultPriority(t SegmentType) int {
	switch t {
	case SegSystem:
		return 100
	case SegUser:
		return 90
	case SegAssistant:
		return 70
	case SegToolCall:
		return 60
	case SegToolResult:
		return 60
	default:
		return 50
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// marshalArguments serialises a tool-call argument value to a string suitable
// for inclusion in segment content. When the value is already a string (the
// normal case after JSON unmarshalling into map[string]any), it is returned
// unchanged. When the value is a JSON object or array (some upstream libraries
// keep arguments as a native map rather than a pre-serialised string), it is
// compactly marshalled to JSON instead of being silently dropped or truncated.
func marshalArguments(v any) string {
	if v == nil {
		return "{}"
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// marshalToolCallsWithCount serialises tool_calls to a summary string and
// returns the number of individual calls found. Call count is used by
// validateToolPairs to detect result-count mismatches.
func marshalToolCallsWithCount(v any) (string, int) {
	switch x := v.(type) {
	case string:
		return x, 1
	case []any:
		if len(x) == 0 {
			return "", 0
		}
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				if fn, ok := m["function"].(map[string]any); ok {
					name := asString(fn["name"])
					args := marshalArguments(fn["arguments"])
					parts = append(parts, name+"("+args+")")
				}
			}
		}
		if len(parts) == 0 {
			// Items present but none have a parseable "function" key.
			// Return a sentinel so the SegToolCall is still emitted and
			// orphan detection is not silently bypassed.
			return "<unparseable_tool_calls>", len(x)
		}
		return strings.Join(parts, "; "), len(x)
	default:
		return "", 0
	}
}
