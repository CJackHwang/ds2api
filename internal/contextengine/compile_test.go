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
	Description     string           `json:"description"`
	Messages        []map[string]any `json:"messages"`
	HistoryText     string           `json:"history_text"`
	FullHistoryLen  int              `json:"full_history_len"`
	FileRefs        []string         `json:"file_refs"`
	ExpectedIssues  []string         `json:"expected_issues"`
	TokenBudgetHint int              `json:"token_budget_hint"`
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

	// Orphan segment must NOT appear in SegmentsIncluded.
	for _, seg := range plan.SegmentsIncluded {
		if seg.Type == SegToolCall {
			t.Errorf("orphan SegToolCall should not be in SegmentsIncluded, but found seg %s", seg.ID)
		}
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
