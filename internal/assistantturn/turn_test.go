package assistantturn

import (
	"net/http"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestBuildTurnFromCollectedTextCitation(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{
		Text:          "See [citation:1]",
		CitationLinks: map[int]string{1: "https://example.com"},
	}, BuildOptions{Model: "deepseek-v4-flash", Prompt: "prompt", SearchEnabled: true})
	if turn.Text != "See [1](https://example.com)" {
		t.Fatalf("text mismatch: %q", turn.Text)
	}
	if turn.StopReason != StopReasonStop {
		t.Fatalf("stop reason mismatch: %q", turn.StopReason)
	}
	if turn.Error != nil {
		t.Fatalf("unexpected error: %#v", turn.Error)
	}
}

func TestBuildTurnFromCollectedKeepsNonStreamReferenceLinks(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{
		Text: "结论[reference:0]，补充[reference:1]。",
		CitationLinks: map[int]string{
			1: "https://example.com/a",
			2: "https://example.com/b",
		},
	}, BuildOptions{Model: "deepseek-v4-flash-search", Prompt: "prompt", SearchEnabled: true})
	want := "结论[0](https://example.com/a)，补充[1](https://example.com/b)。"
	if turn.Text != want {
		t.Fatalf("text mismatch: got %q want %q", turn.Text, want)
	}
}

func TestBuildTurnFromCollectedToolCall(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{
		Text: `<tool_calls><invoke name="Write"><parameter name="content">{"x":1}</parameter></invoke></tool_calls>`,
	}, BuildOptions{
		ToolNames: []string{"Write"},
		ToolsRaw: []any{map[string]any{
			"name": "Write",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string"},
				},
			},
		}},
	})
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(turn.ToolCalls))
	}
	if turn.StopReason != StopReasonToolCalls {
		t.Fatalf("stop reason mismatch: %q", turn.StopReason)
	}
	if _, ok := turn.ToolCalls[0].Input["content"].(string); !ok {
		t.Fatalf("expected content coerced to string, got %#v", turn.ToolCalls[0].Input["content"])
	}
}

func TestBuildTurnFromCollectedThinkingOnlyIsEmptyOutput(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{Thinking: "hidden"}, BuildOptions{})
	if turn.Error == nil || turn.Error.Code != "upstream_empty_output" {
		t.Fatalf("expected empty output error, got %#v", turn.Error)
	}
}

func TestBuildTurnFromCollectedPureEmptyOutputIsUpstreamUnavailable(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{}, BuildOptions{})
	if turn.Error == nil || turn.Error.Status != http.StatusServiceUnavailable || turn.Error.Code != "upstream_unavailable" {
		t.Fatalf("expected upstream unavailable error, got %#v", turn.Error)
	}
}

func TestBuildTurnFromCollectedToolChoiceRequired(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{Text: "hello"}, BuildOptions{
		ToolChoice: promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired},
	})
	if turn.Error == nil || turn.Error.Code != "tool_choice_violation" {
		t.Fatalf("expected tool choice violation, got %#v", turn.Error)
	}
}

func TestBuildTurnFromStreamSnapshotUsesVisibleTextAndRawToolDetection(t *testing.T) {
	turn := BuildTurnFromStreamSnapshot(StreamSnapshot{
		RawText:     `<tool_calls><invoke name="Write"><parameter name="content">{"x":1}</parameter></invoke></tool_calls>`,
		VisibleText: "",
	}, BuildOptions{
		ToolNames: []string{"Write"},
		ToolsRaw: []any{map[string]any{
			"name": "Write",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string"},
				},
			},
		}},
	})
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected stream snapshot tool call, got %d", len(turn.ToolCalls))
	}
	if _, ok := turn.ToolCalls[0].Input["content"].(string); !ok {
		t.Fatalf("expected stream snapshot schema coercion, got %#v", turn.ToolCalls[0].Input["content"])
	}
}

func TestBuildTurnFromStreamSnapshotAlreadyEmittedToolAvoidsEmptyError(t *testing.T) {
	turn := BuildTurnFromStreamSnapshot(StreamSnapshot{AlreadyEmittedCalls: true}, BuildOptions{})
	if turn.Error != nil {
		t.Fatalf("unexpected empty-output error after emitted tool call: %#v", turn.Error)
	}
	if turn.StopReason != StopReasonToolCalls {
		t.Fatalf("stop reason mismatch: %q", turn.StopReason)
	}
}

func TestFinalizeTurnStopOutcome(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{Text: "hello"}, BuildOptions{})
	outcome := FinalizeTurn(turn, FinalizeOptions{})
	if outcome.ShouldFail {
		t.Fatalf("unexpected failure: %#v", outcome.Error)
	}
	if outcome.FinishReason != "stop" || !outcome.HasVisibleText || !outcome.HasVisibleOutput {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestFinalizeTurnToolCallsOutcome(t *testing.T) {
	turn := BuildTurnFromStreamSnapshot(StreamSnapshot{AlreadyEmittedCalls: true}, BuildOptions{})
	outcome := FinalizeTurn(turn, FinalizeOptions{AlreadyEmittedToolCalls: true})
	if outcome.ShouldFail || outcome.FinishReason != "tool_calls" || !outcome.HasToolCalls {
		t.Fatalf("unexpected tool outcome: %#v", outcome)
	}
}

func TestFinalizeTurnContentFilterOutcome(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{ContentFilter: true}, BuildOptions{})
	outcome := FinalizeTurn(turn, FinalizeOptions{})
	if !outcome.ShouldFail || outcome.Error == nil || outcome.Error.Code != "content_filter" {
		t.Fatalf("expected content filter failure, got %#v", outcome)
	}
}
