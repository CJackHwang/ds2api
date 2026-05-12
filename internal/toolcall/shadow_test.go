package toolcall

import (
	"testing"
)

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

// --- ClassifyConfidence tests ---

func TestClassifyConfidence_High_XMLDirectWithWhitelist(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:             true,
		NewSawSyntax:    true,
		NewParsePath:    parsePathXMLDirect,
		NewAmbiguous:    false,
		NewWhitelistHit: true,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceHigh {
		t.Errorf("expected ConfidenceHigh for xml_direct+whitelist, got %s", got)
	}
}

func TestClassifyConfidence_Medium_XMLDirectNoWhitelist(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:             true,
		NewSawSyntax:    true,
		NewParsePath:    parsePathXMLDirect,
		NewAmbiguous:    false,
		NewWhitelistHit: false,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceMedium {
		t.Errorf("expected ConfidenceMedium for xml_direct without whitelist hit, got %s", got)
	}
}

func TestClassifyConfidence_Medium_CDATARecover(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:             true,
		NewSawSyntax:    true,
		NewParsePath:    parsePathXMLCDATARecover,
		NewAmbiguous:    false,
		NewWhitelistHit: true,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceMedium {
		t.Errorf("expected ConfidenceMedium for xml_cdata_recover, got %s", got)
	}
}

func TestClassifyConfidence_Low_NoSyntax(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:          true,
		NewSawSyntax: false,
		NewParsePath: parsePathEmpty,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceLow {
		t.Errorf("expected ConfidenceLow when no syntax seen, got %s", got)
	}
}

func TestClassifyConfidence_Low_Ambiguous(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:             true,
		NewSawSyntax:    true,
		NewParsePath:    parsePathXMLDirect,
		NewAmbiguous:    true,
		NewWhitelistHit: true,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceLow {
		t.Errorf("expected ConfidenceLow for ambiguous input, got %s", got)
	}
}

func TestClassifyConfidence_Low_NormalizeFailed(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:          true,
		NewSawSyntax: true,
		NewParsePath: parsePathNormalizeFailed,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceLow {
		t.Errorf("expected ConfidenceLow for normalize_failed, got %s", got)
	}
}

func TestClassifyConfidence_Low_XMLFailed(t *testing.T) {
	rec := ShadowDiffRecord{
		Ran:          true,
		NewSawSyntax: true,
		NewParsePath: parsePathXMLFailed,
	}
	got := ClassifyConfidence(rec)
	if got != ConfidenceLow {
		t.Errorf("expected ConfidenceLow for xml_parse_failed, got %s", got)
	}
}

// TestRunShadowDiff_ConfidenceSignalsPropagated verifies that confidence
// signals (NewParsePath, NewAmbiguous, NewWhitelistHit) are correctly
// populated in the returned ShadowDiffRecord.
func TestRunShadowDiff_ConfidenceSignalsPropagated(t *testing.T) {
	text := `<tool_calls><invoke name="Bash"><parameter name="command">git status</parameter></invoke></tool_calls>`
	existing := ParseStandaloneToolCallsDetailed(text, []string{"Bash"})
	rec := RunShadowDiff("shadow", existing)

	if !rec.Ran {
		t.Fatal("expected shadow diff to run")
	}
	if rec.NewParsePath != parsePathXMLDirect {
		t.Errorf("expected NewParsePath=%q, got %q", parsePathXMLDirect, rec.NewParsePath)
	}
	if rec.NewAmbiguous {
		t.Error("expected NewAmbiguous=false for unambiguous canonical XML")
	}
	if !rec.NewWhitelistHit {
		t.Error("expected NewWhitelistHit=true when 'Bash' is in available names")
	}
	if c := ClassifyConfidence(rec); c != ConfidenceHigh {
		t.Errorf("expected ConfidenceHigh for xml_direct+whitelist, got %s", c)
	}
}

func TestRunShadowDiff_ConfidenceMedium_CDATARecoverPath(t *testing.T) {
	// Loose CDATA (unclosed section) triggers the CDATA recovery path → ConfidenceMedium.
	text := `<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[hello</parameter></invoke></tool_calls>`
	existing := ParseStandaloneToolCallsDetailed(text, []string{"Write"})
	rec := RunShadowDiff("shadow", existing)
	if !rec.Ran {
		t.Fatal("expected shadow diff to run")
	}
	if rec.NewParsePath != parsePathXMLCDATARecover {
		t.Errorf("expected NewParsePath=%q for unclosed CDATA, got %q", parsePathXMLCDATARecover, rec.NewParsePath)
	}
	if c := ClassifyConfidence(rec); c != ConfidenceMedium {
		t.Errorf("expected ConfidenceMedium for xml_cdata_recover path, got %s", c)
	}
}

// TestConfidenceString verifies the String() representation of each level.
func TestConfidenceString(t *testing.T) {
	cases := []struct {
		c    ParseConfidence
		want string
	}{
		{ConfidenceHigh, "high"},
		{ConfidenceMedium, "medium"},
		{ConfidenceLow, "low"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("ParseConfidence(%d).String() = %q, want %q", tc.c, got, tc.want)
		}
	}
}
