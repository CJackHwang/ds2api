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

func longCurrentInputMessages() []any {
	return []any{
		map[string]any{"role": "system", "content": "system instructions"},
		map[string]any{"role": "user", "content": "first user turn"},
		map[string]any{"role": "assistant", "content": "first assistant turn"},
		map[string]any{"role": "user", "content": strings.Repeat("latest long user turn ", 700)},
	}
}

func longCurrentInputPartMessages() []any {
	return []any{
		map[string]any{"role": "system", "content": "system instructions"},
		map[string]any{"role": "user", "content": "first user turn"},
		map[string]any{"role": "assistant", "content": "first assistant turn"},
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": strings.Repeat("latest long part user turn ", 700)},
			},
		},
	}
}

func oversizedLivePromptMessagesWithoutLongLatestUser() []any {
	return []any{
		map[string]any{"role": "system", "content": "system instructions"},
		map[string]any{"role": "assistant", "content": strings.Repeat("large assistant context ", 900)},
		map[string]any{"role": "tool", "content": strings.Repeat("large tool context ", 600)},
		map[string]any{"role": "user", "content": "short follow-up"},
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

func TestBuildOpenAIHistoryTranscriptUsesInjectedFileWrapper(t *testing.T) {
	_, historyMessages := splitOpenAIHistoryMessages(historySplitTestMessages(), 1)
	transcript := buildOpenAIHistoryTranscript(historyMessages)

	if !strings.HasPrefix(transcript, "[file content end]\n\n") {
		t.Fatalf("expected injected file wrapper prefix, got %q", transcript)
	}
	if !strings.Contains(transcript, "<｜begin▁of▁sentence｜>") {
		t.Fatalf("expected serialized conversation markers, got %q", transcript)
	}
	if !strings.Contains(transcript, "first user turn") || !strings.Contains(transcript, "tool result") {
		t.Fatalf("expected historical turns preserved, got %q", transcript)
	}
	if !strings.Contains(transcript, "[reasoning_content]") || !strings.Contains(transcript, "hidden reasoning") {
		t.Fatalf("expected reasoning block preserved, got %q", transcript)
	}
	if !strings.Contains(transcript, "<|DSML|tool_calls>") {
		t.Fatalf("expected tool calls preserved, got %q", transcript)
	}
	if !strings.HasSuffix(transcript, "\n[file name]: IGNORE\n[file content begin]\n") {
		t.Fatalf("expected injected file wrapper suffix, got %q", transcript)
	}
}

func TestSplitOpenAIHistoryMessagesUsesLatestUserTurn(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "system instructions"},
		map[string]any{"role": "user", "content": "first user turn"},
		map[string]any{"role": "assistant", "content": "first assistant turn"},
		map[string]any{"role": "user", "content": "middle user turn"},
		map[string]any{"role": "assistant", "content": "middle assistant turn"},
		map[string]any{"role": "user", "content": "latest user turn"},
	}

	promptMessages, historyMessages := splitOpenAIHistoryMessages(messages, 1)
	if len(promptMessages) == 0 || len(historyMessages) == 0 {
		t.Fatalf("expected both prompt and history messages, got prompt=%d history=%d", len(promptMessages), len(historyMessages))
	}

	promptText, _ := promptcompat.BuildOpenAIPrompt(promptMessages, nil, "", defaultToolChoicePolicy(), true)
	if !strings.Contains(promptText, "latest user turn") {
		t.Fatalf("expected latest user turn in prompt, got %s", promptText)
	}
	if strings.Contains(promptText, "middle user turn") {
		t.Fatalf("expected middle user turn to be moved into history, got %s", promptText)
	}

	historyText := buildOpenAIHistoryTranscript(historyMessages)
	if !strings.Contains(historyText, "middle user turn") {
		t.Fatalf("expected middle user turn in split history, got %s", historyText)
	}
	if strings.Contains(historyText, "latest user turn") {
		t.Fatalf("expected latest user turn to remain live, got %s", historyText)
	}
}

func TestApplyHistorySplitSkipsFirstTurn(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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

	out, err := h.applyHistorySplit(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply history split failed: %v", err)
	}
	if len(ds.uploadCalls) != 0 {
		t.Fatalf("expected no upload on first turn, got %d", len(ds.uploadCalls))
	}
	if out.FinalPrompt != stdReq.FinalPrompt {
		t.Fatalf("expected prompt unchanged on first turn")
	}
}

func TestApplyHistorySplitCarriesHistoryText(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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

	out, err := h.applyHistorySplit(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply history split failed: %v", err)
	}
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected 2 upload calls, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "HISTORY.txt" || ds.uploadCalls[1].Filename != "TASK_STATE.txt" {
		t.Fatalf("unexpected upload order: %#v", ds.uploadCalls)
	}
	if out.HistoryText != string(ds.uploadCalls[0].Data) {
		t.Fatalf("expected history text to be preserved on normalized request")
	}
}

func TestApplyHistorySplitUploadsLongCurrentInputAndHistory(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": longCurrentInputMessages(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyHistorySplit(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply history split failed: %v", err)
	}
	if len(ds.uploadCalls) != 3 {
		t.Fatalf("expected 3 upload calls, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "CURRENT_INPUT.txt" {
		t.Fatalf("expected current input upload first, got %q", ds.uploadCalls[0].Filename)
	}
	if ds.uploadCalls[1].Filename != "HISTORY.txt" {
		t.Fatalf("expected history upload second, got %q", ds.uploadCalls[1].Filename)
	}
	if ds.uploadCalls[2].Filename != "TASK_STATE.txt" {
		t.Fatalf("expected task state upload third, got %q", ds.uploadCalls[2].Filename)
	}
	if out.HistoryText != string(ds.uploadCalls[1].Data) {
		t.Fatalf("expected history text to match uploaded history file")
	}
	promptText := out.FinalPrompt
	if !strings.Contains(promptText, "CURRENT_INPUT.txt") {
		t.Fatalf("expected prompt to reference uploaded current input file, got %s", promptText)
	}
	if strings.Contains(promptText, "latest long user turn latest long user turn") {
		t.Fatalf("expected oversized current user text removed from live prompt, got %s", promptText)
	}
	if len(out.RefFileIDs) < 3 || out.RefFileIDs[0] != "file-inline-TASK_STATE" || out.RefFileIDs[1] != "file-inline-HISTORY" || out.RefFileIDs[2] != "file-inline-CURRENT_INPUT" {
		t.Fatalf("expected history and current-input ref files prepended, got %#v", out.RefFileIDs)
	}
}

func TestApplyHistorySplitUploadsLongCurrentInputPartsAndHistory(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": longCurrentInputPartMessages(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyHistorySplit(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply history split failed: %v", err)
	}
	if len(ds.uploadCalls) != 3 {
		t.Fatalf("expected 3 upload calls, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "CURRENT_INPUT.txt" || ds.uploadCalls[1].Filename != "HISTORY.txt" || ds.uploadCalls[2].Filename != "TASK_STATE.txt" {
		t.Fatalf("unexpected upload order: %#v", ds.uploadCalls)
	}
	if !strings.Contains(out.FinalPrompt, "CURRENT_INPUT.txt") {
		t.Fatalf("expected prompt to reference uploaded current input file, got %s", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, "latest long part user turn latest long part user turn") {
		t.Fatalf("expected oversized user parts removed from live prompt, got %s", out.FinalPrompt)
	}
}

func TestApplyHistorySplitUploadsOversizedLivePromptFallback(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
		},
		DS: ds,
	}
	req := map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": oversizedLivePromptMessagesWithoutLongLatestUser(),
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	out, err := h.applyHistorySplit(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, stdReq)
	if err != nil {
		t.Fatalf("apply history split failed: %v", err)
	}
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected 2 upload calls, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "CURRENT_INPUT.txt" || ds.uploadCalls[1].Filename != "TASK_STATE.txt" {
		t.Fatalf("unexpected upload order: %#v", ds.uploadCalls)
	}
	if !strings.Contains(out.FinalPrompt, "CURRENT_INPUT.txt") {
		t.Fatalf("expected prompt to reference uploaded current input file, got %s", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, "large assistant context") || strings.Contains(out.FinalPrompt, "large tool context") {
		t.Fatalf("expected oversized live prompt context removed from final prompt, got %s", out.FinalPrompt)
	}
	if got := out.Diagnostics.CurrentInputReason; got != "live_prompt_context_too_large" {
		t.Fatalf("unexpected current input reason: %q", got)
	}
	if got := out.Diagnostics.TaskStateReason; got != "task_continuity_summary_uploaded" {
		t.Fatalf("unexpected task state reason: %q", got)
	}
}

func TestChatCompletionsHistorySplitUploadsHistoryFileAndKeepsLatestPrompt(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected 2 upload calls, got %d", len(ds.uploadCalls))
	}
	upload := ds.uploadCalls[0]
	if upload.Filename != "HISTORY.txt" {
		t.Fatalf("unexpected upload filename: %q", upload.Filename)
	}
	if ds.uploadCalls[1].Filename != "TASK_STATE.txt" {
		t.Fatalf("expected task state upload second, got %#v", ds.uploadCalls)
	}
	if upload.Purpose != "assistants" {
		t.Fatalf("unexpected purpose: %q", upload.Purpose)
	}
	historyText := string(upload.Data)
	if !strings.Contains(historyText, "[file content end]") || !strings.Contains(historyText, "[file name]: IGNORE") {
		t.Fatalf("expected injected IGNORE wrapper, got %s", historyText)
	}
	if strings.Contains(historyText, "latest user turn") {
		t.Fatalf("expected latest turn to remain live, got %s", historyText)
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	promptText, _ := ds.completionReq["prompt"].(string)
	if !strings.Contains(promptText, "latest user turn") {
		t.Fatalf("expected latest turn in completion prompt, got %s", promptText)
	}
	if strings.Contains(promptText, "first user turn") {
		t.Fatalf("expected historical turns removed from completion prompt, got %s", promptText)
	}
	refIDs, _ := ds.completionReq["ref_file_ids"].([]any)
	if len(refIDs) < 2 || refIDs[0] != "file-inline-TASK_STATE" || refIDs[1] != "file-inline-HISTORY" {
		t.Fatalf("expected uploaded history file to be first ref_file_id, got %#v", ds.completionReq["ref_file_ids"])
	}
}

func TestChatCompletionsHistorySplitUploadsLongCurrentInputFile(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": longCurrentInputMessages(),
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
	if len(ds.uploadCalls) != 3 {
		t.Fatalf("expected 3 upload calls, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "CURRENT_INPUT.txt" || ds.uploadCalls[1].Filename != "HISTORY.txt" || ds.uploadCalls[2].Filename != "TASK_STATE.txt" {
		t.Fatalf("unexpected upload order: %#v", ds.uploadCalls)
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	promptText, _ := ds.completionReq["prompt"].(string)
	if !strings.Contains(promptText, "CURRENT_INPUT.txt") {
		t.Fatalf("expected prompt to reference uploaded current input file, got %s", promptText)
	}
	if strings.Contains(promptText, "latest long user turn latest long user turn") {
		t.Fatalf("expected oversized current user text removed from prompt, got %s", promptText)
	}
	refIDs, _ := ds.completionReq["ref_file_ids"].([]any)
	if len(refIDs) < 3 || refIDs[0] != "file-inline-TASK_STATE" || refIDs[1] != "file-inline-HISTORY" || refIDs[2] != "file-inline-CURRENT_INPUT" {
		t.Fatalf("expected history then current-input ref_file_ids, got %#v", ds.completionReq["ref_file_ids"])
	}
}

func TestResponsesHistorySplitUploadsHistoryAndKeepsLatestPrompt(t *testing.T) {
	ds := &inlineUploadDSStub{}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected 2 upload calls, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "HISTORY.txt" || ds.uploadCalls[1].Filename != "TASK_STATE.txt" {
		t.Fatalf("unexpected upload order: %#v", ds.uploadCalls)
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	promptText, _ := ds.completionReq["prompt"].(string)
	if !strings.Contains(promptText, "latest user turn") {
		t.Fatalf("expected latest turn in completion prompt, got %s", promptText)
	}
	if strings.Contains(promptText, "first user turn") {
		t.Fatalf("expected historical turns removed from completion prompt, got %s", promptText)
	}
}

func TestChatCompletionsHistorySplitMapsManagedAuthFailureTo401(t *testing.T) {
	ds := &inlineUploadDSStub{
		uploadErr: &dsclient.RequestFailure{Op: "upload file", Kind: dsclient.FailureManagedUnauthorized, Message: "expired token"},
	}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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

func TestResponsesHistorySplitMapsDirectAuthFailureTo401(t *testing.T) {
	ds := &inlineUploadDSStub{
		uploadErr: &dsclient.RequestFailure{Op: "upload file", Kind: dsclient.FailureDirectUnauthorized, Message: "invalid token"},
	}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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

func TestChatCompletionsHistorySplitUploadFailureReturnsInternalServerError(t *testing.T) {
	ds := &inlineUploadDSStub{uploadErr: errors.New("boom")}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{
			wideInput:           true,
			historySplitEnabled: true,
			historySplitTurns:   1,
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

func TestHistorySplitWorksAcrossAutoDeleteModes(t *testing.T) {
	for _, mode := range []string{"none", "single", "all"} {
		t.Run(mode, func(t *testing.T) {
			ds := &inlineUploadDSStub{}
			h := &openAITestSurface{
				Store: mockOpenAIConfig{
					wideInput:           true,
					autoDeleteMode:      mode,
					historySplitEnabled: true,
					historySplitTurns:   1,
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
			if len(ds.uploadCalls) != 2 {
				t.Fatalf("expected history and task-state uploads for mode=%s, got %d", mode, len(ds.uploadCalls))
			}
			if ds.completionReq == nil {
				t.Fatalf("expected completion payload for mode=%s", mode)
			}
			promptText, _ := ds.completionReq["prompt"].(string)
			if !strings.Contains(promptText, "latest user turn") || strings.Contains(promptText, "first user turn") {
				t.Fatalf("unexpected prompt for mode=%s: %s", mode, promptText)
			}
		})
	}
}

func defaultToolChoicePolicy() promptcompat.ToolChoicePolicy {
	return promptcompat.DefaultToolChoicePolicy()
}
