package contextengine

import (
	"fmt"
	"strings"

	"ds2api/internal/util"

	"github.com/google/uuid"
)

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
// M1 behaviour:
//   - Every message becomes one or two ContextSegments (assistant with
//     tool_calls yields both SegAssistant + SegToolCall).
//   - Orphan tool_call detection: an assistant segment whose tool_calls have
//     no matching subsequent tool-result segment is added to SegmentsTrimmed
//     with reason "orphan_tool_call", and a Warning is emitted.
//   - Token budget is computed (Used) but no trimming is performed yet (M3).
func Compile(input CompileInput) (ContextPlan, error) {
	planID := "plan_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	included := make([]ContextSegment, 0, len(input.Messages))
	trimmed := make([]TrimmedSegment, 0)
	warnings := make([]string, 0)

	// First pass: build segments.
	segs := buildSegments(input.Messages)

	// Second pass: orphan detection.
	// An assistant segment that carries tool_calls is orphaned if the very
	// next segment is NOT a tool_result (or there is no next segment at all).
	for i, seg := range segs {
		if seg.Type != SegToolCall {
			continue
		}
		// A tool_call is paired when the immediately-following segment is a tool_result.
		next := i + 1
		paired := next < len(segs) && segs[next].Type == SegToolResult
		if !paired {
			trimmed = append(trimmed, TrimmedSegment{
				ID:     seg.ID,
				Type:   seg.Type,
				Reason: "orphan_tool_call",
			})
			warnings = append(warnings, fmt.Sprintf("segment %s: orphan tool_call — no matching tool_result follows", seg.ID))
			if i > 0 && segs[i-1].Type == SegAssistant {
				warnings = append(warnings, fmt.Sprintf("segment %s: companion SegAssistant retained for orphaned SegToolCall %s", segs[i-1].ID, seg.ID))
			}
		}
	}

	// Build orphan ID set for exclusion.
	orphanIDs := make(map[string]struct{}, len(trimmed))
	for _, t := range trimmed {
		orphanIDs[t.ID] = struct{}{}
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
			// Emit the text content segment first (may be empty).
			if content != "" {
				segs = append(segs, makeSegment(SegAssistant, "request", content))
			}
			// If the assistant message carries tool_calls, emit a SegToolCall.
			if toolCalls := msg["tool_calls"]; toolCalls != nil {
				tcContent := marshalToolCalls(toolCalls)
				if tcContent != "" {
					segs = append(segs, makeSegment(SegToolCall, "tool", tcContent))
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

func marshalToolCalls(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) == 0 {
			return ""
		}
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				if fn, ok := m["function"].(map[string]any); ok {
					name := asString(fn["name"])
					args := asString(fn["arguments"])
					parts = append(parts, name+"("+args+")")
				}
			}
		}
		if len(parts) == 0 {
			// Items present but none have a parseable "function" key.
			// Return a sentinel so the SegToolCall is still emitted and
			// orphan detection is not silently bypassed.
			return "<unparseable_tool_calls>"
		}
		return strings.Join(parts, "; ")
	default:
		return ""
	}
}
