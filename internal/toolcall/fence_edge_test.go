package toolcall

import (
	"strings"
	"testing"
)

// 4 反引号嵌套 3 反引号
func TestStripFencedCodeBlocks_NestedFourBackticks(t *testing.T) {
	text := "Before\n\x60\x60\x60\x60markdown\nHere is \x60\x60\x60 nested \x60\x60\x60 example\n\x60\x60\x60\x60\nAfter"
	got := stripFencedCodeBlocks(text)
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Fatalf("expected Before and After preserved, got %q", got)
	}
	if strings.Contains(got, "nested") {
		t.Fatalf("expected nested content stripped, got %q", got)
	}
}

// 波浪线围栏
func TestStripFencedCodeBlocks_TildeFence(t *testing.T) {
	text := "Before\n~~~python\ncode here\n~~~\nAfter"
	got := stripFencedCodeBlocks(text)
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Fatalf("expected Before/After, got %q", got)
	}
	if strings.Contains(got, "code here") {
		t.Fatalf("expected code stripped, got %q", got)
	}
}

// 未闭合围栏 + 后面跟真正的工具调用：不应返回空字符串
func TestStripFencedCodeBlocks_UnclosedFencePreservesToolCall(t *testing.T) {
	text := "Example:\n\x60\x60\x60xml\n<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>\n\n<tool_calls><invoke name=\"search\"><parameter name=\"q\">go</parameter></invoke></tool_calls>"
	got := stripFencedCodeBlocks(text)
	if got == "" {
		t.Fatalf("unclosed fence should not truncate everything — real tool call after the fence is lost")
	}
}

// CDATA 内的围栏不应被剥离
func TestStripFencedCodeBlocks_FenceInsideCDATA(t *testing.T) {
	text := "<tool_calls><invoke name=\"write\">\n<parameter name=\"content\"><![CDATA[\n\x60\x60\x60python\nprint('hello')\n\x60\x60\x60\n]]></parameter>\n</invoke></tool_calls>"
	got := stripFencedCodeBlocks(text)
	if !strings.Contains(got, "\x60\x60\x60python") {
		t.Fatalf("fenced code inside CDATA should be preserved, got %q", got)
	}
}

// 连续多个围栏
func TestStripFencedCodeBlocks_MultipleFences(t *testing.T) {
	text := "Before\n\x60\x60\x60\nfence1\n\x60\x60\x60\nMiddle\n\x60\x60\x60\nfence2\n\x60\x60\x60\nAfter"
	got := stripFencedCodeBlocks(text)
	if !strings.Contains(got, "Before") || !strings.Contains(got, "Middle") || !strings.Contains(got, "After") {
		t.Fatalf("expected non-fenced content preserved, got %q", got)
	}
}

// 围栏包含内嵌 ``` 行但没有独立成行
func TestStripFencedCodeBlocks_InlineBackticksNotFence(t *testing.T) {
	text := "Before\n\x60\x60\x60go\nfmt.Println(\x60\x60\x60hello\x60\x60\x60)\n\x60\x60\x60\nAfter"
	got := stripFencedCodeBlocks(text)
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Fatalf("expected Before/After, got %q", got)
	}
}

func TestParseToolCalls_IgnoresMarkdownDocumentationExamples(t *testing.T) {
	text := "解析器支持多种工具调用格式。\n\n" +
		"入口函数 `ParseToolCalls(text, availableToolNames)` 会返回调用列表。\n\n" +
		"核心流程会解析 XML 格式的 `<tool_calls>` / `<invoke>` 标记。\n\n" +
		"### 标准 XML 结构\n" +
		"```xml\n" +
		"<tool_calls>\n" +
		"  <invoke name=\"read_file\">\n" +
		"    <parameter name=\"path\">config.json</parameter>\n" +
		"  </invoke>\n" +
		"</tool_calls>\n" +
		"```\n\n" +
		"DSML 风格形如 `<invoke name=\"tool\">...</invoke>`，也可能提到 `<tool_calls>` 包裹。\n"

	got := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(got.Calls) != 0 {
		t.Fatalf("markdown documentation examples should not parse as tool calls, got %#v", got.Calls)
	}
}

func TestParseToolCalls_IgnoresInlineMarkdownToolCallExample(t *testing.T) {
	text := "示例：`<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>`"

	got := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(got.Calls) != 0 {
		t.Fatalf("inline markdown tool example should not parse as tool calls, got %#v", got.Calls)
	}
}

func TestParseToolCalls_PreservesBackticksInsideToolParameters(t *testing.T) {
	text := "<tool_calls><invoke name=\"Bash\"><parameter name=\"command\">echo `date`</parameter></invoke></tool_calls>"

	got := ParseToolCallsDetailed(text, []string{"Bash"})
	if len(got.Calls) != 1 {
		t.Fatalf("expected one tool call, got %#v", got.Calls)
	}
	if got.Calls[0].Input["command"] != "echo `date`" {
		t.Fatalf("expected command backticks preserved, got %#v", got.Calls[0].Input["command"])
	}
}
