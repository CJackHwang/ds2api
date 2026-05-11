package contextengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contextFixture mirrors tests/compat/fixtures/context/*.json
type contextFixture struct {
	Description             string           `json:"description"`
	Messages                []map[string]any `json:"messages"`
	HistoryText             string           `json:"history_text"`
	FullHistoryLen          int              `json:"full_history_len"`
	FileRefs                []string         `json:"file_refs"`
	ExpectedIssues          []string         `json:"expected_issues"`
	ExpectedWarningsContain []string         `json:"expected_warnings_contain"`
	TokenBudgetHint         int              `json:"token_budget_hint"`
}

func loadFixture(t *testing.T, name string) contextFixture {
	t.Helper()
	// Walk up from package directory to repo root, then into tests/compat/fixtures/context/
	path := filepath.Join("..", "..", "tests", "compat", "fixtures", "context", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	var fix contextFixture
	if err := json.Unmarshal(data, &fix); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return fix
}

func TestCompilePlainMultiturn(t *testing.T) {
	fix := loadFixture(t, "plain_multiturn.json")

	plan, err := Compile(CompileInput{Messages: fix.Messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	if plan.PlanID == "" {
		t.Error("expected non-empty PlanID")
	}
	if len(plan.SegmentsIncluded) == 0 {
		t.Error("expected at least one segment included")
	}
	if len(plan.SegmentsTrimmed) != 0 {
		t.Errorf("expected no trimmed segments for plain_multiturn, got %d", len(plan.SegmentsTrimmed))
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("expected no warnings for plain_multiturn, got %v", plan.Warnings)
	}
	if plan.TokenBudget.Used <= 0 {
		t.Error("expected TokenBudget.Used > 0")
	}
}

func TestCompileToolLoopRead(t *testing.T) {
	fix := loadFixture(t, "tool_loop_read.json")

	plan, err := Compile(CompileInput{Messages: fix.Messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	if len(plan.SegmentsTrimmed) != 0 {
		t.Errorf("expected no trimmed segments for complete tool loop, got %d", len(plan.SegmentsTrimmed))
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("expected no warnings for complete tool loop, got %v", plan.Warnings)
	}

	// Check that both SegToolCall and SegToolResult are present.
	types := make(map[SegmentType]int)
	for _, seg := range plan.SegmentsIncluded {
		types[seg.Type]++
	}
	if types[SegToolCall] == 0 {
		t.Error("expected at least one SegToolCall segment")
	}
	if types[SegToolResult] == 0 {
		t.Error("expected at least one SegToolResult segment")
	}
}

func TestCompileOrphanToolCall(t *testing.T) {
	fix := loadFixture(t, "orphan_tool_call.json")

	plan, err := Compile(CompileInput{Messages: fix.Messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	if len(plan.Warnings) == 0 {
		t.Error("expected at least one warning for orphan_tool_call fixture")
	}
	if len(plan.SegmentsTrimmed) == 0 {
		t.Error("expected at least one trimmed segment for orphan_tool_call fixture")
	}

	// Verify the fixture's expected_issues are reflected.
	for _, issue := range fix.ExpectedIssues {
		found := false
		for _, ts := range plan.SegmentsTrimmed {
			if ts.Reason == issue {
				found = true
				break
			}
		}
		if !found {
			// Also check warnings.
			for _, w := range plan.Warnings {
				if strings.Contains(w, issue) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("expected issue %q not found in trimmed segments or warnings", issue)
		}
	}

	// Trimmed segment IDs must NOT appear in SegmentsIncluded.
	trimmedIDs := make(map[string]struct{}, len(plan.SegmentsTrimmed))
	for _, ts := range plan.SegmentsTrimmed {
		trimmedIDs[ts.ID] = struct{}{}
	}
	for _, seg := range plan.SegmentsIncluded {
		if _, found := trimmedIDs[seg.ID]; found {
			t.Errorf("trimmed segment %s (type %s) should not be in SegmentsIncluded", seg.ID, seg.Type)
		}
	}
}

func TestCompileOrphanToolResult(t *testing.T) {
	fix := loadFixture(t, "orphan_tool_result.json")

	plan, err := Compile(CompileInput{Messages: fix.Messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	if len(plan.Warnings) == 0 {
		t.Error("expected at least one warning for orphan_tool_result fixture")
	}

	found := false
	for _, ts := range plan.SegmentsTrimmed {
		if ts.Reason == "orphan_tool_result" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a TrimmedSegment with reason 'orphan_tool_result'")
	}

	for _, seg := range plan.SegmentsIncluded {
		if seg.Type == SegToolResult {
			t.Errorf("orphaned SegToolResult should not be in SegmentsIncluded, found id=%s", seg.ID)
		}
	}
}

func TestCompileMultiOrphanTools(t *testing.T) {
	fix := loadFixture(t, "multi_orphan_tools.json")

	plan, err := Compile(CompileInput{Messages: fix.Messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	reasons := make(map[string]bool)
	for _, ts := range plan.SegmentsTrimmed {
		reasons[ts.Reason] = true
	}
	for _, issue := range fix.ExpectedIssues {
		if !reasons[issue] {
			t.Errorf("expected trimmed reason %q not found; got reasons: %v", issue, reasons)
		}
	}

	for _, seg := range plan.SegmentsIncluded {
		if seg.Type == SegToolCall || seg.Type == SegToolResult {
			t.Errorf("orphaned tool segment should not be in SegmentsIncluded: id=%s type=%s", seg.ID, seg.Type)
		}
	}
}

func TestCompileMultiCallCountMismatch(t *testing.T) {
	fix := loadFixture(t, "multi_call_count_mismatch.json")

	plan, err := Compile(CompileInput{Messages: fix.Messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	if len(plan.SegmentsTrimmed) != 0 {
		t.Errorf("expected no trimmed segments for partial-result fixture (partial results retained), got %d", len(plan.SegmentsTrimmed))
	}

	for _, wantSubstr := range fix.ExpectedWarningsContain {
		found := false
		for _, w := range plan.Warnings {
			if strings.Contains(w, wantSubstr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning containing %q not found in: %v", wantSubstr, plan.Warnings)
		}
	}

	types := make(map[SegmentType]int)
	for _, seg := range plan.SegmentsIncluded {
		types[seg.Type]++
	}
	if types[SegToolCall] == 0 {
		t.Error("expected SegToolCall to be retained for partial-result scenario")
	}
	if types[SegToolResult] == 0 {
		t.Error("expected SegToolResult to be retained for partial-result scenario")
	}
}

func TestCompileLongHistoryTokenBudget(t *testing.T) {
	fix := loadFixture(t, "long_history_token_budget.json")

	plan, err := Compile(CompileInput{
		Messages:        fix.Messages,
		TokenBudgetHint: fix.TokenBudgetHint,
	})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	if plan.TokenBudget.Used <= 0 {
		t.Error("expected TokenBudget.Used > 0 for long history fixture")
	}
	// PlanID must be stable and non-empty.
	if plan.PlanID == "" {
		t.Error("expected non-empty PlanID")
	}
	// Budget must be propagated from hint.
	if fix.TokenBudgetHint > 0 && plan.TokenBudget.Budget != fix.TokenBudgetHint {
		t.Errorf("TokenBudget.Budget = %d, want %d", plan.TokenBudget.Budget, fix.TokenBudgetHint)
	}
}

func TestDigestDeterministic(t *testing.T) {
	d1 := SHA256Digest("hello world")
	d2 := SHA256Digest("hello world")
	if d1 != d2 {
		t.Errorf("SHA256Digest not deterministic: %q != %q", d1, d2)
	}
	if len(d1) != 64 {
		t.Errorf("expected 64-char hex digest, got %d chars", len(d1))
	}
	d3 := SHA256Digest("different")
	if d1 == d3 {
		t.Error("different inputs produced same digest")
	}
}

func TestCompileSkipsMessagesWithNonStringRole(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "valid message"},
		{"role": 123, "content": "invalid role - integer"}, // non-string role
		{"role": nil, "content": "invalid role - nil"},     // nil role
		{"role": "system", "content": "another valid"},
	}

	plan, err := Compile(CompileInput{Messages: messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// Should only have 2 segments (user and system), the invalid ones are skipped
	if len(plan.SegmentsIncluded) != 2 {
		t.Errorf("expected 2 segments after skipping invalid roles, got %d", len(plan.SegmentsIncluded))
	}
}

func TestCompileSkipsEmptyToolCalls(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "hi", "tool_calls": []any{}}, // empty array
		{"role": "assistant", "content": "bye", "tool_calls": ""},     // empty string
	}

	plan, err := Compile(CompileInput{Messages: messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// Should have user + 2 assistant text segments, but NO SegToolCall segments
	toolCallCount := 0
	for _, seg := range plan.SegmentsIncluded {
		if seg.Type == SegToolCall {
			toolCallCount++
		}
	}
	if toolCallCount != 0 {
		t.Errorf("expected no SegToolCall segments for empty tool_calls, got %d", toolCallCount)
	}
}

func TestCompileHandlesMalformedToolCalls(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "response", "tool_calls": []any{
			map[string]any{"invalid": "structure"}, // missing "function" key
		}},
	}

	plan, err := Compile(CompileInput{Messages: messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// Malformed tool_calls (non-empty array, but no parseable "function" key) should still
	// emit a SegToolCall (with sentinel content) so orphan detection fires.
	// Since there is no following tool result, it must be orphaned → in SegmentsTrimmed,
	// not in SegmentsIncluded.
	for _, seg := range plan.SegmentsIncluded {
		if seg.Type == SegToolCall {
			t.Errorf("orphaned malformed SegToolCall should not be in SegmentsIncluded, but found id=%s", seg.ID)
		}
	}
	found := false
	for _, ts := range plan.SegmentsTrimmed {
		if ts.Type == SegToolCall && ts.Reason == "orphan_tool_call" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected malformed tool_calls to produce an orphaned SegToolCall in SegmentsTrimmed")
	}
}

func TestCompileHandlesMissingFields(t *testing.T) {
	messages := []map[string]any{
		{"role": "user"},      // missing content
		{"content": "hello"},  // missing role
		{"role": "assistant"}, // missing content and tool_calls
		{"role": "tool"},      // missing content
		{"role": "function"},  // missing content
	}

	plan, err := Compile(CompileInput{Messages: messages})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// All messages have empty/missing content or missing role: no segments expected.
	if len(plan.SegmentsIncluded) != 0 {
		t.Errorf("expected 0 segments for empty/missing content and roles, got %d", len(plan.SegmentsIncluded))
	}
}
