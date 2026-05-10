package compat

import (
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/toolcall"
)

// Run with -update to regenerate expected files from current parser output.
var updateToolCallFixtures = flag.Bool("update-toolcalls", false, "regenerate toolcalls expected files")

// toolCallFixture mirrors tests/compat/fixtures/toolcalls/**/*.json
type toolCallFixture struct {
	Text      string   `json:"text"`
	ToolNames []string `json:"tool_names"`
}

// toolCallExpected mirrors tests/compat/expected/toolcalls_*.json
type toolCallExpected struct {
	Calls             []toolCallExpectedCall `json:"calls"`
	SawToolCallSyntax bool                   `json:"sawToolCallSyntax"`
	RejectedByPolicy  bool                   `json:"rejectedByPolicy"`
	RejectedToolNames []string               `json:"rejectedToolNames"`
}

type toolCallExpectedCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func TestGoCompatToolCallFixtures(t *testing.T) {
	root := compatPath("fixtures", "toolcalls")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		rel, _ := filepath.Rel(root, path)
		// Build expected filename: replace path separators with underscores.
		// e.g. true_positive/dsml_single_cdata_read.json
		//   -> toolcalls_true_positive_dsml_single_cdata_read.json
		expectedName := "toolcalls_" + strings.ReplaceAll(strings.TrimSuffix(rel, ".json"), string(filepath.Separator), "_") + ".json"
		expectedPath := compatPath("expected", expectedName)

		name := strings.TrimSuffix(rel, ".json")
		t.Run(name, func(t *testing.T) {
			var fix toolCallFixture
			mustLoadJSON(t, path, &fix)

			res := toolcall.ParseToolCallsDetailed(fix.Text, fix.ToolNames)

			calls := make([]toolCallExpectedCall, 0, len(res.Calls))
			for _, c := range res.Calls {
				input := c.Input
				if input == nil {
					input = map[string]any{}
				}
				calls = append(calls, toolCallExpectedCall{Name: c.Name, Input: input})
			}
			rejNames := res.RejectedToolNames
			if rejNames == nil {
				rejNames = []string{}
			}
			got := toolCallExpected{
				Calls:             calls,
				SawToolCallSyntax: res.SawToolCallSyntax,
				RejectedByPolicy:  res.RejectedByPolicy,
				RejectedToolNames: rejNames,
			}

			if *updateToolCallFixtures {
				raw, _ := json.MarshalIndent(got, "", "  ")
				_ = os.MkdirAll(filepath.Dir(expectedPath), 0o755)
				if err := os.WriteFile(expectedPath, append(raw, '\n'), 0o644); err != nil {
					t.Fatalf("write expected %s: %v", expectedPath, err)
				}
				t.Logf("updated %s", expectedPath)
				return
			}

			var want toolCallExpected
			mustLoadJSON(t, expectedPath, &want)

			if got.SawToolCallSyntax != want.SawToolCallSyntax {
				t.Errorf("sawToolCallSyntax: got=%v want=%v", got.SawToolCallSyntax, want.SawToolCallSyntax)
			}
			if got.RejectedByPolicy != want.RejectedByPolicy {
				t.Errorf("rejectedByPolicy: got=%v want=%v", got.RejectedByPolicy, want.RejectedByPolicy)
			}
			if len(got.Calls) != len(want.Calls) {
				t.Fatalf("calls count: got=%d want=%d\ngot calls: %+v", len(got.Calls), len(want.Calls), got.Calls)
			}
			for i := range got.Calls {
				if got.Calls[i].Name != want.Calls[i].Name {
					t.Errorf("call[%d].name: got=%q want=%q", i, got.Calls[i].Name, want.Calls[i].Name)
				}
				if !reflect.DeepEqual(got.Calls[i].Input, want.Calls[i].Input) {
					gotJSON, _ := json.Marshal(got.Calls[i].Input)
					wantJSON, _ := json.Marshal(want.Calls[i].Input)
					t.Errorf("call[%d].input: got=%s want=%s", i, gotJSON, wantJSON)
				}
			}
			if !reflect.DeepEqual(got.RejectedToolNames, want.RejectedToolNames) {
				t.Errorf("rejectedToolNames: got=%v want=%v", got.RejectedToolNames, want.RejectedToolNames)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
}
