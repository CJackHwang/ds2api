package prompt

import "testing"

func TestStringifyToolCallArgumentsPreservesConcatenatedJSON(t *testing.T) {
	got := StringifyToolCallArguments(`{}{"query":"测试工具调用"}`)
	if got != `{}{"query":"测试工具调用"}` {
		t.Fatalf("expected raw concatenated JSON to be preserved, got %q", got)
	}
}

func TestFormatToolCallsForPromptDSML(t *testing.T) {
	got := FormatToolCallsForPrompt([]any{
		map[string]any{
			"id": "call_1",
			"function": map[string]any{
				"name":      "search_web",
				"arguments": map[string]any{"query": "latest"},
			},
		},
	})
	if got == "" {
		t.Fatal("expected non-empty formatted tool calls")
	}
	if got != "<|DSML|tool_calls>\n  <|DSML|invoke name=\"search_web\">\n    <|DSML|parameter name=\"query\" string=\"true\">latest</|DSML|parameter>\n  </|DSML|invoke>\n</|DSML|tool_calls>" {
		t.Fatalf("unexpected formatted tool call DSML: %q", got)
	}
}

func TestFormatToolCallsForPromptEscapesDSMlEntities(t *testing.T) {
	got := FormatToolCallsForPrompt([]any{
		map[string]any{
			"name":      "search<&>",
			"arguments": `{"q":"a < b && c > d"}`,
		},
	})
	want := "<|DSML|tool_calls>\n  <|DSML|invoke name=\"search&lt;&amp;&gt;\">\n    <|DSML|parameter name=\"q\" string=\"true\">a &lt; b &amp;&amp; c &gt; d</|DSML|parameter>\n  </|DSML|invoke>\n</|DSML|tool_calls>"
	if got != want {
		t.Fatalf("unexpected escaped tool call DSML: %q", got)
	}
}

func TestFormatToolCallsForPromptUsesJSONStringForStructuredContent(t *testing.T) {
	got := FormatToolCallsForPrompt([]any{
		map[string]any{
			"name": "write_file",
			"arguments": map[string]any{
				"path":    "script.sh",
				"content": "#!/bin/bash\nprintf \"hello\"\n",
			},
		},
	})
	want := "<|DSML|tool_calls>\n  <|DSML|invoke name=\"write_file\">\n    <|DSML|parameter name=\"content\" string=\"true\">#!/bin/bash\nprintf \"hello\"\n</|DSML|parameter>\n    <|DSML|parameter name=\"path\" string=\"true\">script.sh</|DSML|parameter>\n  </|DSML|invoke>\n</|DSML|tool_calls>"
	if got != want {
		t.Fatalf("unexpected multiline DSML tool call: %q", got)
	}
}
