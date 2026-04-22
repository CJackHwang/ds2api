package openai

import "testing"

type mockOpenAIConfig struct {
	aliases            map[string]string
	wideInput          bool
	autoDeleteMode     string
	autoDeleteLeaseSet bool
	autoDeleteLease    bool
	toolMode           string
	earlyEmit          string
	responsesTTL       int
	embedProv          string
}

func (m mockOpenAIConfig) ModelAliases() map[string]string { return m.aliases }
func (m mockOpenAIConfig) CompatWideInputStrictOutput() bool {
	return m.wideInput
}
func (m mockOpenAIConfig) CompatStripReferenceMarkers() bool   { return true }
func (m mockOpenAIConfig) ToolcallMode() string                { return m.toolMode }
func (m mockOpenAIConfig) ToolcallEarlyEmitConfidence() string { return m.earlyEmit }
func (m mockOpenAIConfig) ResponsesStoreTTLSeconds() int       { return m.responsesTTL }
func (m mockOpenAIConfig) EmbeddingsProvider() string          { return m.embedProv }
func (m mockOpenAIConfig) AutoDeleteMode() string {
	if m.autoDeleteMode == "" {
		return "none"
	}
	return m.autoDeleteMode
}
func (m mockOpenAIConfig) AutoDeleteSessions() bool { return false }
func (m mockOpenAIConfig) AcquireAutoDeleteAllLease(string) bool {
	if m.autoDeleteLeaseSet {
		return m.autoDeleteLease
	}
	return true
}

func TestNormalizeOpenAIChatRequestWithConfigInterface(t *testing.T) {
	cfg := mockOpenAIConfig{
		aliases: map[string]string{
			"my-model": "deepseek-chat-search",
		},
		wideInput: true,
	}
	req := map[string]any{
		"model":    "my-model",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}
	out, err := normalizeOpenAIChatRequest(cfg, req, "")
	if err != nil {
		t.Fatalf("normalizeOpenAIChatRequest error: %v", err)
	}
	if out.ResolvedModel != "deepseek-chat-search" {
		t.Fatalf("resolved model mismatch: got=%q", out.ResolvedModel)
	}
	if !out.Search || out.Thinking {
		t.Fatalf("unexpected model flags: thinking=%v search=%v", out.Thinking, out.Search)
	}
}

func TestNormalizeOpenAIResponsesRequestWideInputPolicyFromInterface(t *testing.T) {
	req := map[string]any{
		"model": "deepseek-chat",
		"input": "hi",
	}

	_, err := normalizeOpenAIResponsesRequest(mockOpenAIConfig{
		aliases:   map[string]string{},
		wideInput: false,
	}, req, "")
	if err == nil {
		t.Fatal("expected error when wide input is disabled and only input is provided")
	}

	out, err := normalizeOpenAIResponsesRequest(mockOpenAIConfig{
		aliases:   map[string]string{},
		wideInput: true,
	}, req, "")
	if err != nil {
		t.Fatalf("unexpected error when wide input is enabled: %v", err)
	}
	if out.Surface != "openai_responses" {
		t.Fatalf("unexpected surface: %q", out.Surface)
	}
}
