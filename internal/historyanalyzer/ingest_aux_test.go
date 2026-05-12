package historyanalyzer

import (
	"strings"
	"testing"
	"time"

	"ds2api/internal/devcapture"
	"ds2api/internal/rawsample"
)

func TestRecordFromDevCapture(t *testing.T) {
	record := RecordFromDevCapture(devcapture.Entry{
		ID:                "cap_1",
		CreatedAt:         100,
		Label:             "deepseek",
		URL:               "https://example.test",
		StatusCode:        500,
		RequestBody:       `{"token":"secret"}`,
		ResponseBody:      "Bearer abcdefghijklmnop",
		ResponseTruncated: true,
	}, DefaultRedactor())

	if record.RequestID != "cap_1" {
		t.Fatalf("request id = %q, want cap_1", record.RequestID)
	}
	if record.CreatedAt != time.Unix(100, 0).UTC() {
		t.Fatalf("created_at = %s, want unix 100", record.CreatedAt)
	}
	if record.Flags["response_truncated"] != "true" {
		t.Fatalf("flags = %#v, want response_truncated", record.Flags)
	}
	if strings.Contains(record.Snapshots["request_body"].Value, "secret") {
		t.Fatalf("request snapshot leaked secret: %s", record.Snapshots["request_body"].Value)
	}
	if strings.Contains(record.Snapshots["response_body"].Value, "abcdefghijklmnop") {
		t.Fatalf("response snapshot leaked token: %s", record.Snapshots["response_body"].Value)
	}
}

func TestRecordFromRawSample(t *testing.T) {
	record := RecordFromRawSample(rawsample.Meta{
		SampleID:      "sample_1",
		CapturedAtUTC: "2026-05-12T00:00:00Z",
		Source:        "raw-stream",
		Request:       map[string]any{"model": "deepseek-chat"},
		Capture: rawsample.CaptureSummary{
			StatusCode:               200,
			ResponseBytes:            42,
			ContainsReferenceMarkers: true,
			ReferenceMarkerCount:     2,
			ContainsFinishedToken:    true,
			FinishedTokenCount:       1,
		},
	}, "/tmp/sample", DefaultRedactor())

	if record.RequestID != "sample_1" {
		t.Fatalf("request id = %q, want sample_1", record.RequestID)
	}
	if record.Flags["contains_reference_markers"] != "true" {
		t.Fatalf("flags = %#v, want contains_reference_markers", record.Flags)
	}
	if record.Metrics.Extra["response_bytes"] != 42 {
		t.Fatalf("response bytes = %#v, want 42", record.Metrics.Extra["response_bytes"])
	}
}

func TestRecordFromRawSampleSerializesRequestDeterministically(t *testing.T) {
	meta := rawsample.Meta{
		SampleID: "sample_1",
		Request: map[string]any{
			"z": 1,
			"a": 2,
		},
	}

	first := RecordFromRawSample(meta, "/tmp/sample", DefaultRedactor())
	second := RecordFromRawSample(meta, "/tmp/sample", DefaultRedactor())

	if first.Snapshots["request"].Value != `{"a":2,"z":1}` {
		t.Fatalf("request snapshot = %q, want deterministic JSON", first.Snapshots["request"].Value)
	}
	if first.Snapshots["request"].Hash != second.Snapshots["request"].Hash {
		t.Fatalf("hash changed across runs: %s vs %s", first.Snapshots["request"].Hash, second.Snapshots["request"].Hash)
	}
}
