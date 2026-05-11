package toolcall

import "testing"

func TestRunShadowDiff_ModeOff_DoesNotRun(t *testing.T) {
	existing := ToolCallParseResult{Calls: []ParsedToolCall{{Name: "foo", Input: map[string]any{}}}}
	rec := RunShadowDiff("off", existing)
	if rec.Ran {
		t.Error("expected shadow diff not to run when mode=off")
	}
	if rec.HasDiff {
		t.Error("expected no diff when not run")
	}
}

func TestRunShadowDiff_ModeEnforce_DoesNotRun(t *testing.T) {
	existing := ToolCallParseResult{}
	rec := RunShadowDiff("enforce", existing)
	if rec.Ran {
		t.Error("expected shadow diff not to run when mode=enforce")
	}
}

func TestRunShadowDiff_NoDiff_ConsistentResult(t *testing.T) {
	text := `<tool_calls><invoke name="search"><parameter name="q">hello</parameter></invoke></tool_calls>`
	existing := ParseStandaloneToolCallsDetailed(text, nil)
	rec := RunShadowDiff("shadow", existing)
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
	existing := ToolCallParseResult{SourceText: ""}
	rec := RunShadowDiff("shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run")
	}
	if rec.HasDiff {
		t.Error("expected no diff for empty text")
	}
}

func TestRunShadowDiff_HasDiff_CallCountMismatch(t *testing.T) {
	text := `<tool_calls><invoke name="search"><parameter name="q">hello</parameter></invoke></tool_calls>`
	// existing claims 0 calls but SourceText contains a tool call — simulates
	// a future parser divergence where old parser missed the call.
	existing := ToolCallParseResult{
		Calls:             []ParsedToolCall{},
		SawToolCallSyntax: false,
		SourceText:        text,
	}
	rec := RunShadowDiff("shadow", existing)
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
		SourceText:        text,
	}
	rec := RunShadowDiff("shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run")
	}
	if !rec.HasDiff {
		t.Errorf("expected diff when SawToolCallSyntax mismatches, record=%+v", rec)
	}
}

// TestRunShadowDiff_UsesSourceText_NotRawText verifies Bug #1 fix:
// RunShadowDiff uses existing.SourceText, so tool calls found in the thinking
// block (different source than rawText) do not produce false diffs.
func TestRunShadowDiff_UsesSourceText_NotRawText(t *testing.T) {
	thinkingText := `<tool_calls><invoke name="search"><parameter name="q">hello</parameter></invoke></tool_calls>`
	// existing was parsed from thinkingText, not from rawText
	existing := ParseStandaloneToolCallsDetailed(thinkingText, nil)
	if existing.SourceText != thinkingText {
		t.Fatalf("expected SourceText to be set by ParseStandaloneToolCallsDetailed, got %q", existing.SourceText)
	}
	// Running shadow diff: candidate re-runs on existing.SourceText (thinkingText),
	// so result should match — no false diff.
	rec := RunShadowDiff("shadow", existing)
	if !rec.Ran {
		t.Error("expected shadow diff to run")
	}
	if rec.HasDiff {
		t.Errorf("expected no diff when SourceText is used correctly, got record=%+v", rec)
	}
}

// TestRunShadowDiff_NoDiff_PreNormalization verifies Bug #2 fix:
// shadow diff should compare pre-normalization results. If existing.Calls
// contains raw values (before schema normalization), shadow diff should
// find no diff since buildParseCandidate also returns raw values.
func TestRunShadowDiff_NoDiff_PreNormalization(t *testing.T) {
	text := `<tool_calls><invoke name="write"><parameter name="content">hello</parameter></invoke></tool_calls>`
	// pre-normalization result (as returned by DetectAssistantToolCalls)
	existing := ParseStandaloneToolCallsDetailed(text, nil)
	rec := RunShadowDiff("shadow", existing)
	if rec.HasDiff {
		t.Errorf("expected no diff for pre-normalization comparison, got record=%+v", rec)
	}
}
