package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/util"
)

func TestGoCompatSSEFixtures(t *testing.T) {
	files, err := filepath.Glob(compatPath("fixtures", "sse_chunks", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no sse fixtures found")
	}
	for _, fixturePath := range files {
		name := trimExt(filepath.Base(fixturePath))
		expectedPath := compatPath("expected", "sse_"+name+".json")

		var fixture struct {
			Chunk          map[string]any `json:"chunk"`
			ThinkingEnable bool           `json:"thinking_enabled"`
			CurrentType    string         `json:"current_type"`
		}
		mustLoadJSON(t, fixturePath, &fixture)

		var expected struct {
			Parts         []map[string]any `json:"parts"`
			Finished      bool             `json:"finished"`
			NewType       string           `json:"new_type"`
			ContentFilter bool             `json:"content_filter"`
			ErrorMessage  string           `json:"error_message"`
		}
		mustLoadJSON(t, expectedPath, &expected)

		raw, err := json.Marshal(fixture.Chunk)
		if err != nil {
			t.Fatalf("marshal fixture %s failed: %v", name, err)
		}
		res := sse.ParseDeepSeekContentLine(append([]byte("data: "), raw...), fixture.ThinkingEnable, fixture.CurrentType)
		gotParts := make([]map[string]any, 0, len(res.Parts))
		for _, p := range res.Parts {
			gotParts = append(gotParts, map[string]any{
				"text": p.Text,
				"type": p.Type,
			})
		}
		if !reflect.DeepEqual(gotParts, expected.Parts) ||
			res.Stop != expected.Finished ||
			res.NextType != expected.NewType ||
			res.ContentFilter != expected.ContentFilter ||
			res.ErrorMessage != expected.ErrorMessage {
			t.Fatalf("fixture %s mismatch:\n got parts=%#v finished=%v newType=%q contentFilter=%v errorMessage=%q\nwant parts=%#v finished=%v newType=%q contentFilter=%v errorMessage=%q",
				name, gotParts, res.Stop, res.NextType, res.ContentFilter, res.ErrorMessage,
				expected.Parts, expected.Finished, expected.NewType, expected.ContentFilter, expected.ErrorMessage)
		}
	}
}

func TestGoCompatTokenFixtures(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"cases"`
	}
	mustLoadJSON(t, compatPath("fixtures", "token_cases.json"), &fixture)

	var expected struct {
		Cases []struct {
			Name   string `json:"name"`
			Tokens int    `json:"tokens"`
		} `json:"cases"`
	}
	mustLoadJSON(t, compatPath("expected", "token_cases.json"), &expected)

	expectByName := map[string]int{}
	for _, c := range expected.Cases {
		expectByName[c.Name] = c.Tokens
	}
	for _, c := range fixture.Cases {
		want, ok := expectByName[c.Name]
		if !ok {
			t.Fatalf("missing expected token case: %s", c.Name)
		}
		got := util.EstimateTokens(c.Text)
		if got != want {
			t.Fatalf("token fixture %s mismatch: got=%d want=%d", c.Name, got, want)
		}
	}
}

func mustLoadJSON(t *testing.T, path string, out any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("decode %s failed: %v", path, err)
	}
}

func trimExt(name string) string {
	if len(name) > 5 && name[len(name)-5:] == ".json" {
		return name[:len(name)-5]
	}
	return name
}

func compatPath(parts ...string) string {
	prefix := []string{"..", "..", "tests", "compat"}
	return filepath.Join(append(prefix, parts...)...)
}
