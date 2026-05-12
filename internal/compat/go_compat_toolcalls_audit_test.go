package compat

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ds2api/internal/toolcall"
)

// TestToolCallFixtureSemanticAudit performs independent semantic validation of
// the expected output files for each toolcall fixture.  Unlike the existing
// parity test (which checks "parser output == expected file"), this test uses
// structural heuristics to verify that the expected files are *semantically
// correct* — i.e. the expected outcome matches the fixture's intent.
func TestToolCallFixtureSemanticAudit(t *testing.T) {
	root := compatPath("fixtures", "toolcalls")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		rel, _ := filepath.Rel(root, path)
		expectedName := "toolcalls_" + strings.ReplaceAll(strings.TrimSuffix(rel, ".json"), string(filepath.Separator), "_") + ".json"
		expectedPath := compatPath("expected", expectedName)

		name := strings.TrimSuffix(rel, ".json")
		t.Run(name, func(t *testing.T) {
			var fix toolCallFixture
			mustLoadJSON(t, path, &fix)

			var exp toolCallExpected
			mustLoadJSON(t, expectedPath, &exp)

			// --- Check 1: directory classification consistency ---
			checkDirectoryClassification(t, rel, exp)

			// --- Check 2: sawToolCallSyntax cross-validation ---
			checkSawToolCallSyntax(t, fix.Text, exp)

			// --- Check 3: tool_names ↔ calls name matching ---
			checkToolNamesMatch(t, fix.ToolNames, exp)

			// --- Check 4: rejectedByPolicy internal consistency ---
			checkRejectedByPolicyConsistency(t, exp)

			// --- Check 5: invoke tag count cross-validation ---
			checkInvokeCount(t, rel, fix.Text, exp)

			// --- Check 6: CDATA parameter value sampling ---
			checkParameterValuesInText(t, fix.Text, exp)
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
}

// checkDirectoryClassification verifies that true_positive fixtures have
// non-empty calls and false_positive fixtures have empty calls.
func checkDirectoryClassification(t *testing.T, rel string, exp toolCallExpected) {
	t.Helper()
	dir := filepath.Dir(rel)
	switch {
	case strings.HasPrefix(dir, "true_positive"):
		if len(exp.Calls) == 0 {
			t.Errorf("[classification] true_positive fixture has 0 expected calls — expected at least 1")
		}
	case strings.HasPrefix(dir, "false_positive"):
		if len(exp.Calls) > 0 {
			t.Errorf("[classification] false_positive fixture has %d expected calls — expected 0", len(exp.Calls))
		}
	}
}

// checkSawToolCallSyntax uses the public ContainsToolCallWrapperSyntaxOutsideIgnored
// detector to independently verify the expected sawToolCallSyntax flag.
func checkSawToolCallSyntax(t *testing.T, text string, exp toolCallExpected) {
	t.Helper()
	hasDSML, hasCanonical := toolcall.ContainsToolCallWrapperSyntaxOutsideIgnored(strings.TrimSpace(text))
	detected := hasDSML || hasCanonical
	if detected && !exp.SawToolCallSyntax {
		t.Errorf("[sawToolCallSyntax] independent detector found wrapper syntax (dsml=%v canonical=%v) but expected says false", hasDSML, hasCanonical)
	}
	if !detected && exp.SawToolCallSyntax {
		t.Errorf("[sawToolCallSyntax] independent detector found NO wrapper syntax but expected says true")
	}
}

// checkToolNamesMatch verifies that each expected call name appears in the
// fixture's tool_names list (case-insensitive).
func checkToolNamesMatch(t *testing.T, toolNames []string, exp toolCallExpected) {
	t.Helper()
	if len(exp.Calls) == 0 || len(toolNames) == 0 {
		return
	}
	lowerNames := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		lowerNames[strings.ToLower(strings.TrimSpace(n))] = true
	}
	for i, c := range exp.Calls {
		if !lowerNames[strings.ToLower(c.Name)] {
			t.Errorf("[tool_names] calls[%d].name=%q not found in fixture tool_names %v", i, c.Name, toolNames)
		}
	}
}

// checkRejectedByPolicyConsistency verifies internal consistency of the
// rejectedByPolicy flag and rejectedToolNames list.
func checkRejectedByPolicyConsistency(t *testing.T, exp toolCallExpected) {
	t.Helper()
	if exp.RejectedByPolicy {
		if len(exp.Calls) != 0 {
			t.Errorf("[rejectedByPolicy] rejectedByPolicy=true but calls has %d entries — expected 0", len(exp.Calls))
		}
		if len(exp.RejectedToolNames) == 0 {
			t.Errorf("[rejectedByPolicy] rejectedByPolicy=true but rejectedToolNames is empty")
		}
	}
}

// invokeNameRe is a lightweight regex to find invoke tags with a name attribute
// across all DSML variants.  It intentionally does NOT use the parser.
var invokeNameRe = regexp.MustCompile(`(?i)<[^>]*?invoke[^>]*?\bname\s*=\s*["']([^"']+)["']`)

// checkInvokeCount independently counts invoke tags in the fixture text
// (outside fenced code blocks) and compares with the expected call count.
func checkInvokeCount(t *testing.T, rel, text string, exp toolCallExpected) {
	t.Helper()
	dir := filepath.Dir(rel)
	if !strings.HasPrefix(dir, "true_positive") {
		return
	}
	stripped := stripMarkdownFences(text)
	matches := invokeNameRe.FindAllStringSubmatch(stripped, -1)
	if len(matches) != len(exp.Calls) {
		t.Errorf("[invoke_count] regex found %d invoke tags but expected has %d calls", len(matches), len(exp.Calls))
	}
}

// checkParameterValuesInText verifies that string values in expected call
// inputs actually appear somewhere in the fixture text.
func checkParameterValuesInText(t *testing.T, text string, exp toolCallExpected) {
	t.Helper()
	for i, c := range exp.Calls {
		for key, val := range c.Input {
			s, ok := val.(string)
			if !ok {
				continue
			}
			if s == "" {
				continue
			}
			if !strings.Contains(text, s) {
				t.Errorf("[param_value] calls[%d].input[%q]=%q not found in fixture text", i, key, truncate(s, 80))
			}
		}
	}
}

// stripMarkdownFences removes content inside fenced code blocks (``` or ~~~)
// so that invoke tags inside code examples are not counted.
func stripMarkdownFences(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	inFence := false
	fencePrefix := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inFence {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = true
				fencePrefix = trimmed[:3]
				continue
			}
			out = append(out, line)
		} else {
			if strings.HasPrefix(trimmed, fencePrefix) && strings.TrimSpace(strings.TrimPrefix(trimmed, fencePrefix)) == "" {
				inFence = false
			}
		}
	}
	return strings.Join(out, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Reuse types from go_compat_toolcalls_test.go via package-level declarations.
// The types toolCallFixture, toolCallExpected, toolCallExpectedCall are already
// defined in go_compat_toolcalls_test.go within the same package.
// mustLoadJSON and compatPath are defined in go_compat_test.go.
