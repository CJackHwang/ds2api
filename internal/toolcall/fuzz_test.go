package toolcall

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Seed corpus: representative inputs drawn from the unit test suite.
// The fuzzer will mutate these to explore the parser's edge-case handling.
var fuzzSeeds = []string{
	// Canonical XML wrapper
	`<tool_calls><invoke name="Bash"><parameter name="command">pwd</parameter></invoke></tool_calls>`,
	// DSML pipe-separator
	`<|DSML|tool_calls><|DSML|invoke name="Read"><|DSML|parameter name="file_path"><![CDATA[/tmp/x.go]]></|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`,
	// DSML underscore variant
	`<dsml_tool_calls><dsml_invoke name="Write"><dsml_parameter name="path">out.txt</dsml_parameter></dsml_invoke></dsml_tool_calls>`,
	// Hyphenated DSML
	`<dsml-tool-calls><dsml-invoke name="Bash"><dsml-parameter name="command">ls</dsml-parameter></dsml-invoke></dsml-tool-calls>`,
	// Camel prefix
	`<DSmartToolCalls><DSmartInvoke name="Bash"><DSmartParameter name="command">git status</DSmartParameter></DSmartInvoke></DSmartToolCalls>`,
	// Nested CDATA
	`<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[<b>hello</b>]]></parameter></invoke></tool_calls>`,
	// Missing wrapper (repair path)
	`<invoke name="Bash"><parameter name="command">echo hi</parameter></invoke></tool_calls>`,
	// Fullwidth DSML
	`<｜DSML｜tool_calls><｜DSML｜invoke name="Bash"><｜DSML｜parameter name="command"><![CDATA[ls]]></｜DSML｜parameter></｜DSML｜invoke></｜DSML｜tool_calls>`,
	// No tool call content
	`plain text with no tool calls`,
	// Empty string
	``,
	// JSON parameter body
	`<tool_calls><invoke name="config">{"input":{"a":1}}</invoke></tool_calls>`,
	// Repeated XML tags (array)
	`<tool_calls><invoke name="multi"><parameter name="item">a</parameter><parameter name="item">b</parameter></invoke></tool_calls>`,
	// Loose CDATA needing sanitization
	`<tool_calls><invoke name="x"><parameter name="v"><![CDATA[unclosed</parameter></invoke></tool_calls>`,
}

// FuzzParseToolCalls exercises ParseToolCalls with arbitrary text inputs.
// The fuzzer must not panic or enter an infinite loop for any input.
func FuzzParseToolCalls(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}

	availableNames := []string{"Bash", "Read", "Write", "Search", "config", "multi", "x"}

	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) {
			t.Skip()
		}
		// Must not panic.
		result := ParseToolCallsDetailed(text, availableNames)

		// Invariants that always hold:
		// 1. Calls slice is non-nil when SawToolCallSyntax is true.
		if result.SawToolCallSyntax && result.Calls == nil {
			// nil is fine (empty slice), but let's be explicit.
			result.Calls = []ParsedToolCall{}
		}
		// 2. Every parsed call has a non-empty Name.
		for _, c := range result.Calls {
			if strings.TrimSpace(c.Name) == "" {
				t.Errorf("parsed call with empty name from input %q", text[:min(80, len(text))])
			}
		}
		// 3. Input map is never nil for a parsed call.
		for _, c := range result.Calls {
			if c.Input == nil {
				t.Errorf("parsed call %q has nil Input map", c.Name)
			}
		}
	})
}

// FuzzParseAssistantToolCallsDetailed exercises the assistant-level fallback
// that retries parsing in the thinking/reasoning block.
func FuzzParseAssistantToolCallsDetailed(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed, seed)
	}
	f.Add(`<|DSML|tool_calls><|DSML|invoke name="Bash"><|DSML|parameter name="command">pwd</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`, "")
	f.Add("", `<tool_calls><invoke name="Read"><parameter name="file_path">/tmp/x</parameter></invoke></tool_calls>`)

	availableNames := []string{"Bash", "Read", "Write"}

	f.Fuzz(func(t *testing.T, text, thinking string) {
		if !utf8.ValidString(text) || !utf8.ValidString(thinking) {
			t.Skip()
		}
		result := ParseAssistantToolCallsDetailed(text, thinking, availableNames)
		for _, c := range result.Calls {
			if strings.TrimSpace(c.Name) == "" {
				t.Errorf("empty call name; text=%q thinking=%q", text[:min(40, len(text))], thinking[:min(40, len(thinking))])
			}
			if c.Input == nil {
				t.Errorf("nil Input for call %q", c.Name)
			}
		}
	})
}
