package openai

import (
	"strings"
	"testing"
)

func TestProcessToolSieveInterceptsXMLToolCallWithoutLeak(t *testing.T) {
	var state toolStreamSieveState
	// Simulate a model producing XML tool call output chunk by chunk.
	chunks := []string{
		"<tool_calls>\n",
		"  <tool_call>\n",
		"    <tool_name>read_file</tool_name>\n",
		`    <parameters>{"path":"README.MD"}</parameters>` + "\n",
		"  </tool_call>\n",
		"</tool_calls>",
	}
	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent string
	var toolCalls int
	for _, evt := range events {
		if evt.Content != "" {
			textContent += evt.Content
		}
		toolCalls += len(evt.ToolCalls)
	}

	if strings.Contains(textContent, "<tool_call") {
		t.Fatalf("XML tool call content leaked to text: %q", textContent)
	}
	if strings.Contains(textContent, "read_file") {
		t.Fatalf("tool name leaked to text: %q", textContent)
	}
	if toolCalls == 0 {
		t.Fatal("expected tool calls to be extracted, got none")
	}
}

func TestProcessToolSieveXMLWithLeadingText(t *testing.T) {
	var state toolStreamSieveState
	// Model outputs some prose then an XML tool call.
	chunks := []string{
		"Let me check the file.\n",
		"<tool_calls>\n  <tool_call>\n    <tool_name>read_file</tool_name>\n",
		`    <parameters>{"path":"go.mod"}</parameters>` + "\n  </tool_call>\n</tool_calls>",
	}
	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent string
	var toolCalls int
	for _, evt := range events {
		if evt.Content != "" {
			textContent += evt.Content
		}
		toolCalls += len(evt.ToolCalls)
	}

	// Leading text should be emitted.
	if !strings.Contains(textContent, "Let me check the file.") {
		t.Fatalf("expected leading text to be emitted, got %q", textContent)
	}
	// The XML itself should NOT leak.
	if strings.Contains(textContent, "<tool_call") {
		t.Fatalf("XML tool call content leaked to text: %q", textContent)
	}
	if toolCalls == 0 {
		t.Fatal("expected tool calls to be extracted, got none")
	}
}

func TestProcessToolSievePassesThroughNonToolXMLBlock(t *testing.T) {
	var state toolStreamSieveState
	chunk := `<tool_call><title>示例 XML</title><body>plain text xml payload</body></tool_call>`
	events := processToolSieveChunk(&state, chunk, []string{"read_file"})
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}
	if toolCalls != 0 {
		t.Fatalf("expected no tool calls for plain XML payload, got %d events=%#v", toolCalls, events)
	}
	if textContent.String() != chunk {
		t.Fatalf("expected XML payload to pass through unchanged, got %q", textContent.String())
	}
}

func TestProcessToolSieveNonToolXMLKeepsSuffixForToolParsing(t *testing.T) {
	var state toolStreamSieveState
	chunk := `<tool_call><title>plain xml</title></tool_call><invoke name="read_file"><parameters>{"path":"README.MD"}</parameters></invoke>`
	events := processToolSieveChunk(&state, chunk, []string{"read_file"})
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}
	if !strings.Contains(textContent.String(), `<tool_call><title>plain xml</title></tool_call>`) {
		t.Fatalf("expected leading non-tool XML to be preserved, got %q", textContent.String())
	}
	if strings.Contains(textContent.String(), `<invoke name="read_file">`) {
		t.Fatalf("expected invoke tool XML to be intercepted, got %q", textContent.String())
	}
	if toolCalls != 1 {
		t.Fatalf("expected exactly one parsed tool call from suffix, got %d events=%#v", toolCalls, events)
	}
}

func TestProcessToolSievePassesThroughMalformedExecutableXMLBlock(t *testing.T) {
	var state toolStreamSieveState
	chunk := `<tool_call><parameters>{"path":"README.md"}</parameters></tool_call>`
	events := processToolSieveChunk(&state, chunk, []string{"read_file"})
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected malformed executable-looking XML to stay text, got %d events=%#v", toolCalls, events)
	}
	if textContent.String() != chunk {
		t.Fatalf("expected malformed executable-looking XML to pass through unchanged, got %q", textContent.String())
	}
}

func TestProcessToolSievePassesThroughFencedXMLToolCallExamples(t *testing.T) {
	var state toolStreamSieveState
	input := strings.Join([]string{
		"Before first example.\n```",
		"xml\n<tool_call><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\n",
		"Between examples.\n```xml\n",
		"<tool_call><tool_name>search</tool_name><parameters>{\"q\":\"golang\"}</parameters></tool_call>\n",
		"```\nAfter examples.",
	}, "")

	chunks := []string{
		"Before first example.\n```",
		"xml\n<tool_call><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\n",
		"Between examples.\n```xml\n",
		"<tool_call><tool_name>search</tool_name><parameters>{\"q\":\"golang\"}</parameters></tool_call>\n",
		"```\nAfter examples.",
	}

	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"read_file", "search"})...)
	}
	events = append(events, flushToolSieve(&state, []string{"read_file", "search"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		if evt.Content != "" {
			textContent.WriteString(evt.Content)
		}
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected fenced XML examples to stay text, got %d tool calls events=%#v", toolCalls, events)
	}
	if textContent.String() != input {
		t.Fatalf("expected fenced XML examples to pass through unchanged, got %q", textContent.String())
	}
}

func TestProcessToolSieveKeepsPartialXMLTagInsideFencedExample(t *testing.T) {
	var state toolStreamSieveState
	input := strings.Join([]string{
		"Example:\n```xml\n<tool_ca",
		"ll><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\n",
		"Done.",
	}, "")

	chunks := []string{
		"Example:\n```xml\n<tool_ca",
		"ll><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\n",
		"Done.",
	}

	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		if evt.Content != "" {
			textContent.WriteString(evt.Content)
		}
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected partial fenced XML to stay text, got %d tool calls events=%#v", toolCalls, events)
	}
	if textContent.String() != input {
		t.Fatalf("expected partial fenced XML to pass through unchanged, got %q", textContent.String())
	}
}

func TestProcessToolSievePartialXMLTagHeldBack(t *testing.T) {
	var state toolStreamSieveState
	// Chunk ends with a partial XML tool tag.
	events := processToolSieveChunk(&state, "Hello <tool_ca", []string{"read_file"})

	var textContent string
	for _, evt := range events {
		textContent += evt.Content
	}

	// "Hello " should be emitted, but "<tool_ca" should be held back.
	if strings.Contains(textContent, "<tool_ca") {
		t.Fatalf("partial XML tag should not be emitted, got %q", textContent)
	}
	if !strings.Contains(textContent, "Hello") {
		t.Fatalf("expected 'Hello' text to be emitted, got %q", textContent)
	}
}

func TestFindToolSegmentStartDetectsXMLToolCalls(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"tool_calls_tag", "some text <tool_calls>\n", 10},
		{"tool_call_tag", "prefix <tool_call>\n", 7},
		{"invoke_tag", "text <invoke name=\"foo\">body</invoke>", 5},
		{"xml_inside_code_fence", "```xml\n<tool_call><tool_name>read_file</tool_name></tool_call>\n```", -1},
		{"function_call_tag", "<function_call name=\"foo\">body</function_call>", 0},
		{"no_xml", "just plain text", -1},
		{"gemini_json_no_detect", `some text {"functionCall":{"name":"search"}}`, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findToolSegmentStart(nil, tc.input)
			if got != tc.want {
				t.Fatalf("findToolSegmentStart(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestFindPartialXMLToolTagStart(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"partial_tool_call", "Hello <tool_ca", 6},
		{"partial_invoke", "Prefix <inv", 7},
		{"partial_lt_only", "Text <", 5},
		{"complete_tag", "Text <tool_call>done", -1},
		{"no_lt", "plain text", -1},
		{"closed_lt", "a < b > c", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findPartialXMLToolTagStart(tc.input)
			if got != tc.want {
				t.Fatalf("findPartialXMLToolTagStart(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestHasOpenXMLToolTag(t *testing.T) {
	if !hasOpenXMLToolTag("<tool_call>\n<tool_name>foo</tool_name>") {
		t.Fatal("should detect open XML tool tag without closing tag")
	}
	if hasOpenXMLToolTag("<tool_call>\n<tool_name>foo</tool_name></tool_call>") {
		t.Fatal("should return false when closing tag is present")
	}
	if hasOpenXMLToolTag("plain text without any XML") {
		t.Fatal("should return false for plain text")
	}
}

// Test the EXACT scenario the user reports: token-by-token streaming where
// <tool_calls> tag arrives in small pieces.
func TestProcessToolSieveTokenByTokenXMLNoLeak(t *testing.T) {
	var state toolStreamSieveState
	// Simulate DeepSeek model generating tokens one at a time.
	chunks := []string{
		"<",
		"tool",
		"_calls",
		">\n",
		"  <",
		"tool",
		"_call",
		">\n",
		"    <",
		"tool",
		"_name",
		">",
		"read",
		"_file",
		"</",
		"tool",
		"_name",
		">\n",
		"    <",
		"parameters",
		">",
		`{"path"`,
		`: "README.MD"`,
		`}`,
		"</",
		"parameters",
		">\n",
		"  </",
		"tool",
		"_call",
		">\n",
		"</",
		"tool",
		"_calls",
		">",
	}
	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent string
	var toolCalls int
	for _, evt := range events {
		if evt.Content != "" {
			textContent += evt.Content
		}
		toolCalls += len(evt.ToolCalls)
	}

	if strings.Contains(textContent, "<tool_call") {
		t.Fatalf("XML tool call content leaked to text in token-by-token mode: %q", textContent)
	}
	if strings.Contains(textContent, "tool_calls>") {
		t.Fatalf("closing tag fragment leaked to text: %q", textContent)
	}
	if strings.Contains(textContent, "read_file") {
		t.Fatalf("tool name leaked to text: %q", textContent)
	}
	if toolCalls == 0 {
		t.Fatal("expected tool calls to be extracted, got none")
	}
}

// Test that flushToolSieve on incomplete XML falls back to raw text.
func TestFlushToolSieveIncompleteXMLFallsBackToText(t *testing.T) {
	var state toolStreamSieveState
	// XML block starts but stream ends before completion.
	chunks := []string{
		"<tool_calls>\n",
		"  <tool_call>\n",
		"    <tool_name>read_file</tool_name>\n",
	}
	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"read_file"})...)
	}
	// Stream ends abruptly - flush should NOT dump raw XML.
	events = append(events, flushToolSieve(&state, []string{"read_file"})...)

	var textContent string
	for _, evt := range events {
		if evt.Content != "" {
			textContent += evt.Content
		}
	}

	if textContent != strings.Join(chunks, "") {
		t.Fatalf("expected incomplete XML to fall back to raw text, got %q", textContent)
	}
}

// Test that the opening tag "<tool_calls>\n  " is NOT emitted as text content.
func TestOpeningXMLTagNotLeakedAsContent(t *testing.T) {
	var state toolStreamSieveState
	// First chunk is the opening tag - should be held, not emitted.
	evts1 := processToolSieveChunk(&state, "<tool_calls>\n  ", []string{"read_file"})
	for _, evt := range evts1 {
		if strings.Contains(evt.Content, "<tool_calls>") {
			t.Fatalf("opening tag leaked on first chunk: %q", evt.Content)
		}
	}

	// Remaining content arrives.
	evts2 := processToolSieveChunk(&state, "<tool_call>\n    <tool_name>read_file</tool_name>\n    <parameters>{\"path\":\"README.MD\"}</parameters>\n  </tool_call>\n</tool_calls>", []string{"read_file"})
	evts2 = append(evts2, flushToolSieve(&state, []string{"read_file"})...)

	var textContent string
	var toolCalls int
	allEvents := append(evts1, evts2...)
	for _, evt := range allEvents {
		if evt.Content != "" {
			textContent += evt.Content
		}
		toolCalls += len(evt.ToolCalls)
	}

	if strings.Contains(textContent, "<tool_call") {
		t.Fatalf("XML content leaked: %q", textContent)
	}
	if toolCalls == 0 {
		t.Fatal("expected tool calls to be extracted")
	}
}

func TestProcessToolSieveFallsBackToRawAttemptCompletion(t *testing.T) {
	var state toolStreamSieveState
	// Simulate an agent outputting attempt_completion XML tag.
	// If it does not parse as a tool call, it should fall back to raw text.
	chunks := []string{
		"Done with task.\n",
		"<attempt_completion>\n",
		"  <result>Here is the answer</result>\n",
		"</attempt_completion>",
	}
	var events []toolStreamEvent
	for _, c := range chunks {
		events = append(events, processToolSieveChunk(&state, c, []string{"attempt_completion"})...)
	}
	events = append(events, flushToolSieve(&state, []string{"attempt_completion"})...)

	var textContent string
	for _, evt := range events {
		if evt.Content != "" {
			textContent += evt.Content
		}
	}

	if !strings.Contains(textContent, "Done with task.\n") {
		t.Fatalf("expected leading text to be emitted, got %q", textContent)
	}

	if textContent != strings.Join(chunks, "") {
		t.Fatalf("expected agent XML to fall back to raw text, got %q", textContent)
	}
}
