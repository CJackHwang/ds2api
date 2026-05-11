package toolcall

import "testing"

func TestRunShadowDiff_ModeOff_DoesNotRun(t *testing.T) {
	existing := ToolCallParseResult{Calls: []ParsedToolCall{{Name: "foo", Input: map[string]any{}}}}
	rec := RunShadowDiff("anything", "off", existing)
	if rec.Ran {
		t.Error("expected shadow diff not to run when mode=off")
	}
	if rec.HasDiff {
		t.Error("expected no diff when not run")
	}
}

func TestRunShadowDiff_ModeEnforce_DoesNotRun(t *testing.T) {
	existing := ToolCallParseResult{}
	rec := RunShadowDiff("anything", "enforce", existing)
	if rec.Ran {
		t.Error("expected shadow diff not to run when mode=enforce")
	}
}

func TestRunShadowDiff_NoDiff_ConsistentResult(t *testing.T) {
	text := `<tool_calls><invoke name="search"><parameter name="q">hello</parameter></invoke></tool_calls>`
	existing := ParseStandaloneToolCallsDetailed(text, nil)
	rec := RunShadowDiff(text, "shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run in shadow mode")
	}
	if rec.HasDiff {
		t.Errorf("expected no diff for consistent result, got record=%+v", rec)
	}
	if rec.OldCallCount != 1 || rec.NewCallCount != 1 {
		t.Errorf("expected 1 call each, got old=%d new=%d", rec.OldCallCount, rec.NewCallCount)
	}
}

func TestRunShadowDiff_NoDiff_EmptyText(t *testing.T) {
	existing := ToolCallParseResult{}
	rec := RunShadowDiff("", "shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run")
	}
	if rec.HasDiff {
		t.Error("expected no diff for empty text")
	}
}

func TestRunShadowDiff_HasDiff_CallCountMismatch(t *testing.T) {
	text := `<tool_calls><invoke name="search"><parameter name="q">hello</parameter></invoke></tool_calls>`
	existing := ToolCallParseResult{
		Calls:             []ParsedToolCall{},
		SawToolCallSyntax: false,
	}
	rec := RunShadowDiff(text, "shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run")
	}
	if !rec.HasDiff {
		t.Errorf("expected diff when existing has 0 calls but candidate finds 1, record=%+v", rec)
	}
	if rec.OldCallCount != 0 {
		t.Errorf("expected old call count 0, got %d", rec.OldCallCount)
	}
	if rec.NewCallCount != 1 {
		t.Errorf("expected new call count 1, got %d", rec.NewCallCount)
	}
}

func TestRunShadowDiff_HasDiff_SyntaxMismatch(t *testing.T) {
	text := `<tool_calls><invoke name="search"><parameter name="q">test</parameter></invoke></tool_calls>`
	existing := ToolCallParseResult{
		Calls:             []ParsedToolCall{{Name: "search", Input: map[string]any{"q": "test"}}},
		SawToolCallSyntax: false,
	}
	rec := RunShadowDiff(text, "shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run")
	}
	if !rec.HasDiff {
		t.Errorf("expected diff when SawToolCallSyntax mismatches, record=%+v", rec)
	}
}
