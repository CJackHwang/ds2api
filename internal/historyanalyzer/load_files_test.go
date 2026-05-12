package historyanalyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ds2api/internal/rawsample"
)

func TestLoadDevCapturesAcceptsEnvelope(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "captures.json")
	body := `{"items":[{"id":"cap_1","created_at":1770000000,"label":"openai.chat","status_code":200,"response_body":"ok","response_truncated":true}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	records, scope, err := LoadDevCaptures(DevCaptureLoadOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Flags["response_truncated"] != "true" {
		t.Fatalf("flags = %#v, want response_truncated", records[0].Flags)
	}
	if len(scope.Sources) != 1 || scope.Sources[0].Kind != "devcapture" {
		t.Fatalf("scope = %#v, want devcapture source", scope)
	}
}

func TestLoadRawSamplesAcceptsRootDir(t *testing.T) {
	tmp := t.TempDir()
	sampleDir := filepath.Join(tmp, "sample-1")
	if err := os.MkdirAll(sampleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := rawsample.Meta{
		SampleID:      "sample-1",
		CapturedAtUTC: "2026-05-13T00:00:00Z",
		Source:        "unit",
		Request:       map[string]any{"prompt": "hello"},
		Capture: rawsample.CaptureSummary{
			StatusCode:               200,
			ResponseBytes:            128,
			ContainsFinishedToken:    true,
			FinishedTokenCount:       1,
			ContainsReferenceMarkers: true,
			ReferenceMarkerCount:     1,
		},
	}
	body, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sampleDir, "meta.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	stream := `data: {"v":"<tool_calls><invoke name=\"demo\"></invoke></tool_calls>"}`
	if err := os.WriteFile(filepath.Join(sampleDir, "upstream.stream.sse"), []byte(stream), 0o644); err != nil {
		t.Fatal(err)
	}

	records, scope, err := LoadRawSamples(RawSampleLoadOptions{Path: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Flags["contains_reference_markers"] != "true" || records[0].Flags["contains_finished_token"] != "true" {
		t.Fatalf("flags = %#v, want rawsample flags", records[0].Flags)
	}
	if records[0].Text["upstream_text"] == "" {
		t.Fatalf("upstream_text was not loaded: %#v", records[0].Text)
	}
	report := New(DefaultRules()...).Analyze(records, AnalyzeOptions{})
	if len(report.Findings) == 0 {
		t.Fatalf("expected upstream stream finding, got %#v", report)
	}
	if len(scope.Sources) != 1 || scope.Sources[0].Kind != "rawsample" {
		t.Fatalf("scope = %#v, want rawsample source", scope)
	}
}
