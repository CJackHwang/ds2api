package openai

import "testing"

func TestSanitizeLeakedOutputRemovesEmptyJSONFence(t *testing.T) {
	raw := "before\n```json\n```\nafter"
	got := sanitizeLeakedOutput(raw)
	if got != "before\n\nafter" {
		t.Fatalf("unexpected sanitized empty json fence: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesLeakedWireToolCallAndResult(t *testing.T) {
	raw := "开始\n[{\"function\":{\"arguments\":\"{\\\"command\\\":\\\"java -version\\\"}\",\"name\":\"exec\"},\"id\":\"callb9a321\",\"type\":\"function\"}]< | Tool | >{\"content\":\"openjdk version 21\",\"tool_call_id\":\"callb9a321\"}\n结束"
	got := sanitizeLeakedOutput(raw)
	if got != "开始\n\n结束" {
		t.Fatalf("unexpected sanitize result for leaked wire format: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesStandaloneMetaMarkers(t *testing.T) {
	raw := "A<| end_of_sentence |><| Assistant |>B<| end_of_thinking |>C<|end▁of▁thinking|>D<|end▁of▁sentence|>E<| end_of_toolresults |>F<|end▁of▁instructions|>G"
	got := sanitizeLeakedOutput(raw)
	if got != "ABCDEFG" {
		t.Fatalf("unexpected sanitize result for meta markers: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesFullwidthDelimitedMetaMarkers(t *testing.T) {
	fw := "\uff5c"
	raw := "A<" + fw + "end▁of▁sentence" + fw + ">B<" + fw + " Assistant " + fw + ">C<" + fw + "end_of_toolresults" + fw + ">D"
	got := sanitizeLeakedOutput(raw)
	if got != "ABCD" {
		t.Fatalf("unexpected sanitize result for fullwidth-delimited meta markers: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesAssistantEndOfToolCallsMarkers(t *testing.T) {
	fw := "\uff5c"
	raw := "A<|Assistant_END_OF_TOOL_CALLS|>B<" + fw + "Assistant▁END▁OF▁TOOL_CALLS" + fw + ">C<|end_of_tool_calls|>D"
	got := sanitizeLeakedOutput(raw)
	if got != "ABCD" {
		t.Fatalf("unexpected sanitize result for assistant end-of-tool-calls markers: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesFullToolResultSection(t *testing.T) {
	fw := "\uff5c"
	raw := "开始<" + fw + "Tool" + fw + ">[{\"content\":\"openjdk version 21\"}]<" + fw + "end▁of▁toolresults" + fw + ">结束"
	got := sanitizeLeakedOutput(raw)
	if got != "开始结束" {
		t.Fatalf("unexpected sanitize result for leaked tool result section: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesThinkAndBosMarkers(t *testing.T) {
	raw := "A<think>B</think>C<|begin▁of▁sentence|>D<| begin_of_sentence |>E<|begin_of_sentence|>F"
	got := sanitizeLeakedOutput(raw)
	if got != "ABCDEF" {
		t.Fatalf("unexpected sanitize result for think/BOS markers: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesThoughtMarkers(t *testing.T) {
	raw := "A<|▁of▁thought|>B<| of_thought |>C<| begin_of_thought |>D<| end_of_thought |>E"
	got := sanitizeLeakedOutput(raw)
	if got != "ABCDE" {
		t.Fatalf("unexpected sanitize result for leaked thought markers: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesFullwidthDelimitedBosAndThoughtMarkers(t *testing.T) {
	fw := "\uff5c"
	raw := "A<" + fw + "begin▁of▁sentence" + fw + ">B<" + fw + "▁of▁thought" + fw + ">C<" + fw + " begin_of_thought " + fw + ">D"
	got := sanitizeLeakedOutput(raw)
	if got != "ABCD" {
		t.Fatalf("unexpected sanitize result for fullwidth-delimited BOS/thought markers: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesDanglingThinkBlock(t *testing.T) {
	raw := "Answer prefix<think>internal reasoning that never closes"
	got := sanitizeLeakedOutput(raw)
	if got != "Answer prefix" {
		t.Fatalf("unexpected sanitize result for dangling think block: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesCompleteDSMLToolCallWrapper(t *testing.T) {
	raw := "前置文本\n<|DSML|tool_calls>\n<|DSML|invoke name=\"Bash\">\n<|DSML|parameter name=\"command\"></|DSML|parameter>\n</|DSML|invoke>\n</|DSML|tool_calls>\n后置文本"
	got := sanitizeLeakedOutput(raw)
	if got != "前置文本\n\n后置文本" {
		t.Fatalf("unexpected sanitize result for leaked dsml wrapper: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesAgentXMLLeaks(t *testing.T) {
	raw := "Done.<attempt_completion><result>Some final answer</result></attempt_completion>"
	got := sanitizeLeakedOutput(raw)
	if got != "Done.Some final answer" {
		t.Fatalf("unexpected sanitize result for agent XML leak: %q", got)
	}
}

func TestSanitizeLeakedOutputPreservesStandaloneResultTags(t *testing.T) {
	raw := "Example XML: <result>value</result>"
	got := sanitizeLeakedOutput(raw)
	if got != raw {
		t.Fatalf("unexpected sanitize result for standalone result tag: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesDanglingAgentXMLOpeningTags(t *testing.T) {
	raw := "Done.<attempt_completion><result>Some final answer"
	got := sanitizeLeakedOutput(raw)
	if got != "Done.Some final answer" {
		t.Fatalf("unexpected sanitize result for dangling opening tags: %q", got)
	}
}

func TestSanitizeLeakedOutputRemovesDanglingAgentXMLClosingTags(t *testing.T) {
	raw := "Done.Some final answer</result></attempt_completion>"
	got := sanitizeLeakedOutput(raw)
	if got != "Done.Some final answer" {
		t.Fatalf("unexpected sanitize result for dangling closing tags: %q", got)
	}
}

func TestSanitizeLeakedOutputPreservesUnrelatedResultTagsWhenWrapperLeaks(t *testing.T) {
	raw := "Done.<attempt_completion><result>Some final answer\nExample XML: <result>value</result>"
	got := sanitizeLeakedOutput(raw)
	want := "Done.Some final answer\nExample XML: <result>value</result>"
	if got != want {
		t.Fatalf("unexpected sanitize result for mixed leaked wrapper + xml example: %q", got)
	}
}
