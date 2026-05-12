package toolstream

import (
	"strings"
	"testing"
)

// 波浪线围栏内的工具调用标签不应触发工具调用
func TestProcessToolSieveTildeFenceDoesNotTriggerToolCall(t *testing.T) {
	var state State
	chunks := []string{
		"示例：\n~~~xml\n",
		"<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>\n",
		"~~~\n",
		"完毕。",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected tilde-fenced tool example to stay text, got %d tool calls", toolCalls)
	}
	if !strings.Contains(textContent.String(), "示例") || !strings.Contains(textContent.String(), "完毕") {
		t.Fatalf("expected surrounding text preserved, got %q", textContent.String())
	}
}

// 4 反引号嵌套 3 反引号（内含工具标签）不应触发
func TestProcessToolSieveNestedFourBacktickFenceDoesNotTrigger(t *testing.T) {
	var state State
	input := "说明：\n````xml\n```\n<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">x</parameter></invoke></tool_calls>\n```\n````\n结束。"
	chunks := strings.SplitAfter(input, "\n")
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected 4-backtick fenced example to stay text, got %d tool calls", toolCalls)
	}
}

func TestProcessToolSieveMarkdownDocumentationExamplesDoNotTrigger(t *testing.T) {
	var state State
	chunks := []string{
		"解析器支持多种工具调用格式。\n\n",
		"入口函数 `ParseToolCalls(text, availableToolNames)` 会返回调用列表。\n\n",
		"核心流程会解析 XML 格式的 `<tool_calls>` / `<invoke>` 标记。\n\n",
		"### 标准 XML 结构\n",
		"```xml\n",
		"<tool_calls>\n",
		"  <invoke name=\"read_file\">\n",
		"    <parameter name=\"path\">config.json</parameter>\n",
		"  </invoke>\n",
		"</tool_calls>\n",
		"```\n\n",
		"DSML 风格形如 `<invoke name=\"tool\">...</invoke>`，也可能提到 `<tool_calls>` 包裹。\n",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected markdown documentation examples to stay text, got %d tool calls", toolCalls)
	}
	if !strings.Contains(textContent.String(), "标准 XML 结构") || !strings.Contains(textContent.String(), "DSML 风格") {
		t.Fatalf("expected documentation text preserved, got %q", textContent.String())
	}
}

func TestProcessToolSieveInlineMarkdownToolCallSplitAcrossChunksDoesNotTrigger(t *testing.T) {
	var state State
	chunks := []string{
		"示例：`",
		"<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>",
		"` 完毕。",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var textContent strings.Builder
	toolCalls := 0
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		toolCalls += len(evt.ToolCalls)
	}

	if toolCalls != 0 {
		t.Fatalf("expected split inline markdown tool example to stay text, got %d tool calls", toolCalls)
	}
	if !strings.Contains(textContent.String(), "<tool_calls>") || !strings.Contains(textContent.String(), "完毕") {
		t.Fatalf("expected inline example text preserved, got %q", textContent.String())
	}
}

func TestProcessToolSieveUnclosedInlineMarkdownBeforeToolDoesTrigger(t *testing.T) {
	var state State
	input := "note with stray ` before real call " +
		"<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">real.md</parameter></invoke></tool_calls>"

	var events []Event
	events = append(events, ProcessChunk(&state, input, []string{"read_file"})...)
	events = append(events, Flush(&state, []string{"read_file"})...)

	var textContent strings.Builder
	var calls []string
	for _, evt := range events {
		textContent.WriteString(evt.Content)
		for _, call := range evt.ToolCalls {
			if path, _ := call.Input["path"].(string); path != "" {
				calls = append(calls, path)
			}
		}
	}

	if len(calls) != 1 || calls[0] != "real.md" {
		t.Fatalf("expected real tool call after stray backtick, got %#v from events %#v", calls, events)
	}
	if !strings.Contains(textContent.String(), "stray ` before real call") {
		t.Fatalf("expected stray-backtick prefix preserved, got %q", textContent.String())
	}
}

func TestProcessToolSieveUnclosedInlineMarkdownBeforeSplitToolDoesTriggerOnFlush(t *testing.T) {
	var state State
	chunks := []string{
		"note with stray ` before real call ",
		"<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">real.md</parameter></invoke></tool_calls>",
	}

	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var calls []string
	for _, evt := range events {
		for _, call := range evt.ToolCalls {
			if path, _ := call.Input["path"].(string); path != "" {
				calls = append(calls, path)
			}
		}
	}

	if len(calls) != 1 || calls[0] != "real.md" {
		t.Fatalf("expected split real tool call after stray backtick, got %#v from events %#v", calls, events)
	}
}
