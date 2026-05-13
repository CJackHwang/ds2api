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
	currentInputFilename    = promptcompat.CurrentInputContextFilename
	currentToolsFilename    = promptcompat.CurrentToolsContextFilename
	currentInputContentType = "text/plain; charset=utf-8"
	currentInputPurpose     = "assistants"
)

type CurrentInputConfigReader interface {
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
}

type CurrentInputUploader interface {
	UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
}

type Service struct {
	Store CurrentInputConfigReader
	DS    CurrentInputUploader
}

func (s Service) ApplyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if stdReq.CurrentInputFileApplied || s.DS == nil || s.Store == nil || a == nil || !s.Store.CurrentInputFileEnabled() {
		return stdReq, nil
	}
	threshold := s.Store.CurrentInputFileMinChars()

	index, text := latestUserInputForFile(stdReq.Messages)
	if index < 0 {
		return stdReq, nil
	}
	if len([]rune(text)) < threshold {
		return stdReq, nil
	}
	fileText := promptcompat.BuildOpenAICurrentInputContextTranscript(stdReq.Messages)
	if strings.TrimSpace(fileText) == "" {
		return stdReq, errors.New("current user input file produced empty transcript")
	}
	toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscript(stdReq.ToolsRaw, stdReq.ToolChoice)
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
		Filename:    currentInputFilename,
		ContentType: currentInputContentType,
		Purpose:     currentInputPurpose,
		ModelType:   modelType,
		Data:        []byte(fileText),
	}, 3)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	fileID := strings.TrimSpace(result.ID)
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}

	toolFileID := ""
	if strings.TrimSpace(toolsText) != "" {
		result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
			Filename:    currentToolsFilename,
			ContentType: currentInputContentType,
			Purpose:     currentInputPurpose,
			ModelType:   modelType,
			Data:        []byte(toolsText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload current tools file: %w", err)
		}
		toolFileID = strings.TrimSpace(result.ID)
		if toolFileID == "" {
			return stdReq, errors.New("upload current tools file returned empty file id")
		}
	}

	messages := []any{
		map[string]any{
			"role":    "user",
			"content": currentInputFilePrompt(toolFileID != ""),
		},
	}

	stdReq.Messages = messages
	stdReq.HistoryText = fileText
	stdReq.CurrentInputFileApplied = true
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	stdReq.RefFileIDs = prependUniqueRefFileIDs(stdReq.RefFileIDs, fileID, toolFileID)
	stdReq.FinalPrompt, stdReq.ToolNames = promptcompat.BuildOpenAIPromptWithToolInstructionsOnly(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking)
	// Token accounting must reflect the actual downstream context:
	// uploaded context files + the continuation live prompt.
	tokenParts := []string{fileText}
	if strings.TrimSpace(toolsText) != "" {
		tokenParts = append(tokenParts, toolsText)
	}
	tokenParts = append(tokenParts, stdReq.FinalPrompt)
	stdReq.PromptTokenText = strings.Join(tokenParts, "\n")
	return stdReq, nil
}

func (s Service) ReuploadAppliedCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if !stdReq.CurrentInputFileApplied || s.DS == nil || a == nil {
		return stdReq, nil
	}
	fileText := strings.TrimSpace(stdReq.HistoryText)
	if fileText == "" {
		return stdReq, nil
	}
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
		Filename:    currentInputFilename,
		ContentType: currentInputContentType,
		Purpose:     currentInputPurpose,
		ModelType:   modelType,
		Data:        []byte(stdReq.HistoryText),
	}, 3)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	fileID := strings.TrimSpace(result.ID)
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}

	toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscript(stdReq.ToolsRaw, stdReq.ToolChoice)
	toolFileID := ""
	if strings.TrimSpace(toolsText) != "" {
		result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
			Filename:    currentToolsFilename,
			ContentType: currentInputContentType,
			Purpose:     currentInputPurpose,
			ModelType:   modelType,
			Data:        []byte(toolsText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload current tools file: %w", err)
		}
		toolFileID = strings.TrimSpace(result.ID)
		if toolFileID == "" {
			return stdReq, errors.New("upload current tools file returned empty file id")
		}
	}

	stdReq.RefFileIDs = replaceGeneratedCurrentInputRefs(stdReq.RefFileIDs, stdReq.CurrentInputFileID, stdReq.CurrentToolsFileID, fileID, toolFileID)
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	return stdReq, nil
}

func latestUserInputForFile(messages []any) (int, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "user" {
			continue
		}
		text := promptcompat.NormalizeOpenAIContentForPrompt(msg["content"])
		if strings.TrimSpace(text) == "" {
			return -1, ""
		}
		return i, text
	}
	return -1, ""
}

func currentInputFilePrompt(hasToolsFile bool) string {
	prompt := "Continue from the latest state in the attached DS2API_HISTORY.txt context. Treat it as the current working state and answer the latest user request directly."
	if hasToolsFile {
		prompt += " Available tool descriptions and parameter schemas are attached in DS2API_TOOLS.txt; use only those tools and follow the tool-call format rules in this prompt."
	}
	return prompt
}

func prependUniqueRefFileIDs(existing []string, fileIDs ...string) []string {
	out := make([]string, 0, len(existing)+len(fileIDs))
	seen := map[string]struct{}{}
	for _, fileID := range fileIDs {
		trimmed := strings.TrimSpace(fileID)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	return out
}

func replaceGeneratedCurrentInputRefs(existing []string, oldHistoryID, oldToolsID, newHistoryID, newToolsID string) []string {
	filtered := make([]string, 0, len(existing))
	old := map[string]struct{}{}
	for _, id := range []string{oldHistoryID, oldToolsID} {
		trimmed := strings.ToLower(strings.TrimSpace(id))
		if trimmed != "" {
			old[trimmed] = struct{}{}
		}
	}
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := old[strings.ToLower(trimmed)]; ok {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return prependUniqueRefFileIDs(filtered, newHistoryID, newToolsID)
}
