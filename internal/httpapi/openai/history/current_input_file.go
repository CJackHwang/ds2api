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
	"ds2api/internal/observe"
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
	ContextEngineMode() string
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
	historyHash := currentInputContentHash(fileText)
	toolsHash := ""
	if strings.TrimSpace(toolsText) != "" {
		toolsHash = currentInputContentHash(toolsText)
	}
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	fileID, err := s.uploadGeneratedFile(ctx, a, currentInputFilename, modelType, fileText)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}
	toolFileID := ""
	toolCacheHit := false
	cacheHits := 0
	cacheMisses := 0
	if strings.TrimSpace(toolsText) != "" {
		var err error
		toolFileID, toolCacheHit, err = s.uploadCachedToolsFile(ctx, a, modelType, toolsText, toolsHash)
		if err != nil {
			return stdReq, fmt.Errorf("upload current tools file: %w", err)
		}
		if toolFileID == "" {
			return stdReq, errors.New("upload current tools file returned empty file id")
		}
		if toolCacheHit {
			cacheHits++
		} else {
			cacheMisses++
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
	stdReq.FinalPrompt, stdReq.ToolNames = promptcompat.BuildOpenAIPromptWithToolInstructionsOnly(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking, s.Store.ContextEngineMode())
	promptHash := currentInputContentHash(stdReq.FinalPrompt)
	stdReq.CurrentInputHash = historyHash
	stdReq.CurrentToolsHash = toolsHash
	stdReq.CurrentPromptHash = promptHash
	stdReq.CurrentToolsFileCacheHit = toolCacheHit
	// Token accounting must reflect the actual downstream context:
	// uploaded context files + the continuation live prompt.
	tokenParts := []string{fileText}
	if strings.TrimSpace(toolsText) != "" {
		tokenParts = append(tokenParts, toolsText)
	}
	tokenParts = append(tokenParts, stdReq.FinalPrompt)
	stdReq.PromptTokenText = strings.Join(tokenParts, "\n")
	observe.RecordCurrentInputFiles(ctx, observe.CurrentInputFileMetrics{
		HistoryHash: historyHash,
		ToolsHash:   toolsHash,
		PromptHash:  promptHash,
		CacheHits:   cacheHits,
		CacheMisses: cacheMisses,
		RefCount:    generatedCurrentInputRefCount(fileID, toolFileID),
	})
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
	historyHash := currentInputContentHash(stdReq.HistoryText)
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	fileID, err := s.uploadGeneratedFile(ctx, a, currentInputFilename, modelType, stdReq.HistoryText)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}

	toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscript(stdReq.ToolsRaw, stdReq.ToolChoice)
	toolsHash := ""
	if strings.TrimSpace(toolsText) != "" {
		toolsHash = currentInputContentHash(toolsText)
	}
	toolFileID := ""
	toolCacheHit := false
	cacheHits := 0
	cacheMisses := 0
	if strings.TrimSpace(toolsText) != "" {
		var err error
		toolFileID, toolCacheHit, err = s.uploadCachedToolsFile(ctx, a, modelType, toolsText, toolsHash)
		if err != nil {
			return stdReq, fmt.Errorf("upload current tools file: %w", err)
		}
		if toolFileID == "" {
			return stdReq, errors.New("upload current tools file returned empty file id")
		}
		if toolCacheHit {
			cacheHits++
		} else {
			cacheMisses++
		}
	}

	stdReq.RefFileIDs = replaceGeneratedCurrentInputRefs(stdReq.RefFileIDs, stdReq.CurrentInputFileID, stdReq.CurrentToolsFileID, fileID, toolFileID)
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	stdReq.CurrentInputHash = historyHash
	stdReq.CurrentToolsHash = toolsHash
	stdReq.CurrentPromptHash = currentInputContentHash(stdReq.FinalPrompt)
	stdReq.CurrentToolsFileCacheHit = toolCacheHit
	observe.RecordCurrentInputFiles(ctx, observe.CurrentInputFileMetrics{
		HistoryHash: historyHash,
		ToolsHash:   toolsHash,
		PromptHash:  stdReq.CurrentPromptHash,
		CacheHits:   cacheHits,
		CacheMisses: cacheMisses,
		RefCount:    generatedCurrentInputRefCount(fileID, toolFileID),
	})
	return stdReq, nil
}

func (s Service) uploadGeneratedFile(ctx context.Context, a *auth.RequestAuth, filename, modelType, text string) (string, error) {
	result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
		Filename:    filename,
		ContentType: currentInputContentType,
		Purpose:     currentInputPurpose,
		ModelType:   modelType,
		Data:        []byte(text),
	}, 3)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return strings.TrimSpace(result.ID), nil
}

func (s Service) uploadCachedToolsFile(ctx context.Context, a *auth.RequestAuth, modelType, text, hash string) (string, bool, error) {
	key := generatedFileCacheKey{
		AccountScope: currentInputCacheScope(a),
		ModelType:    strings.TrimSpace(modelType),
		Filename:     currentToolsFilename,
		ContentHash:  strings.TrimSpace(hash),
	}
	if fileID, ok := currentInputToolsFileCache.lookup(key); ok {
		return fileID, true, nil
	}
	fileID, err := s.uploadGeneratedFile(ctx, a, currentToolsFilename, modelType, text)
	if err != nil {
		return "", false, err
	}
	currentInputToolsFileCache.store(key, fileID)
	return fileID, false, nil
}

func generatedCurrentInputRefCount(fileIDs ...string) int {
	count := 0
	for _, fileID := range fileIDs {
		if strings.TrimSpace(fileID) != "" {
			count++
		}
	}
	return count
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
