package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
	"ds2api/internal/util"
)

func historySplitTestMessages() []any {
	toolCalls := []any{
		map[string]any{
			"name":      "search",
			"arguments": map[string]any{"query": "docs"},
		},
	}
	return []any{
		map[string]any{"role": "system", "content": "system instructions"},
		map[string]any{"role": "user", "content": "first user turn"},
		map[string]any{
			"role":              "assistant",
			"content":           "",
			"reasoning_content": "hidden reasoning",
			"tool_calls":        toolCalls,
		},
		map[string]any{
			"role":         "tool",
			"name":         "search",
			"tool_call_id": "call-1",
			"content":      "tool result",
		},
		map[string]any{"role": "user", "content": "latest user turn"},
	}
}

type streamStatusManagedAuthStub struct{}

func (streamStatusManagedAuthStub) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{
		UseConfigToken: true,
		DeepSeekToken:  "managed-token",
		CallerID:       "caller:test",
		AccountID:      "acct:test",
		TriedAccounts:  map[string]bool{},
	}, nil
}

func (streamStatusManagedAuthStub) DetermineCaller(_ *http.Request) (*auth.RequestAuth, error) {
	return (&streamStatusManagedAuthStub{}).Determine(nil)
}

func (streamStatusManagedAuthStub) Release(_ *auth.RequestAuth) {}

func TestBuildOpenAICurrentInputContextTranscriptUsesNumberedHistorySections(t *testing.T) {
	transcript := buildOpenAICurrentInputContextTranscript(historySplitTestMessages())

	if strings.Contains(transcript, "[file content end]") || strings.Contains(transcript, "[file content begin]") || strings.Contains(transcript, "[file name]:") {
		t.Fatalf("expected transcript without file wrapper tags, got %q", transcript)
	}
	if !strings.Contains(transcript, "# DS2API_HISTORY.txt") {
		t.Fatalf("expected history transcript header, got %q", transcript)
	}
	if !strings.Contains(transcript, "Prior conversation history and tool progress.") {
		t.Fatalf("expected history transcript description, got %q", transcript)
	}
	for _, want := range []string{
		"=== 1. SYSTEM ===",
		"=== 2. USER ===",
		"=== 3. ASSISTANT ===",
		"=== 4. TOOL ===",
		"=== 5. USER ===",
		"first user turn",
		"tool result",
		"latest user turn",
		"[reasoning_content]",
		"hidden reasoning",
		"<|DSML|tool_calls>",
	} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("expected transcript to contain %q, got %q", want, transcript)
		}
	}
}

func TestApplyCurrentInputFileSkipsShortInputWhenThresholdNotReached(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
			currentInputMin:     10,
		},
		DS: ds,
	}
	req := map[string]any{
		"model": "deepseek-v4-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if len(ds.uploadCalls) != 0 {
		t.Fatalf("expected no upload on first turn, got %d", len(ds.uploadCalls))
	}
	if out.FinalPrompt != stdReq.FinalPrompt {
		t.Fatalf("expected prompt unchanged on first turn")
	}
}

func TestApplyThinkingInjectionAppendsLatestUserPrompt(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			thinkingInjection: boolPtr(true),
		},
		DS: ds,
	}
	req := map[string]any{
		"model": "deepseek-v4-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply thinking injection failed: %v", err)
	}
	if len(ds.uploadCalls) != 0 {
		t.Fatalf("expected no upload for first short turn, got %d", len(ds.uploadCalls))
	}
	if !strings.Contains(out.FinalPrompt, "hello\n\n"+promptcompat.ThinkingInjectionMarker) {
		t.Fatalf("expected thinking injection after latest user message, got %s", out.FinalPrompt)
	}
}

func TestApplyThinkingInjectionUsesCustomPrompt(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			thinkingInjection: boolPtr(true),
			thinkingPrompt:    "custom thinking format",
		},
		DS: ds,
	}
	req := map[string]any{
		"model": "deepseek-v4-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply thinking injection failed: %v", err)
	}
	if !strings.Contains(out.FinalPrompt, "hello\n\ncustom thinking format") {
		t.Fatalf("expected custom thinking injection after latest user message, got %s", out.FinalPrompt)
	}
}

func TestApplyCurrentInputFileDisabledPassThrough(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: false,
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-vision",
		"messages": historySplitTestMessages(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if len(ds.uploadCalls) != 0 {
		t.Fatalf("expected no uploads when both split modes are disabled, got %d", len(ds.uploadCalls))
	}
	if out.CurrentInputFileApplied || out.HistoryText != "" {
		t.Fatalf("expected direct pass-through, got current_input=%v history=%q", out.CurrentInputFileApplied, out.HistoryText)
	}
	if !strings.Contains(out.FinalPrompt, "first user turn") || !strings.Contains(out.FinalPrompt, "latest user turn") {
		t.Fatalf("expected original prompt context to stay inline, got %s", out.FinalPrompt)
	}
}

func TestApplyCurrentInputFileUploadsFirstTurnWithNumberedHistoryTranscript(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
			currentInputMin:     10,
			thinkingInjection:   boolPtr(true),
		},
		DS: ds,
	}
	req := map[string]any{
		"model": "deepseek-v4-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "first turn content that is long enough"},
		},
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 current input upload, got %d", len(ds.uploadCalls))
	}
	upload := ds.uploadCalls[0]
	if upload.Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("unexpected upload filename: %q", upload.Filename)
	}
	uploadedText := string(upload.Data)
	if strings.Contains(uploadedText, "[file content end]") || strings.Contains(uploadedText, "[file content begin]") || strings.Contains(uploadedText, "[file name]:") {
		t.Fatalf("expected uploaded transcript without file wrapper tags, got %q", uploadedText)
	}
	for _, want := range []string{
		"# DS2API_HISTORY.txt",
		"=== 1. USER ===",
		"first turn content that is long enough",
	} {
		if !strings.Contains(uploadedText, want) {
			t.Fatalf("expected uploaded transcript to contain %q, got %q", want, uploadedText)
		}
	}
	if !strings.Contains(uploadedText, promptcompat.ThinkingInjectionMarker) {
		t.Fatalf("expected thinking injection in current input file, got %q", uploadedText)
	}

	if strings.Contains(out.FinalPrompt, "first turn content that is long enough") {
		t.Fatalf("expected current input text to be replaced in live prompt, got %s", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, "CURRENT_USER_INPUT.txt") || strings.Contains(out.FinalPrompt, "Read that file") {
		t.Fatalf("expected live prompt not to instruct file reads, got %s", out.FinalPrompt)
	}
	if !strings.Contains(out.FinalPrompt, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation-oriented prompt in live prompt, got %s", out.FinalPrompt)
	}
	if len(out.RefFileIDs) != 1 || out.RefFileIDs[0] != "file-inline-1" {
		t.Fatalf("expected current input file id in ref_file_ids, got %#v", out.RefFileIDs)
	}
	if !strings.Contains(out.PromptTokenText, "first turn content that is long enough") {
		t.Fatalf("expected prompt token text to preserve original full context, got %q", out.PromptTokenText)
	}
	if !strings.Contains(out.PromptTokenText, "# DS2API_HISTORY.txt") || !strings.Contains(out.PromptTokenText, "=== 1. USER ===") {
		t.Fatalf("expected prompt token text to include numbered history transcript, got %q", out.PromptTokenText)
	}
}

func TestApplyCurrentInputFilePreservesFullContextPromptForTokenCounting(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
			currentInputMin:     0,
			thinkingInjection:   boolPtr(true),
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-vision",
		"messages": historySplitTestMessages(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if out.FinalPrompt == stdReq.FinalPrompt {
		t.Fatalf("expected live prompt to be rewritten after current input file")
	}
	// PromptTokenText must include the uploaded file content (which contains the full context)
	// plus the neutral live prompt — reflecting the actual downstream token cost.
	if !strings.Contains(out.PromptTokenText, "first user turn") || !strings.Contains(out.PromptTokenText, "latest user turn") {
		t.Fatalf("expected prompt token text to contain file context with full conversation, got %q", out.PromptTokenText)
	}
	if strings.Contains(out.PromptTokenText, "[file content end]") || strings.Contains(out.PromptTokenText, "[file name]:") {
		t.Fatalf("expected prompt token text to omit file wrapper tags, got %q", out.PromptTokenText)
	}
	if !strings.Contains(out.PromptTokenText, "# DS2API_HISTORY.txt") || !strings.Contains(out.PromptTokenText, "=== 1. SYSTEM ===") {
		t.Fatalf("expected prompt token text to include numbered history transcript, got %q", out.PromptTokenText)
	}
	if !strings.Contains(out.PromptTokenText, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected prompt token text to also include continuation prompt, got %q", out.PromptTokenText)
	}
	if strings.Contains(out.FinalPrompt, "first user turn") || strings.Contains(out.FinalPrompt, "latest user turn") {
		t.Fatalf("expected live prompt to hide original turns, got %q", out.FinalPrompt)
	}
}

func TestApplyCurrentInputFileUploadsFullContextFile(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
			currentInputMin:     0,
			thinkingInjection:   boolPtr(true),
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-vision",
		"messages": historySplitTestMessages(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if !out.CurrentInputFileApplied {
		t.Fatalf("expected current input file to apply")
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected one current input upload, got %d", len(ds.uploadCalls))
	}
	upload := ds.uploadCalls[0]
	if upload.Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("expected DS2API_HISTORY.txt upload, got %q", upload.Filename)
	}
	if upload.ModelType != "vision" {
		t.Fatalf("expected vision model type for vision request, got %q", upload.ModelType)
	}
	uploadedText := string(upload.Data)
	for _, want := range []string{"# DS2API_HISTORY.txt", "=== 1. SYSTEM ===", "=== 2. USER ===", "=== 3. ASSISTANT ===", "=== 4. TOOL ===", "=== 5. USER ===", "system instructions", "first user turn", "hidden reasoning", "tool result", "latest user turn", promptcompat.ThinkingInjectionMarker} {
		if !strings.Contains(uploadedText, want) {
			t.Fatalf("expected full context file to contain %q, got %q", want, uploadedText)
		}
	}
	if strings.Contains(out.FinalPrompt, "first user turn") || strings.Contains(out.FinalPrompt, "latest user turn") || strings.Contains(out.FinalPrompt, "CURRENT_USER_INPUT.txt") || strings.Contains(out.FinalPrompt, "Read that file") {
		t.Fatalf("expected live prompt to use only a continuation instruction, got %s", out.FinalPrompt)
	}
	if !strings.Contains(out.FinalPrompt, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation-oriented prompt in live prompt, got %s", out.FinalPrompt)
	}
}

func TestApplyCurrentInputFileUploadsToolsContextSeparately(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
			currentInputMin:     0,
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": "search docs",
					"parameters": map[string]any{
						"type": "object",
					},
				},
			},
		},
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected history and tools uploads, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("expected first upload to be DS2API_HISTORY.txt, got %q", ds.uploadCalls[0].Filename)
	}
	if ds.uploadCalls[1].Filename != "DS2API_TOOLS.txt" {
		t.Fatalf("expected second upload to be DS2API_TOOLS.txt, got %q", ds.uploadCalls[1].Filename)
	}
	historyText := string(ds.uploadCalls[0].Data)
	if strings.Contains(historyText, "You have access to these tools") || strings.Contains(historyText, "Description: search docs") {
		t.Fatalf("history transcript should not embed tool descriptions, got %q", historyText)
	}
	toolsText := string(ds.uploadCalls[1].Data)
	for _, want := range []string{"# DS2API_TOOLS.txt", "Tool: search", "Description: search docs", `Parameters: {"type":"object"}`} {
		if !strings.Contains(toolsText, want) {
			t.Fatalf("expected tools transcript to contain %q, got %q", want, toolsText)
		}
	}
	if strings.Contains(toolsText, "TOOL CALL FORMAT") {
		t.Fatalf("tools transcript should not duplicate tool format instructions, got %q", toolsText)
	}
	if !strings.Contains(out.FinalPrompt, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") || !strings.Contains(out.FinalPrompt, "DS2API_TOOLS.txt") {
		t.Fatalf("expected live prompt to reference both context files, got %q", out.FinalPrompt)
	}
	if !strings.Contains(out.FinalPrompt, "TOOL CALL FORMAT") || !strings.Contains(out.FinalPrompt, "Remember: The ONLY valid way to use tools") {
		t.Fatalf("expected live prompt to retain tool format instructions, got %q", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, "You have access to these tools") || strings.Contains(out.FinalPrompt, "Description: search docs") || strings.Contains(out.FinalPrompt, "Parameters:") {
		t.Fatalf("expected live prompt to omit tool descriptions after tools upload, got %q", out.FinalPrompt)
	}
	if len(out.RefFileIDs) < 2 || out.RefFileIDs[0] != "file-inline-1" || out.RefFileIDs[1] != "file-inline-2" {
		t.Fatalf("expected history and tools file ids first, got %#v", out.RefFileIDs)
	}
	if !strings.Contains(out.PromptTokenText, "# DS2API_HISTORY.txt") || !strings.Contains(out.PromptTokenText, "# DS2API_TOOLS.txt") || !strings.Contains(out.PromptTokenText, "Description: search docs") {
		t.Fatalf("expected prompt token text to include uploaded history and tools content, got %q", out.PromptTokenText)
	}
}

func TestApplyCurrentInputFileCarriesHistoryText(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply current input file failed: %v", err)
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(ds.uploadCalls))
	}
	if out.HistoryText != string(ds.uploadCalls[0].Data) {
		t.Fatalf("expected current input file flow to preserve uploaded text in history, got %q", out.HistoryText)
	}
	if !strings.Contains(out.HistoryText, "# DS2API_HISTORY.txt") || !strings.Contains(out.HistoryText, "=== 1. SYSTEM ===") {
		t.Fatalf("expected history text to use numbered transcript format, got %q", out.HistoryText)
	}
}

func TestChatCompletionsCurrentInputFileUploadsContextAndKeepsNeutralPrompt(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(ds.uploadCalls))
	}
	upload := ds.uploadCalls[0]
	if upload.Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("unexpected upload filename: %q", upload.Filename)
	}
	if upload.Purpose != "assistants" {
		t.Fatalf("unexpected purpose: %q", upload.Purpose)
	}
	historyText := string(upload.Data)
	if strings.Contains(historyText, "[file content end]") || strings.Contains(historyText, "[file content begin]") || strings.Contains(historyText, "[file name]:") {
		t.Fatalf("expected history transcript without file wrapper tags, got %s", historyText)
	}
	if !strings.Contains(historyText, "# DS2API_HISTORY.txt") || !strings.Contains(historyText, "=== 1. SYSTEM ===") {
		t.Fatalf("expected history transcript to use numbered sections, got %s", historyText)
	}
	if !strings.Contains(historyText, "latest user turn") {
		t.Fatalf("expected full context to include latest turn, got %s", historyText)
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	promptText, _ := ds.completionReq["prompt"].(string)
	if !strings.Contains(promptText, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation-oriented prompt, got %s", promptText)
	}
	if strings.Contains(promptText, "first user turn") || strings.Contains(promptText, "latest user turn") {
		t.Fatalf("expected prompt to hide original turns, got %s", promptText)
	}
	refIDs, _ := ds.completionReq["ref_file_ids"].([]any)
	if len(refIDs) == 0 || refIDs[0] != "file-inline-1" {
		t.Fatalf("expected uploaded current input file to be first ref_file_id, got %#v", ds.completionReq["ref_file_ids"])
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	usage, _ := body["usage"].(map[string]any)
	promptTokens := int(usage["prompt_tokens"].(float64))
	neutralCount := util.CountPromptTokens(promptText, "deepseek-v4-flash")
	if promptTokens <= neutralCount {
		t.Fatalf("expected prompt_tokens to exceed neutral live prompt count (includes file context), got=%d neutral=%d", promptTokens, neutralCount)
	}
}

func TestResponsesCurrentInputFileUploadsContextAndKeepsNeutralPrompt(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(ds.uploadCalls))
	}
	historyText := string(ds.uploadCalls[0].Data)
	if !strings.Contains(historyText, "# DS2API_HISTORY.txt") || !strings.Contains(historyText, "=== 1. SYSTEM ===") {
		t.Fatalf("expected uploaded history text to use numbered transcript format, got %s", historyText)
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	promptText, _ := ds.completionReq["prompt"].(string)
	if !strings.Contains(promptText, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation-oriented prompt, got %s", promptText)
	}
	if strings.Contains(promptText, "first user turn") || strings.Contains(promptText, "latest user turn") {
		t.Fatalf("expected prompt to hide original turns, got %s", promptText)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	usage, _ := body["usage"].(map[string]any)
	inputTokens := int(usage["input_tokens"].(float64))
	neutralCount := util.CountPromptTokens(promptText, "deepseek-v4-flash")
	if inputTokens <= neutralCount {
		t.Fatalf("expected input_tokens to exceed neutral live prompt count (includes file context), got=%d neutral=%d", inputTokens, neutralCount)
	}
}

func TestResponsesCurrentInputFileUploadsToolsSeparately(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": "search docs",
					"parameters":  map[string]any{"type": "object"},
				},
			},
		},
		"stream": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected history and tools uploads, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "DS2API_HISTORY.txt" || ds.uploadCalls[1].Filename != "DS2API_TOOLS.txt" {
		t.Fatalf("unexpected upload filenames: %#v", ds.uploadCalls)
	}
	historyText := string(ds.uploadCalls[0].Data)
	if strings.Contains(historyText, "Description: search docs") {
		t.Fatalf("history transcript should not embed tool descriptions, got %q", historyText)
	}
	toolsText := string(ds.uploadCalls[1].Data)
	if !strings.Contains(toolsText, "# DS2API_TOOLS.txt") || !strings.Contains(toolsText, "Tool: search") || !strings.Contains(toolsText, "Description: search docs") {
		t.Fatalf("expected tools transcript to include schema, got %q", toolsText)
	}
	promptText, _ := ds.completionReq["prompt"].(string)
	if !strings.Contains(promptText, "DS2API_TOOLS.txt") || !strings.Contains(promptText, "TOOL CALL FORMAT") {
		t.Fatalf("expected live prompt to reference tools file and retain format instructions, got %q", promptText)
	}
	if strings.Contains(promptText, "Description: search docs") {
		t.Fatalf("live prompt should not inline tool descriptions, got %q", promptText)
	}
	refIDs, _ := ds.completionReq["ref_file_ids"].([]any)
	if len(refIDs) < 2 || refIDs[0] != "file-inline-1" || refIDs[1] != "file-inline-2" {
		t.Fatalf("expected history and tools ref ids first, got %#v", ds.completionReq["ref_file_ids"])
	}
}

func TestChatCompletionsCurrentInputFileMapsManagedAuthFailureTo401(t *testing.T) {
	ds := &inlineUploadDSStub{
		uploadErr: &dsclient.RequestFailure{Op: "upload file", Kind: dsclient.FailureManagedUnauthorized, Message: "expired token"},
	}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusManagedAuthStub{},
		DS:   ds,
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer managed-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Please re-login the account in admin") {
		t.Fatalf("expected managed auth error message, got %s", rec.Body.String())
	}
}

func TestResponsesCurrentInputFileMapsDirectAuthFailureTo401(t *testing.T) {
	ds := &inlineUploadDSStub{
		uploadErr: &dsclient.RequestFailure{Op: "upload file", Kind: dsclient.FailureDirectUnauthorized, Message: "invalid token"},
	}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Invalid token") {
		t.Fatalf("expected direct auth error message, got %s", rec.Body.String())
	}
}

func TestChatCompletionsCurrentInputFileUploadFailureReturnsInternalServerError(t *testing.T) {
	ds := &inlineUploadDSStub{uploadErr: errors.New("boom")}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCurrentInputFileWorksAcrossAutoDeleteModes(t *testing.T) {
	for _, mode := range []string{"none", "single", "all"} {
		t.Run(mode, func(t *testing.T) {
			ds := &inlineUploadDSStub{}
			h := &openAITestSurface{
				Store: mockOpenAIConfig{
					autoDeleteMode:      mode,
					currentInputEnabled: true,
				},
				Auth: streamStatusAuthStub{},
				DS:   ds,
			}
			reqBody, _ := json.Marshal(map[string]any{
				"model":    "deepseek-v4-flash",
				"messages": historySplitTestMessages(),
				"stream":   false,
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(reqBody)))
			req.Header.Set("Authorization", "Bearer direct-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.ChatCompletions(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			if len(ds.uploadCalls) != 1 {
				t.Fatalf("expected current input upload for mode=%s, got %d", mode, len(ds.uploadCalls))
			}
			historyText := string(ds.uploadCalls[0].Data)
			if !strings.Contains(historyText, "# DS2API_HISTORY.txt") || !strings.Contains(historyText, "=== 1. SYSTEM ===") {
				t.Fatalf("expected uploaded history text to use numbered transcript format, got %s", historyText)
			}
			if ds.completionReq == nil {
				t.Fatalf("expected completion payload for mode=%s", mode)
			}
			promptText, _ := ds.completionReq["prompt"].(string)
			if !strings.Contains(promptText, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") || strings.Contains(promptText, "first user turn") || strings.Contains(promptText, "latest user turn") {
				t.Fatalf("unexpected prompt for mode=%s: %s", mode, promptText)
			}
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}
