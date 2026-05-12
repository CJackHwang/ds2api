package promptcompat

import (
	"strings"
	"testing"

	"ds2api/internal/contextengine"
)

func TestBuildOpenAIFinalPrompt_HandlerPathIncludesToolRoundtripSemantics(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "查北京天气"},
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id": "call_1",
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": "{\"city\":\"beijing\"}",
					},
				},
			},
		},
		map[string]any{
			"role":         "tool",
			"tool_call_id": "call_1",
			"name":         "get_weather",
			"content":      map[string]any{"temp": 18, "condition": "sunny"},
		},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	finalPrompt, toolNames := buildOpenAIFinalPrompt(messages, tools, "", false)
	if len(toolNames) != 1 || toolNames[0] != "get_weather" {
		t.Fatalf("unexpected tool names: %#v", toolNames)
	}
	if !strings.Contains(finalPrompt, `"condition":"sunny"`) {
		t.Fatalf("handler finalPrompt should preserve tool output content: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "<｜DSML｜tool_calls>") {
		t.Fatalf("handler finalPrompt should preserve assistant tool history: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, `<｜DSML｜invoke name="get_weather">`) {
		t.Fatalf("handler finalPrompt should include tool name history: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPrompt_VercelPreparePathKeepsFinalAnswerInstruction(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "请调用工具"},
	}
	tools := []any{
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
	}

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
	if !strings.Contains(finalPrompt, "Remember: The ONLY valid way to use tools is the <｜DSML｜tool_calls>...</｜DSML｜tool_calls> block at the end of your response.") {
		t.Fatalf("vercel prepare finalPrompt missing final tool-call anchor instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "TOOL CALL FORMAT") {
		t.Fatalf("vercel prepare finalPrompt missing xml format instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "Do NOT wrap XML in markdown fences") {
		t.Fatalf("vercel prepare finalPrompt missing no-fence xml instruction: %q", finalPrompt)
	}
	if strings.Contains(finalPrompt, "```json") {
		t.Fatalf("vercel prepare finalPrompt should not require fenced tool calls: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPromptPrependsOutputIntegrityGuard(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "请调用工具"},
	}
	tools := []any{
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
	}

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
	guardIdx := strings.Index(finalPrompt, "Output integrity guard")
	toolIdx := strings.Index(finalPrompt, "TOOL CALL FORMAT")
	if guardIdx < 0 {
		t.Fatalf("expected output integrity guard in final prompt, got: %q", finalPrompt)
	}
	if toolIdx < 0 {
		t.Fatalf("expected tool instructions in final prompt, got: %q", finalPrompt)
	}
	if guardIdx > toolIdx {
		t.Fatalf("expected output integrity guard to precede tool instructions, got: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPromptReadLikeToolIncludesCacheGuard(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "请读取文件"},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "read_file",
				"description": "Read a file",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
	if !strings.Contains(finalPrompt, "Read-tool cache guard") {
		t.Fatalf("read-like tool prompt missing cache guard: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "provides no file body") {
		t.Fatalf("read-like tool prompt missing no-body handling: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "Do not repeatedly call the same read request") {
		t.Fatalf("read-like tool prompt missing loop guard: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPromptNonReadToolOmitsCacheGuard(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "搜索一下"},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "search",
				"description": "Search docs",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
	if strings.Contains(finalPrompt, "Read-tool cache guard") {
		t.Fatalf("non-read tool prompt should not include read cache guard: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPromptWithThinkingKeepsPromptUnchanged(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "继续回答上一个问题"},
	}

	finalPromptThinking, _ := buildOpenAIFinalPrompt(messages, nil, "", true)
	finalPromptPlain, _ := buildOpenAIFinalPrompt(messages, nil, "", false)
	if finalPromptThinking != finalPromptPlain {
		t.Fatalf("expected thinking flag not to prepend continuation contract, thinking=%q plain=%q", finalPromptThinking, finalPromptPlain)
	}
}

// --- Stage 6: Cross-theme E2E pipeline tests ---

// TestBuildOpenAIPromptShadowMode verifies the full normalisation →
// context-engine shadow path does not panic and returns a valid prompt.
func TestBuildOpenAIPromptShadowMode(t *testing.T) {
	contextengine.GlobalPlanBuffer().Clear()

	messages := []any{
		map[string]any{"role": "system", "content": "You are a coding assistant."},
		map[string]any{"role": "user", "content": "Read my file"},
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id": "call_1", "type": "function",
					"function": map[string]any{
						"name":      "Read",
						"arguments": `{"file_path":"/tmp/hello.go"}`,
					},
				},
			},
		},
		map[string]any{"role": "tool", "tool_call_id": "call_1", "name": "Read", "content": "package main"},
		map[string]any{"role": "user", "content": "What does it do?"},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "Read",
				"description": "Read a file",
				"parameters":  map[string]any{"type": "object"},
			},
		},
	}

	// shadow mode: context engine compiles in parallel without affecting prompt.
	prompt, toolNames := BuildOpenAIPrompt(messages, tools, "", DefaultToolChoicePolicy(), false, "shadow")
	if len(toolNames) != 1 || toolNames[0] != "Read" {
		t.Errorf("unexpected toolNames: %v", toolNames)
	}
	if !strings.Contains(prompt, "package main") {
		t.Errorf("tool result must survive normalization into prompt, got: %q", prompt[:min(200, len(prompt))])
	}

	plans := contextengine.GlobalPlanBuffer().Snapshot()
	if len(plans) == 0 {
		t.Fatal("expected context engine shadow plan to be captured")
	}
	if plans[0].SegmentsTrimmed != 0 {
		t.Fatalf("valid tool loop should not trim tool segments in shadow plan, got summary: %#v", plans[0])
	}
	for _, warning := range plans[0].Warnings {
		if strings.Contains(warning, "orphan_tool_result") {
			t.Fatalf("valid tool loop must not warn orphan_tool_result, got warnings: %v", plans[0].Warnings)
		}
	}
}

// TestBuildOpenAIPromptReasoningPlusToolPairChain verifies that an assistant
// message with both reasoning_content and tool_calls is normalised and reaches
// the prompt intact in shadow mode.
func TestBuildOpenAIPromptReasoningPlusToolPairChain(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "solve it"},
		map[string]any{
			"role":              "assistant",
			"reasoning_content": "I should search first.",
			"tool_calls": []any{
				map[string]any{
					"id": "call_r1", "type": "function",
					"function": map[string]any{
						"name":      "search",
						"arguments": `{"q":"golang context"}`,
					},
				},
			},
		},
		map[string]any{"role": "tool", "tool_call_id": "call_r1", "name": "search", "content": "Go context docs"},
		map[string]any{"role": "user", "content": "ok thanks"},
	}

	p, _ := BuildOpenAIPrompt(messages, nil, "", DefaultToolChoicePolicy(), false, "shadow")
	// reasoning block must appear in the prompt (normalised by promptcompat).
	if !strings.Contains(p, "I should search first.") {
		t.Errorf("reasoning_content must appear in prompt, got snippet: %q", p[:min(200, len(p))])
	}
	// tool result must appear.
	if !strings.Contains(p, "Go context docs") {
		t.Errorf("tool result must survive into prompt, got snippet: %q", p[:min(200, len(p))])
	}
}

// TestBuildOpenAIPromptForAdapterGeminiPath verifies the Gemini adapter path
// (BuildOpenAIPromptForAdapter) produces the same output as the direct path
// for a standard multi-turn conversation.
func TestBuildOpenAIPromptForAdapterGeminiPath(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "Be brief."},
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "assistant", "content": "hi there"},
		map[string]any{"role": "user", "content": "how are you?"},
	}

	direct, _ := BuildOpenAIPrompt(messages, nil, "", DefaultToolChoicePolicy(), false, "off")
	adapter, _ := BuildOpenAIPromptForAdapter(messages, nil, "", false, "off")
	if direct != adapter {
		t.Errorf("Gemini adapter path must produce same prompt as direct path\ndirect:  %q\nadapter: %q", direct, adapter)
	}
}

// TestResponsesAPIToPromptPipeline verifies that a Responses API input
// (normalised via NormalizeResponsesInputAsMessages) feeds correctly into
// BuildOpenAIPrompt and produces a valid prompt.
func TestResponsesAPIToPromptPipeline(t *testing.T) {
	// Simulated Responses API input: assistant reasoning message followed by
	// a function_call and function_result.
	responsesInput := []any{
		map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "reasoning", "text": "I need to look this up."},
			},
		},
		map[string]any{
			"type":      "function_call",
			"call_id":   "fc_1",
			"name":      "search",
			"arguments": `{"query":"golang docs"}`,
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": "fc_1",
			"output":  "Go documentation results",
		},
		map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": "summarise"}},
		},
	}

	normalised := NormalizeResponsesInputAsMessages(responsesInput)
	if len(normalised) == 0 {
		t.Fatal("NormalizeResponsesInputAsMessages returned empty for valid Responses API input")
	}

	// Wrap as []any for BuildOpenAIPrompt.
	msgs := make([]any, len(normalised))
	copy(msgs, normalised)

	p, _ := BuildOpenAIPrompt(msgs, nil, "", DefaultToolChoicePolicy(), false, "off")
	if !strings.Contains(p, "summarise") {
		t.Errorf("current user message must survive Responses→OpenAI pipeline, got: %q", p[:min(200, len(p))])
	}
}
