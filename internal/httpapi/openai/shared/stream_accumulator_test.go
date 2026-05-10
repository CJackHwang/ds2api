package shared

import (
	"testing"

	"ds2api/internal/sse"
)

func TestStreamAccumulatorAppliesThinkingAndTextDedupe(t *testing.T) {
	acc := StreamAccumulator{ThinkingEnabled: true, StripReferenceMarkers: true}
	thinkingPrefix := "this is a long thinking snapshot prefix used by DeepSeek continue replay"
	textPrefix := "this is a long visible answer snapshot prefix used by DeepSeek continue replay"
	first := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts: []sse.ContentPart{
			{Type: "thinking", Text: thinkingPrefix},
			{Type: "text", Text: textPrefix},
		},
	})
	second := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts: []sse.ContentPart{
			{Type: "thinking", Text: thinkingPrefix + " next"},
			{Type: "text", Text: textPrefix + " world"},
		},
	})

	if !first.ContentSeen || !second.ContentSeen {
		t.Fatalf("expected both chunks to mark content seen")
	}
	if got := acc.RawThinking.String(); got != thinkingPrefix+" next" {
		t.Fatalf("raw thinking = %q", got)
	}
	if got := acc.Thinking.String(); got != thinkingPrefix+" next" {
		t.Fatalf("thinking = %q", got)
	}
	if got := acc.RawText.String(); got != textPrefix+" world" {
		t.Fatalf("raw text = %q", got)
	}
	if got := acc.Text.String(); got != textPrefix+" world" {
		t.Fatalf("text = %q", got)
	}
	if got := second.Parts[0].VisibleText; got != " next" {
		t.Fatalf("thinking delta = %q", got)
	}
	if got := second.Parts[1].VisibleText; got != " world" {
		t.Fatalf("text delta = %q", got)
	}
}

func TestStreamAccumulatorKeepsHiddenThinkingForToolDetection(t *testing.T) {
	acc := StreamAccumulator{ThinkingEnabled: false, StripReferenceMarkers: true}
	result := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts: []sse.ContentPart{
			{Type: "thinking", Text: "<tool_calls></tool_calls>"},
		},
		ToolDetectionThinkingParts: []sse.ContentPart{
			{Type: "thinking", Text: "detect"},
			{Type: "thinking", Text: " tools"},
		},
	})

	if !result.ContentSeen {
		t.Fatalf("expected hidden thinking to count as upstream content")
	}
	if got := acc.RawThinking.String(); got != "<tool_calls></tool_calls>" {
		t.Fatalf("raw thinking = %q", got)
	}
	if got := acc.Thinking.String(); got != "" {
		t.Fatalf("visible thinking = %q", got)
	}
	if got := acc.ToolDetectionThinking.String(); got != "detect tools" {
		t.Fatalf("tool detection thinking = %q", got)
	}
}

func TestStreamAccumulatorSuppressesCitationTextWhenSearchEnabled(t *testing.T) {
	acc := StreamAccumulator{SearchEnabled: true, StripReferenceMarkers: true}
	result := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: "[citation:1]"}},
	})

	if !result.ContentSeen {
		t.Fatalf("expected citation chunk to mark upstream content")
	}
	if len(result.Parts) != 1 || !result.Parts[0].CitationOnly {
		t.Fatalf("expected citation-only delta, got %#v", result.Parts)
	}
	if got := acc.RawText.String(); got != "[citation:1]" {
		t.Fatalf("raw text = %q", got)
	}
	if got := acc.Text.String(); got != "" {
		t.Fatalf("visible text = %q", got)
	}
}

func TestStreamAccumulatorStripsToolResultSectionAcrossTextChunks(t *testing.T) {
	acc := StreamAccumulator{StripReferenceMarkers: true}
	first := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: "visible:<|Tool|>"}},
	})
	second := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: `[{"content":"secret","tool_call_id":"call_123"}]`}},
	})
	third := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: "<|end_of_toolresults|> after"}},
	})

	if got := acc.RawText.String(); got != `visible:<|Tool|>[{"content":"secret","tool_call_id":"call_123"}]<|end_of_toolresults|> after` {
		t.Fatalf("raw text = %q", got)
	}
	if got := acc.Text.String(); got != "visible: after" {
		t.Fatalf("visible text = %q", got)
	}
	if !first.ContentSeen || !second.ContentSeen || !third.ContentSeen {
		t.Fatalf("expected all chunks to mark upstream content")
	}
	if got := first.Parts[0].VisibleText; got != "visible:" {
		t.Fatalf("first visible delta = %q", got)
	}
	if got := second.Parts[0].VisibleText; got != "" {
		t.Fatalf("payload visible delta = %q", got)
	}
	if got := third.Parts[0].VisibleText; got != " after" {
		t.Fatalf("closing visible delta = %q", got)
	}
}

func TestStreamAccumulatorStripsFullwidthToolResultSectionAcrossTextChunks(t *testing.T) {
	acc := StreamAccumulator{StripReferenceMarkers: true}
	acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: "x<｜Tool｜>"}},
	})
	acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: `{"content":"secret"}`}},
	})
	acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: "<｜end▁of▁toolresults｜>y"}},
	})

	if got := acc.Text.String(); got != "xy" {
		t.Fatalf("visible text = %q", got)
	}
}

func TestStreamAccumulatorStripsToolResultSectionAcrossThinkingChunks(t *testing.T) {
	acc := StreamAccumulator{ThinkingEnabled: true, StripReferenceMarkers: true}
	acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "thinking", Text: "thought <|Tool|>"}},
	})
	payload := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "thinking", Text: `[{"content":"secret"}]`}},
	})
	acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "thinking", Text: "<|end_of_toolresults|>resumes"}},
	})

	if got := acc.RawThinking.String(); got != `thought <|Tool|>[{"content":"secret"}]<|end_of_toolresults|>resumes` {
		t.Fatalf("raw thinking = %q", got)
	}
	if got := acc.Thinking.String(); got != "thought resumes" {
		t.Fatalf("visible thinking = %q", got)
	}
	if got := payload.Parts[0].VisibleText; got != "" {
		t.Fatalf("payload visible delta = %q", got)
	}
}

func TestStreamAccumulatorStripsInlineCitationAndReferenceMarkers(t *testing.T) {
	acc := StreamAccumulator{SearchEnabled: true, StripReferenceMarkers: true}
	result := acc.Apply(sse.LineResult{
		Parsed: true,
		Parts:  []sse.ContentPart{{Type: "text", Text: "广州天气[citation:1] 多云[reference:0]"}},
	})

	if !result.ContentSeen {
		t.Fatalf("expected marker chunk to mark upstream content")
	}
	if got := acc.Text.String(); got != "广州天气 多云" {
		t.Fatalf("visible text = %q", got)
	}
	if len(result.Parts) != 1 || result.Parts[0].VisibleText != "广州天气 多云" {
		t.Fatalf("unexpected parts: %#v", result.Parts)
	}
}
