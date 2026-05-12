package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/historyanalyzer"
)

func TestRunWritesReportAndFixtureCandidates(t *testing.T) {
	tmp := t.TempDir()
	historyPath := filepath.Join(tmp, "history.json")
	body := `{
  "items": [
    {
      "id": "chat_1",
      "created_at": 1770000000000,
      "status": "success",
      "status_code": 200,
      "surface": "openai.chat",
      "model": "deepseek-chat",
      "content": "<tool_calls><invoke name=\"search\"><parameter name=\"q\">Bearer abcdefghijklmnop</parameter></invoke></tool_calls>"
    }
  ]
}`
	if err := os.WriteFile(historyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	outMarkdown := filepath.Join(tmp, "report.md")
	outJSON := filepath.Join(tmp, "report.json")
	outFixtures := filepath.Join(tmp, "fixture-candidates.json")
	err := run(cliOptions{
		historyPath:     historyPath,
		scope:           "unit history analyzer",
		outMarkdown:     outMarkdown,
		outJSON:         outJSON,
		outFixtures:     outFixtures,
		maxExcerptRunes: 240,
	})
	if err != nil {
		t.Fatal(err)
	}

	md := readFile(t, outMarkdown)
	if !strings.Contains(md, "HA_TOOL_MARKER_LEAK") {
		t.Fatalf("markdown missing marker leak finding:\n%s", md)
	}
	if strings.Contains(md, "abcdefghijklmnop") {
		t.Fatalf("markdown leaked token:\n%s", md)
	}

	var report historyanalyzer.Report
	if err := json.Unmarshal([]byte(readFile(t, outJSON)), &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.TotalRecords != 1 {
		t.Fatalf("total records = %d, want 1", report.Summary.TotalRecords)
	}
	if report.Summary.TotalFindings == 0 {
		t.Fatalf("expected findings in report: %#v", report)
	}
	if report.Readiness == nil || report.Readiness.Decision != "NO-GO" {
		t.Fatalf("readiness = %#v, want NO-GO", report.Readiness)
	}

	var candidates []historyanalyzer.FixtureCandidate
	if err := json.Unmarshal([]byte(readFile(t, outFixtures)), &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected fixture candidates")
	}
}

func TestRunRequiresAtLeastOneInput(t *testing.T) {
	err := run(cliOptions{
		outMarkdown: filepath.Join(t.TempDir(), "report.md"),
		outJSON:     filepath.Join(t.TempDir(), "report.json"),
	})
	if err == nil || !strings.Contains(err.Error(), "at least one input") {
		t.Fatalf("error = %v, want input requirement", err)
	}
}

func TestRunWritesOnlyReportToStdout(t *testing.T) {
	tmp := t.TempDir()
	historyPath := filepath.Join(tmp, "history.json")
	if err := os.WriteFile(historyPath, []byte(`{"items":[{"id":"chat_1","created_at":1770000000000,"status":"success","status_code":200,"content":"ok"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()
	err = run(cliOptions{
		historyPath: historyPath,
		outMarkdown: filepath.Join(tmp, "report.md"),
		outJSON:     "-",
		outFixtures: "",
	})
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = reader.Close()
	}()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "wrote ") {
		t.Fatalf("stdout contains progress text: %s", string(body))
	}
	var report historyanalyzer.Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("stdout is not report JSON: %v\n%s", err, string(body))
	}
}

func TestRunRejectsMultipleStdoutOutputs(t *testing.T) {
	err := run(cliOptions{
		historyPath: filepath.Join(t.TempDir(), "missing.json"),
		outMarkdown: "-",
		outJSON:     "-",
	})
	if err == nil || !strings.Contains(err.Error(), "only one output target") {
		t.Fatalf("error = %v, want stdout target validation", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
