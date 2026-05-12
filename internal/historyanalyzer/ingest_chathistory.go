package historyanalyzer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ds2api/internal/chathistory"
)

var safeChatHistoryIDRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type ChatHistoryLoadOptions struct {
	Path     string
	Redactor Redactor
}

type chatHistoryDetailEnvelope struct {
	Version int               `json:"version"`
	Item    chathistory.Entry `json:"item"`
}

type chatHistoryIndexProbe struct {
	Items []json.RawMessage `json:"items"`
}

func LoadChatHistory(opts ChatHistoryLoadOptions) ([]AnalysisRecord, ReportScope, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return nil, ReportScope{}, errors.New("chat history path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ReportScope{}, fmt.Errorf("read chat history: %w", err)
	}

	entries, err := decodeChatHistory(raw, path)
	if err != nil {
		return nil, ReportScope{}, err
	}
	source := SourceRef{Kind: "chat_history", Path: path}
	scope := ReportScope{
		Name: "chat history",
		Sources: []SourceRef{
			source,
		},
	}
	return RecordsFromChatHistory(entries, source, opts.Redactor), scope, nil
}

func RecordsFromChatHistory(entries []chathistory.Entry, source SourceRef, redactor Redactor) []AnalysisRecord {
	if redactor.MaxExcerptRunes == 0 {
		redactor = DefaultRedactor()
	}
	ordered := make([]chathistory.Entry, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		ordered = append(ordered, entry)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt == ordered[j].CreatedAt {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CreatedAt < ordered[j].CreatedAt
	})

	out := make([]AnalysisRecord, 0, len(ordered))
	for _, entry := range ordered {
		out = append(out, recordFromChatHistoryEntry(entry, source, redactor))
	}
	return out
}

func decodeChatHistory(raw []byte, path string) ([]chathistory.Entry, error) {
	if chatHistoryIndexHasDetails(raw) {
		var index chathistory.File
		if err := json.Unmarshal(raw, &index); err != nil {
			return nil, fmt.Errorf("decode chat history index: %w", err)
		}
		return readChatHistoryDetails(index, path)
	}

	var legacy struct {
		Items []chathistory.Entry `json:"items"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("decode legacy chat history: %w", err)
	}
	return legacy.Items, nil
}

func chatHistoryIndexHasDetails(raw []byte) bool {
	var probe chatHistoryIndexProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	for _, item := range probe.Items {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			continue
		}
		if _, ok := fields["detail_revision"]; ok {
			return true
		}
	}
	return false
}

func readChatHistoryDetails(index chathistory.File, path string) ([]chathistory.Entry, error) {
	detailDir := path + ".d"
	entries := make([]chathistory.Entry, 0, len(index.Items))
	for _, summary := range index.Items {
		id := strings.TrimSpace(summary.ID)
		if id == "" {
			continue
		}
		detailPath, err := chatHistoryDetailPath(detailDir, id)
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(detailPath)
		if err != nil {
			return nil, fmt.Errorf("read chat history detail %s: %w", id, err)
		}
		var envelope chatHistoryDetailEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("decode chat history detail %s: %w", id, err)
		}
		entries = append(entries, envelope.Item)
	}
	return entries, nil
}

func safeChatHistoryID(id string) bool {
	return safeChatHistoryIDRe.MatchString(id) && !strings.Contains(id, "..")
}

func chatHistoryDetailPath(detailDir, id string) (string, error) {
	if !safeChatHistoryID(id) {
		return "", fmt.Errorf("unsafe chat history detail id %q", id)
	}
	cleanDir := filepath.Clean(detailDir)
	detailPath := filepath.Join(cleanDir, id+".json")
	rel, err := filepath.Rel(cleanDir, detailPath)
	if err != nil {
		return "", fmt.Errorf("resolve chat history detail path %q: %w", id, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("chat history detail path escapes detail dir: %q", id)
	}
	return detailPath, nil
}

func recordFromChatHistoryEntry(entry chathistory.Entry, source SourceRef, redactor Redactor) AnalysisRecord {
	text := map[string]string{}
	addText(text, "user_input", entry.UserInput)
	addText(text, "history_text", entry.HistoryText)
	addText(text, "final_prompt", entry.FinalPrompt)
	addText(text, "reasoning", entry.ReasoningContent)
	addText(text, "content", entry.Content)
	addText(text, "error", entry.Error)
	addText(text, "messages", renderChatMessages(entry.Messages))

	source.ID = entry.ID
	return AnalysisRecord{
		RequestID:    entry.ID,
		CreatedAt:    unixMillis(entry.CreatedAt),
		Surface:      entry.Surface,
		Protocol:     entry.Surface,
		Model:        entry.Model,
		Stream:       entry.Stream,
		Status:       entry.Status,
		StatusCode:   entry.StatusCode,
		FinishReason: entry.FinishReason,
		Text:         text,
		Snapshots:    snapshotsFromText(text, redactor),
		Metrics: RuntimeMetrics{
			ElapsedMs: entry.ElapsedMs,
			Extra:     usageExtra(entry.Usage),
		},
		Sources: []SourceRef{source},
	}
}

func usageExtra(usage map[string]any) map[string]any {
	if len(usage) == 0 {
		return nil
	}
	return map[string]any{"usage": usage}
}

func renderChatMessages(messages []chathistory.Message) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" && content == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		if role != "" {
			b.WriteString(role)
			b.WriteString(": ")
		}
		b.WriteString(content)
	}
	return b.String()
}

func addText(fields map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fields[key] = value
}

func unixMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}
