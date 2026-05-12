package historyanalyzer

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ds2api/internal/devcapture"
	"ds2api/internal/rawsample"
)

func RecordFromDevCapture(entry devcapture.Entry, redactor Redactor) AnalysisRecord {
	if redactor.MaxExcerptRunes == 0 {
		redactor = DefaultRedactor()
	}
	text := map[string]string{}
	addText(text, "request_body", entry.RequestBody)
	addText(text, "response_body", entry.ResponseBody)

	snapshots := snapshotsFromText(text, redactor)
	flags := map[string]string{}
	if entry.ResponseTruncated {
		flags["response_truncated"] = "true"
	}

	return AnalysisRecord{
		RequestID:  entry.ID,
		CreatedAt:  unixSeconds(entry.CreatedAt),
		Surface:    entry.Label,
		Protocol:   "devcapture",
		StatusCode: entry.StatusCode,
		Text:       text,
		Snapshots:  snapshots,
		Flags:      flags,
		Sources: []SourceRef{
			{Kind: "devcapture", ID: entry.ID, Note: entry.URL},
		},
	}
}

func RecordsFromDevCapture(entries []devcapture.Entry, redactor Redactor) []AnalysisRecord {
	out := make([]AnalysisRecord, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		out = append(out, RecordFromDevCapture(entry, redactor))
	}
	return out
}

func RecordFromRawSample(meta rawsample.Meta, dir string, redactor Redactor) AnalysisRecord {
	if redactor.MaxExcerptRunes == 0 {
		redactor = DefaultRedactor()
	}
	text := map[string]string{}
	addText(text, "request", stablePayloadText(meta.Request))

	flags := map[string]string{}
	if meta.Capture.ContainsReferenceMarkers {
		flags["contains_reference_markers"] = "true"
	}
	if meta.Capture.ContainsFinishedToken {
		flags["contains_finished_token"] = "true"
	}

	capturedAt, _ := time.Parse(time.RFC3339, meta.CapturedAtUTC)
	return AnalysisRecord{
		RequestID:  meta.SampleID,
		CreatedAt:  capturedAt.UTC(),
		Surface:    meta.Source,
		Protocol:   "rawsample",
		StatusCode: meta.Capture.StatusCode,
		Text:       text,
		Snapshots:  snapshotsFromText(text, redactor),
		Flags:      flags,
		Metrics: RuntimeMetrics{
			Extra: map[string]any{
				"response_bytes":          meta.Capture.ResponseBytes,
				"reference_marker_count":  meta.Capture.ReferenceMarkerCount,
				"finished_token_count":    meta.Capture.FinishedTokenCount,
				"capture_rounds":          meta.Capture.Rounds,
				"capture_response_status": meta.Capture.StatusCode,
			},
		},
		Sources: []SourceRef{
			{Kind: "rawsample", Path: dir, ID: meta.SampleID},
		},
	}
}

func stablePayloadText(value any) string {
	body, err := json.Marshal(value)
	if err == nil {
		return string(body)
	}
	return fmt.Sprintf("%v", value)
}

func snapshotsFromText(text map[string]string, redactor Redactor) map[string]TextSnapshot {
	if len(text) == 0 {
		return nil
	}
	out := make(map[string]TextSnapshot, len(text))
	for field, value := range text {
		excerpt := redactor.Excerpt(value)
		out[field] = TextSnapshot{
			Value: excerpt,
			Hash:  HashText(excerpt),
		}
	}
	return out
}

func unixSeconds(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}
