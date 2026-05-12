package historyanalyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ds2api/internal/chathistory"
)

func TestLoadChatHistoryV2ReadsDetailsAndRedactsSnapshots(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "chat_history.json")
	detailDir := indexPath + ".d"
	if err := os.MkdirAll(detailDir, 0o755); err != nil {
		t.Fatal(err)
	}

	index := chathistory.File{
		Version: chathistory.FileVersion,
		Limit:   chathistory.DefaultLimit,
		Items: []chathistory.SummaryEntry{
			{ID: "chat_b", CreatedAt: 2000, DetailRevision: 1},
			{ID: "chat_a", CreatedAt: 1000, DetailRevision: 1},
		},
	}
	writeJSONFile(t, indexPath, index)
	writeJSONFile(t, filepath.Join(detailDir, "chat_a.json"), map[string]any{
		"version": chathistory.FileVersion,
		"item": chathistory.Entry{
			ID:          "chat_a",
			CreatedAt:   1000,
			Surface:     "chat.completions",
			Model:       "deepseek-chat",
			UserInput:   "hello user@example.com",
			FinalPrompt: "Authorization: Bearer abcdefghijklmnop",
			Content:     "ok",
			Usage:       map[string]any{"total_tokens": 3},
		},
	})
	writeJSONFile(t, filepath.Join(detailDir, "chat_b.json"), map[string]any{
		"version": chathistory.FileVersion,
		"item": chathistory.Entry{
			ID:        "chat_b",
			CreatedAt: 2000,
			Content:   "second",
		},
	})

	records, scope, err := LoadChatHistory(ChatHistoryLoadOptions{Path: indexPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].RequestID != "chat_a" {
		t.Fatalf("records not sorted oldest-first: %#v", records)
	}
	if scope.Sources[0].Path != indexPath {
		t.Fatalf("scope source path = %q, want %q", scope.Sources[0].Path, indexPath)
	}
	if records[0].Text["final_prompt"] == "" {
		t.Fatal("raw text missing from in-memory record")
	}
	if strings.Contains(records[0].Snapshots["final_prompt"].Value, "abcdefghijklmnop") {
		t.Fatalf("snapshot leaked bearer token: %s", records[0].Snapshots["final_prompt"].Value)
	}
	if strings.Contains(records[0].Snapshots["user_input"].Value, "user@example.com") {
		t.Fatalf("snapshot leaked email: %s", records[0].Snapshots["user_input"].Value)
	}
	if records[0].Metrics.Extra["usage"] == nil {
		t.Fatalf("usage was not preserved in metrics extra: %#v", records[0].Metrics.Extra)
	}

	body, err := json.Marshal(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "abcdefghijklmnop") || strings.Contains(string(body), "user@example.com") {
		t.Fatalf("serialized record leaked raw text: %s", body)
	}
}

func TestLoadChatHistoryLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	writeJSONFile(t, path, map[string]any{
		"items": []chathistory.Entry{
			{ID: "legacy_1", CreatedAt: 1234, Content: "legacy content"},
		},
	})

	records, _, err := LoadChatHistory(ChatHistoryLoadOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].RequestID != "legacy_1" {
		t.Fatalf("request id = %q, want legacy_1", records[0].RequestID)
	}
	if records[0].CreatedAt != time.UnixMilli(1234).UTC() {
		t.Fatalf("created_at = %s, want %s", records[0].CreatedAt, time.UnixMilli(1234).UTC())
	}
}

func TestLoadChatHistoryRejectsUnsafeDetailID(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "chat_history.json")
	index := chathistory.File{
		Version: chathistory.FileVersion,
		Limit:   chathistory.DefaultLimit,
		Items: []chathistory.SummaryEntry{
			{ID: "../outside", CreatedAt: 1000, DetailRevision: 1},
		},
	}
	writeJSONFile(t, indexPath, index)

	_, _, err := LoadChatHistory(ChatHistoryLoadOptions{Path: indexPath})
	if err == nil {
		t.Fatal("expected unsafe detail id to fail")
	}
	if !strings.Contains(err.Error(), "unsafe chat history detail id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatHistoryDetailPathStaysUnderDetailDir(t *testing.T) {
	got, err := chatHistoryDetailPath("/tmp/history.json.d", "chat_safe-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("/tmp/history.json.d", "chat_safe-1.json") {
		t.Fatalf("path = %q, want detail path", got)
	}

	if _, err := chatHistoryDetailPath("/tmp/history.json.d", "../outside"); err == nil {
		t.Fatal("expected unsafe id to fail")
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
