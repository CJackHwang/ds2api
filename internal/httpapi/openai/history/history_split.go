package history

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
)

const (
	historySplitFilename    = "HISTORY.txt"
	historySplitContentType = "text/plain; charset=utf-8"
	historySplitPurpose     = "assistants"
	currentInputFilename    = "CURRENT_INPUT.txt"
	currentInputPromptNote  = "The user's full current request is attached as CURRENT_INPUT.txt. Treat that attached file as the authoritative full text for this turn."
	taskStateFilename       = "TASK_STATE.txt"
	taskStatePromptNote     = "The current task continuity state is attached as TASK_STATE.txt. Use it to preserve objectives, tool-format constraints, and recent failure context for this turn."
	currentInputMaxChars    = 12000
)

type Service struct {
	Store shared.ConfigReader
	DS    shared.DeepSeekCaller
}

func (s Service) Apply(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if s.DS == nil || s.Store == nil || a == nil {
		return stdReq, nil
	}

	promptMessages, historyMessages := SplitOpenAIHistoryMessages(stdReq.Messages, s.Store.HistorySplitTriggerAfterTurns())
	updatedMessages := promptMessages
	if len(updatedMessages) == 0 {
		updatedMessages = stdReq.Messages
	}

	currentUploaded := false
	if latestIdx, latestText, ok := findLatestLongUserTextMessage(updatedMessages); ok {
		result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
			Filename:    currentInputFilename,
			ContentType: historySplitContentType,
			Purpose:     historySplitPurpose,
			Data:        []byte(latestText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload current input file: %w", err)
		}
		fileID := strings.TrimSpace(result.ID)
		if fileID == "" {
			return stdReq, errors.New("upload current input file returned empty file id")
		}
		updatedMessages = cloneMessages(updatedMessages)
		msg, _ := updatedMessages[latestIdx].(map[string]any)
		copied := cloneMessage(msg)
		copied["content"] = currentInputPromptNote
		updatedMessages[latestIdx] = copied
		stdReq.RefFileIDs = prependUniqueRefFileID(stdReq.RefFileIDs, fileID)
		stdReq.Diagnostics.CurrentInputUpload = &promptcompat.FileUploadDiagnostic{
			Filename: currentInputFilename,
			Bytes:    len(latestText),
			FileID:   fileID,
		}
		stdReq.Diagnostics.CurrentInputReason = "latest_user_text_too_long"
		stdReq.Diagnostics.RefFileIDs = promptcompat.CloneStringSlice(stdReq.RefFileIDs)
		currentUploaded = true
		config.Logger.Info("[history_split] uploaded current input file",
			"filename", currentInputFilename,
			"bytes", len(latestText),
			"file_id", fileID,
			"reason", "latest_user_text_too_long",
		)
	} else {
		stdReq.Diagnostics.CurrentInputReason = currentInputSkipReason(updatedMessages)
	}

	if !currentUploaded {
		if compactMessages, liveText, ok := buildOversizedLivePromptFallback(updatedMessages); ok {
			result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
				Filename:    currentInputFilename,
				ContentType: historySplitContentType,
				Purpose:     historySplitPurpose,
				Data:        []byte(liveText),
			}, 3)
			if err != nil {
				return stdReq, fmt.Errorf("upload current input file: %w", err)
			}
			fileID := strings.TrimSpace(result.ID)
			if fileID == "" {
				return stdReq, errors.New("upload current input file returned empty file id")
			}
			updatedMessages = compactMessages
			stdReq.RefFileIDs = prependUniqueRefFileID(stdReq.RefFileIDs, fileID)
			stdReq.Diagnostics.CurrentInputUpload = &promptcompat.FileUploadDiagnostic{
				Filename: currentInputFilename,
				Bytes:    len(liveText),
				FileID:   fileID,
			}
			stdReq.Diagnostics.CurrentInputReason = "live_prompt_context_too_large"
			stdReq.Diagnostics.RefFileIDs = promptcompat.CloneStringSlice(stdReq.RefFileIDs)
			currentUploaded = true
			config.Logger.Info("[history_split] uploaded current input file",
				"filename", currentInputFilename,
				"bytes", len(liveText),
				"file_id", fileID,
				"reason", "live_prompt_context_too_large",
			)
		}
	}

	if len(historyMessages) > 0 {
		historyText := promptcompat.BuildOpenAIHistoryTranscript(historyMessages)
		if strings.TrimSpace(historyText) == "" {
			return stdReq, errors.New("history split produced empty transcript")
		}

		result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
			Filename:    historySplitFilename,
			ContentType: historySplitContentType,
			Purpose:     historySplitPurpose,
			Data:        []byte(historyText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload history file: %w", err)
		}
		fileID := strings.TrimSpace(result.ID)
		if fileID == "" {
			return stdReq, errors.New("upload history file returned empty file id")
		}
		stdReq.HistoryText = historyText
		stdReq.RefFileIDs = prependUniqueRefFileID(stdReq.RefFileIDs, fileID)
		stdReq.Diagnostics.HistoryUpload = &promptcompat.FileUploadDiagnostic{
			Filename: historySplitFilename,
			Bytes:    len(historyText),
			FileID:   fileID,
		}
		stdReq.Diagnostics.HistoryReason = "split_history_available"
		stdReq.Diagnostics.RefFileIDs = promptcompat.CloneStringSlice(stdReq.RefFileIDs)
		config.Logger.Info("[history_split] uploaded history file",
			"filename", historySplitFilename,
			"bytes", len(historyText),
			"file_id", fileID,
			"reason", "split_history_available",
		)
	} else {
		stdReq.Diagnostics.HistoryReason = historyUploadSkipReason(stdReq.Messages, historyMessages)
	}

	if taskStateText, ok := buildTaskStateText(updatedMessages, stdReq, len(historyMessages) > 0 || currentUploaded); ok {
		result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
			Filename:    taskStateFilename,
			ContentType: historySplitContentType,
			Purpose:     historySplitPurpose,
			Data:        []byte(taskStateText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload task state file: %w", err)
		}
		fileID := strings.TrimSpace(result.ID)
		if fileID == "" {
			return stdReq, errors.New("upload task state file returned empty file id")
		}
		updatedMessages = prependTaskStateNotice(updatedMessages)
		stdReq.RefFileIDs = prependUniqueRefFileID(stdReq.RefFileIDs, fileID)
		stdReq.Diagnostics.TaskStateUpload = &promptcompat.FileUploadDiagnostic{
			Filename: taskStateFilename,
			Bytes:    len(taskStateText),
			FileID:   fileID,
		}
		stdReq.Diagnostics.TaskStateReason = "task_continuity_summary_uploaded"
		stdReq.Diagnostics.RefFileIDs = promptcompat.CloneStringSlice(stdReq.RefFileIDs)
		config.Logger.Info("[history_split] uploaded task state file",
			"filename", taskStateFilename,
			"bytes", len(taskStateText),
			"file_id", fileID,
			"reason", "task_continuity_summary_uploaded",
		)
	} else {
		stdReq.Diagnostics.TaskStateReason = "task_state_not_needed"
	}

	stdReq.Messages = updatedMessages
	stdReq.FinalPrompt, stdReq.ToolNames = promptcompat.BuildOpenAIPrompt(updatedMessages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking)
	return stdReq, nil
}

func SplitOpenAIHistoryMessages(messages []any, triggerAfterTurns int) ([]any, []any) {
	if triggerAfterTurns <= 0 {
		triggerAfterTurns = 1
	}
	lastUserIndex := -1
	userTurns := 0
	for i, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "user" {
			continue
		}
		userTurns++
		lastUserIndex = i
	}
	if userTurns <= triggerAfterTurns || lastUserIndex < 0 {
		return messages, nil
	}

	promptMessages := make([]any, 0, len(messages)-lastUserIndex)
	historyMessages := make([]any, 0, lastUserIndex)
	for i, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			if i >= lastUserIndex {
				promptMessages = append(promptMessages, raw)
			} else {
				historyMessages = append(historyMessages, raw)
			}
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		switch role {
		case "system", "developer":
			promptMessages = append(promptMessages, raw)
		default:
			if i >= lastUserIndex {
				promptMessages = append(promptMessages, raw)
			} else {
				historyMessages = append(historyMessages, raw)
			}
		}
	}
	if len(promptMessages) == 0 {
		return messages, nil
	}
	return promptMessages, historyMessages
}

func prependUniqueRefFileID(existing []string, fileID string) []string {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return existing
	}
	out := make([]string, 0, len(existing)+1)
	out = append(out, fileID)
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || strings.EqualFold(trimmed, fileID) {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func findLatestLongUserTextMessage(messages []any) (int, string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "user" {
			continue
		}
		text := strings.TrimSpace(promptcompat.NormalizeOpenAIContentForPrompt(msg["content"]))
		if len(text) <= currentInputMaxChars {
			return -1, "", false
		}
		return i, text, true
	}
	return -1, "", false
}

func buildOversizedLivePromptFallback(messages []any) ([]any, string, bool) {
	liveText := promptcompat.BuildOpenAIHistoryTranscript(messages)
	if len(strings.TrimSpace(liveText)) <= currentInputMaxChars {
		return nil, "", false
	}
	kept := make([]any, 0, len(messages)+1)
	nonSystemCount := 0
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role == "system" || role == "developer" {
			kept = append(kept, raw)
			continue
		}
		nonSystemCount++
	}
	if nonSystemCount < 2 {
		return nil, "", false
	}
	kept = append(kept, map[string]any{
		"role":    "user",
		"content": currentInputPromptNote,
	})
	return kept, liveText, true
}

func currentInputSkipReason(messages []any) string {
	if len(messages) == 0 {
		return "no_live_messages"
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "user" {
			continue
		}
		text := strings.TrimSpace(promptcompat.NormalizeOpenAIContentForPrompt(msg["content"]))
		if text == "" {
			return "latest_user_text_empty"
		}
		if len(text) <= currentInputMaxChars {
			return "latest_user_text_below_threshold"
		}
		return "latest_user_text_too_long"
	}
	return "no_user_message"
}

func historyUploadSkipReason(allMessages []any, historyMessages []any) string {
	if len(historyMessages) > 0 {
		return "split_history_available"
	}
	userTurns := 0
	for _, raw := range allMessages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(shared.AsString(msg["role"])), "user") {
			userTurns++
		}
	}
	if userTurns <= 1 {
		return "not_enough_user_turns"
	}
	return "history_split_empty"
}

func buildTaskStateText(messages []any, stdReq promptcompat.StandardRequest, needed bool) (string, bool) {
	if !needed {
		return "", false
	}
	latestUser := ""
	recent := make([]string, 0, 6)
	nonSystemCount := 0
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		content := strings.TrimSpace(promptcompat.NormalizeOpenAIContentForPrompt(msg["content"]))
		if role != "system" && role != "developer" && content != "" {
			nonSystemCount++
		}
		if role == "user" && content != "" {
			latestUser = content
		}
		if role == "" || content == "" {
			continue
		}
		recent = append(recent, strings.ToUpper(role)+": "+truncateTaskStateLine(content, 280))
		if len(recent) > 6 {
			recent = recent[len(recent)-6:]
		}
	}
	if nonSystemCount == 0 && latestUser == "" {
		return "", false
	}
	lines := []string{
		"Task continuity summary for this request.",
		"",
		"Use this file to preserve the current objective, constraints, and recent failure context.",
	}
	if latestUser != "" {
		lines = append(lines, "", "Latest user request:", latestUser)
	}
	lines = append(lines,
		"",
		"Hard requirements:",
		"- Treat CURRENT_INPUT.txt as authoritative if it is attached.",
		"- Treat HISTORY.txt as earlier context if it is attached.",
		"- Use the canonical <|DSML|tool_calls> wrapper when calling tools.",
		"- Do not mix plain text after a tool-call block.",
	)
	lines = append(lines,
		"",
		"Recent decision signals:",
		"- current_input_reason: "+fallbackTaskStateReason(stdReq.Diagnostics.CurrentInputReason),
		"- history_reason: "+fallbackTaskStateReason(stdReq.Diagnostics.HistoryReason),
	)
	if len(stdReq.RefFileIDs) > 0 {
		lines = append(lines, "", "Existing attached file ids:")
		for _, id := range stdReq.RefFileIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			lines = append(lines, "- "+id)
		}
	}
	if len(recent) > 0 {
		lines = append(lines, "", "Recent live context:")
		for _, line := range recent {
			lines = append(lines, "- "+line)
		}
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return "", false
	}
	return text, true
}

func prependTaskStateNotice(messages []any) []any {
	if len(messages) == 0 {
		return []any{map[string]any{
			"role":    "system",
			"content": taskStatePromptNote,
		}}
	}
	out := cloneMessages(messages)
	for i, raw := range out {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "system" && role != "developer" {
			continue
		}
		copied := cloneMessage(msg)
		normalized := strings.TrimSpace(promptcompat.NormalizeOpenAIContentForPrompt(copied["content"]))
		if normalized == "" {
			copied["content"] = taskStatePromptNote
		} else if !strings.Contains(normalized, taskStatePromptNote) {
			copied["content"] = normalized + "\n\n" + taskStatePromptNote
		}
		out[i] = copied
		return out
	}
	return append([]any{map[string]any{
		"role":    "system",
		"content": taskStatePromptNote,
	}}, out...)
}

func truncateTaskStateLine(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func fallbackTaskStateReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "unknown"
	}
	return reason
}

func cloneMessages(messages []any) []any {
	if len(messages) == 0 {
		return nil
	}
	out := make([]any, len(messages))
	copy(out, messages)
	return out
}

func cloneMessage(msg map[string]any) map[string]any {
	if msg == nil {
		return nil
	}
	out := make(map[string]any, len(msg))
	for k, v := range msg {
		out[k] = v
	}
	return out
}
