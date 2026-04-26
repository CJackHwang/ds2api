package toolcall

import (
	"strings"
	"testing"
)

func TestFormatOpenAIToolCalls(t *testing.T) {
	formatted := FormatOpenAIToolCalls([]ParsedToolCall{{Name: "search", Input: map[string]any{"q": "x"}}})
	if len(formatted) != 1 {
		t.Fatalf("expected 1, got %d", len(formatted))
	}
	fn, _ := formatted[0]["function"].(map[string]any)
	if fn["name"] != "search" {
		t.Fatalf("unexpected function name: %#v", fn)
	}
}

func TestParseToolCallsSupportsToolCallsWrapper(t *testing.T) {
	text := `<tool_calls><invoke name="Bash"><parameter name="command">pwd</parameter><parameter name="description">show cwd</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected original tool name Bash, got %q", calls[0].Name)
	}
	if calls[0].Input["command"] != "pwd" {
		t.Fatalf("expected command argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsDSMLWrapper(t *testing.T) {
	text := `<|DSML|tool_calls><|DSML|invoke name="Bash"><|DSML|parameter name="command" string="true">pwd</|DSML|parameter><|DSML|parameter name="description" string="true">show cwd</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected original tool name Bash, got %q", calls[0].Name)
	}
	if calls[0].Input["command"] != "pwd" || calls[0].Input["description"] != "show cwd" {
		t.Fatalf("expected command and description arguments, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsLooseDSMLWrapper(t *testing.T) {
	text := `<DSML|tool_calls><DSML|invoke name="Bash"><DSML|parameter name="command" string="true">pwd</DSML|parameter><DSML|parameter name="description" string="true">show cwd</DSML|parameter></DSML|invoke></DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected original tool name Bash, got %q", calls[0].Name)
	}
	if calls[0].Input["command"] != "pwd" || calls[0].Input["description"] != "show cwd" {
		t.Fatalf("expected command and description arguments, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsBareDSMLWrapper(t *testing.T) {
	text := `<DSMLtool_calls><DSMLinvoke name="Read"><DSMLparameter name="file_path" string="true">E:/KSWJJ/blog/backend/storage/articles.go</DSMLparameter><DSMLparameter name="limit" string="false">30</DSMLparameter><DSMLparameter name="offset" string="false">295</DSMLparameter></DSMLinvoke></DSMLtool_calls>`
	calls := ParseToolCalls(text, []string{"Read"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Read" {
		t.Fatalf("expected original tool name Read, got %q", calls[0].Name)
	}
	if calls[0].Input["file_path"] != "E:/KSWJJ/blog/backend/storage/articles.go" {
		t.Fatalf("unexpected file_path: %#v", calls[0].Input)
	}
	if got, ok := calls[0].Input["limit"].(float64); !ok || got != 30 {
		t.Fatalf("unexpected limit: %#v", calls[0].Input["limit"])
	}
	if got, ok := calls[0].Input["offset"].(float64); !ok || got != 295 {
		t.Fatalf("unexpected offset: %#v", calls[0].Input["offset"])
	}
}

func TestParseToolCallsSupportsDSMLTypedParameters(t *testing.T) {
	text := `<|DSML|tool_calls><|DSML|invoke name="search"><|DSML|parameter name="top_k" string="false">3</|DSML|parameter><|DSML|parameter name="exact" string="false">true</|DSML|parameter><|DSML|parameter name="filters" string="false">{"lang":"go"}</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if got, ok := calls[0].Input["top_k"].(float64); !ok || got != 3 {
		t.Fatalf("expected numeric top_k=3, got %#v", calls[0].Input["top_k"])
	}
	if got, ok := calls[0].Input["exact"].(bool); !ok || !got {
		t.Fatalf("expected bool exact=true, got %#v", calls[0].Input["exact"])
	}
	filters, ok := calls[0].Input["filters"].(map[string]any)
	if !ok || filters["lang"] != "go" {
		t.Fatalf("expected object filters.lang=go, got %#v", calls[0].Input["filters"])
	}
}

func TestParseToolCallsSupportsStandaloneToolWithMultilineCDATAAndRepeatedXMLTags(t *testing.T) {
	text := `<tool_calls><invoke name="write_file"><parameter name="path">script.sh</parameter><parameter name="content"><![CDATA[#!/bin/bash
echo "hello"
]]></parameter><parameter name="item">first</parameter><parameter name="item">second</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"write_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "write_file" {
		t.Fatalf("expected tool name write_file, got %q", calls[0].Name)
	}
	if calls[0].Input["path"] != "script.sh" {
		t.Fatalf("expected path argument, got %#v", calls[0].Input)
	}
	content, _ := calls[0].Input["content"].(string)
	if !strings.Contains(content, "#!/bin/bash") || !strings.Contains(content, "echo \"hello\"") {
		t.Fatalf("expected multiline CDATA content to be preserved, got %#v", calls[0].Input["content"])
	}
	items, ok := calls[0].Input["item"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected repeated XML tags to become an array, got %#v", calls[0].Input["item"])
	}
}

func TestParseToolCallsKeepsToolSyntaxInsideCDATAAsParameterText(t *testing.T) {
	payload := strings.Join([]string{
		"# Release notes",
		"",
		"```xml",
		"<tool_calls>",
		"  <invoke name=\"demo\">",
		"    <parameter name=\"value\">x</parameter>",
		"  </invoke>",
		"</tool_calls>",
		"```",
	}, "\n")
	text := `<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[` + payload + `]]></parameter><parameter name="file_path">DS2API-4.0-Release-Notes.md</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"Write"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	content, _ := calls[0].Input["content"].(string)
	if content != payload {
		t.Fatalf("expected CDATA payload with nested tool syntax to survive intact, got %q", content)
	}
	if calls[0].Input["file_path"] != "DS2API-4.0-Release-Notes.md" {
		t.Fatalf("expected file_path parameter, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsInvokeParameters(t *testing.T) {
	text := `<tool_calls><invoke name="get_weather"><parameter name="city">beijing</parameter><parameter name="unit">c</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"get_weather"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "get_weather" {
		t.Fatalf("expected tool name get_weather, got %q", calls[0].Name)
	}
	if calls[0].Input["city"] != "beijing" || calls[0].Input["unit"] != "c" {
		t.Fatalf("expected parsed json parameters, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsPreservesRawMalformedParams(t *testing.T) {
	text := `<tool_calls><invoke name="execute_command"><parameter name="command">cd /root && git status</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"execute_command"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "execute_command" {
		t.Fatalf("expected tool name execute_command, got %q", calls[0].Name)
	}
	raw, ok := calls[0].Input["command"].(string)
	if !ok {
		t.Fatalf("expected raw command tracking, got %#v", calls[0].Input)
	}
	if raw != "cd /root && git status" {
		t.Fatalf("expected raw arguments to be preserved, got %q", raw)
	}
}

func TestParseToolCallsSupportsParamsJSONWithAmpersandCommand(t *testing.T) {
	text := `<tool_calls><invoke name="execute_command"><parameter name="command">sshpass -p 'xxx' ssh -o StrictHostKeyChecking=no -p 1111 root@111.111.111.111 'cd /root && git clone https://github.com/ericc-ch/copilot-api.git'</parameter><parameter name="cwd"></parameter><parameter name="timeout"></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"execute_command"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "execute_command" {
		t.Fatalf("expected tool name execute_command, got %q", calls[0].Name)
	}
	cmd, _ := calls[0].Input["command"].(string)
	if !strings.Contains(cmd, "&& git clone") {
		t.Fatalf("expected command to keep && segment, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsDoesNotTreatParamsNameTagAsToolName(t *testing.T) {
	text := `<tool_calls><invoke name="execute_command"><parameter name="tool_name">file.txt</parameter><parameter name="command">pwd</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"execute_command"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "execute_command" {
		t.Fatalf("expected tool name execute_command, got %q", calls[0].Name)
	}
	if calls[0].Input["tool_name"] != "file.txt" {
		t.Fatalf("expected parameter name preserved, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsDetailedMarksToolCallsSyntax(t *testing.T) {
	text := `<tool_calls><invoke name="Bash"><parameter name="command">pwd</parameter></invoke></tool_calls>`
	res := ParseToolCallsDetailed(text, []string{"bash"})
	if !res.SawToolCallSyntax {
		t.Fatalf("expected SawToolCallSyntax=true, got %#v", res)
	}
	if len(res.Calls) != 1 {
		t.Fatalf("expected one parsed call, got %#v", res)
	}
}

func TestParseToolCallsDetailedMarksDSMLToolCallsSyntax(t *testing.T) {
	text := `<|DSML|tool_calls><|DSML|invoke name="Bash"><|DSML|parameter name="command" string="true">pwd</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	res := ParseToolCallsDetailed(text, []string{"bash"})
	if !res.SawToolCallSyntax {
		t.Fatalf("expected SawToolCallSyntax=true, got %#v", res)
	}
	if len(res.Calls) != 1 {
		t.Fatalf("expected one parsed call, got %#v", res)
	}
}

func TestParseToolCallsDetailedMarksLooseDSMLToolCallsSyntax(t *testing.T) {
	text := `<DSML|tool_calls><DSML|invoke name="Bash"><DSML|parameter name="command" string="true">pwd</DSML|parameter></DSML|invoke></DSML|tool_calls>`
	res := ParseToolCallsDetailed(text, []string{"bash"})
	if !res.SawToolCallSyntax {
		t.Fatalf("expected SawToolCallSyntax=true, got %#v", res)
	}
	if len(res.Calls) != 1 {
		t.Fatalf("expected one parsed call, got %#v", res)
	}
}

func TestParseToolCallsSupportsInlineJSONToolObject(t *testing.T) {
	text := `<tool_calls><invoke name="Bash">{"input":{"command":"pwd","description":"show cwd"}}</invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected original tool name Bash, got %q", calls[0].Name)
	}
	if calls[0].Input["command"] != "pwd" {
		t.Fatalf("expected command argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsDoesNotAcceptMismatchedMarkupTags(t *testing.T) {
	text := `<tool_calls><invoke name="read_file"><parameter name="path">README.md</function></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 0 {
		t.Fatalf("expected mismatched tags to be rejected, got %#v", calls)
	}
}

func TestParseToolCallsDoesNotTreatNameInsideParamsAsToolName(t *testing.T) {
	text := `<tool_calls><invoke><parameter name="path">README.md</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 0 {
		t.Fatalf("expected no tool call when name appears only under params, got %#v", calls)
	}
}

func TestParseToolCallsRejectsLegacyToolsWrapper(t *testing.T) {
	text := `<tools><tool_call><tool_name>read_file</tool_name><param>{"path":"README.md"}</param></tool_call></tools>`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 0 {
		t.Fatalf("expected legacy tools wrapper to be rejected, got %#v", calls)
	}
}

func TestParseToolCallsRejectsBareInvokeWithoutToolCallsWrapper(t *testing.T) {
	text := `<invoke name="read_file"><parameter name="path">README.md</parameter></invoke>`
	res := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(res.Calls) != 0 {
		t.Fatalf("expected bare invoke to be rejected, got %#v", res.Calls)
	}
	if res.SawToolCallSyntax {
		t.Fatalf("expected bare invoke to no longer count as supported syntax, got %#v", res)
	}
}

func TestParseToolCallsRepairsMissingOpeningToolCallsWrapperWhenClosingTagExists(t *testing.T) {
	text := `Before tool call
<invoke name="read_file"><parameter name="path">README.md</parameter></invoke>
</tool_calls>
after`
	res := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected repaired wrapper to parse exactly one call, got %#v", res)
	}
	if res.Calls[0].Name != "read_file" {
		t.Fatalf("expected repaired wrapper to preserve tool name, got %#v", res.Calls[0])
	}
	if got, _ := res.Calls[0].Input["path"].(string); got != "README.md" {
		t.Fatalf("expected repaired wrapper to preserve args, got %#v", res.Calls[0].Input)
	}
	if !res.SawToolCallSyntax {
		t.Fatalf("expected repaired wrapper to mark tool syntax seen, got %#v", res)
	}
}

func TestParseToolCallsRejectsLegacyCanonicalBody(t *testing.T) {
	text := `<tool_calls><invoke name="read_file"><tool_name>read_file</tool_name><param>{"path":"README.md"}</param></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 0 {
		t.Fatalf("expected legacy canonical body to be rejected, got %#v", calls)
	}
}

func TestRepairInvalidJSONBackslashes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"path": "C:\Users\name"}`, `{"path": "C:\\Users\name"}`},
		{`{"cmd": "cd D:\git_codes"}`, `{"cmd": "cd D:\\git_codes"}`},
		{`{"text": "line1\nline2"}`, `{"text": "line1\nline2"}`},
		{`{"path": "D:\\back\\slash"}`, `{"path": "D:\\back\\slash"}`},
		{`{"unicode": "\u2705"}`, `{"unicode": "\u2705"}`},
		{`{"invalid_u": "\u123"}`, `{"invalid_u": "\\u123"}`},
	}

	for _, tt := range tests {
		got := repairInvalidJSONBackslashes(tt.input)
		if got != tt.expected {
			t.Errorf("repairInvalidJSONBackslashes(%s) = %s; want %s", tt.input, got, tt.expected)
		}
	}
}

func TestRepairLooseJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{tool_calls: [{"name": "search", "input": {"q": "go"}}]}`, `{"tool_calls": [{"name": "search", "input": {"q": "go"}}]}`},
		{`{name: "search", input: {q: "go"}}`, `{"name": "search", "input": {"q": "go"}}`},
	}

	for _, tt := range tests {
		got := RepairLooseJSON(tt.input)
		if got != tt.expected {
			t.Errorf("RepairLooseJSON(%s) = %s; want %s", tt.input, got, tt.expected)
		}
	}
}

func TestParseToolCallInputRepairsControlCharsInPath(t *testing.T) {
	in := `{"path":"D:\tmp\new\readme.txt","content":"line1\nline2"}`
	parsed := parseToolCallInput(in)

	path, ok := parsed["path"].(string)
	if !ok {
		t.Fatalf("expected path string in parsed input, got %#v", parsed["path"])
	}
	if path != `D:\tmp\new\readme.txt` {
		t.Fatalf("expected repaired windows path, got %q", path)
	}

	content, ok := parsed["content"].(string)
	if !ok {
		t.Fatalf("expected content string in parsed input, got %#v", parsed["content"])
	}
	if content != "line1\nline2" {
		t.Fatalf("expected non-path field to keep decoded escapes, got %q", content)
	}
}

func TestRepairLooseJSONWithNestedObjects(t *testing.T) {
	// 测试嵌套对象的修复：DeepSeek 幻觉输出，每个元素内部包含嵌套 {}
	// 注意：正则只支持单层嵌套，不支持更深层次的嵌套
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// 1. 单层嵌套对象（核心修复目标）
		{
			name:     "单层嵌套 - 2个元素",
			input:    `"todos": {"content": "研究算法", "input": {"q": "8 queens"}}, {"content": "实现", "input": {"path": "queens.py"}}`,
			expected: `"todos": [{"content": "研究算法", "input": {"q": "8 queens"}}, {"content": "实现", "input": {"path": "queens.py"}}]`,
		},
		// 2. 3个单层嵌套对象
		{
			name:     "3个单层嵌套对象",
			input:    `"items": {"a": {"x":1}}, {"b": {"y":2}}, {"c": {"z":3}}`,
			expected: `"items": [{"a": {"x":1}}, {"b": {"y":2}}, {"c": {"z":3}}]`,
		},
		// 3. 混合嵌套：有些字段是对象，有些是原始值
		{
			name:     "混合嵌套 - 对象和原始值混合",
			input:    `"items": {"name": "test", "config": {"timeout": 30}}, {"name": "test2", "config": {"timeout": 60}}`,
			expected: `"items": [{"name": "test", "config": {"timeout": 30}}, {"name": "test2", "config": {"timeout": 60}}]`,
		},
		// 4. 4个嵌套对象（边界测试）
		{
			name:     "4个嵌套对象",
			input:    `"todos": {"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}`,
			expected: `"todos": [{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}]`,
		},
		// 5. DeepSeek 典型幻觉：无空格逗号分隔
		{
			name:     "无空格逗号分隔",
			input:    `"results": {"name": "a"}, {"name": "b"}, {"name": "c"}`,
			expected: `"results": [{"name": "a"}, {"name": "b"}, {"name": "c"}]`,
		},
		// 6. 嵌套数组（数组在对象内，不是深层嵌套）
		{
			name:     "对象内包含数组",
			input:    `"data": {"items": [1,2,3]}, {"items": [4,5,6]}`,
			expected: `"data": [{"items": [1,2,3]}, {"items": [4,5,6]}]`,
		},
		// 7. 真实的 DeepSeek 8皇后问题输出
		{
			name:     "DeepSeek 8皇后真实输出",
			input:    `"todos": {"content": "研究8皇后算法", "status": "pending"}, {"content": "实现Python脚本", "status": "pending"}, {"content": "验证结果", "status": "pending"}`,
			expected: `"todos": [{"content": "研究8皇后算法", "status": "pending"}, {"content": "实现Python脚本", "status": "pending"}, {"content": "验证结果", "status": "pending"}]`,
		},
		// 8. 简单无嵌套对象（回归测试）
		{
			name:     "简单无嵌套对象",
			input:    `"items": {"a": 1}, {"b": 2}`,
			expected: `"items": [{"a": 1}, {"b": 2}]`,
		},
		// 9. 更复杂的单层嵌套
		{
			name:     "复杂单层嵌套",
			input:    `"functions": {"name": "execute", "input": {"command": "ls"}}, {"name": "read", "input": {"file": "a.txt"}}`,
			expected: `"functions": [{"name": "execute", "input": {"command": "ls"}}, {"name": "read", "input": {"file": "a.txt"}}]`,
		},
		// 10. 5个嵌套对象
		{
			name:     "5个嵌套对象",
			input:    `"tasks": {"id":1}, {"id":2}, {"id":3}, {"id":4}, {"id":5}`,
			expected: `"tasks": [{"id":1}, {"id":2}, {"id":3}, {"id":4}, {"id":5}]`,
		},
	}

	for _, tt := range tests {
		got := RepairLooseJSON(tt.input)
		if got != tt.expected {
			t.Errorf("[%s] RepairLooseJSON with nested objects:\n  input:    %s\n  got:      %s\n  expected: %s", tt.name, tt.input, got, tt.expected)
		}
	}
}

func TestParseToolCallsUnescapesHTMLEntityArguments(t *testing.T) {
	text := `<tool_calls><invoke name="Bash"><parameter name="command">echo a &gt; out.txt</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one call, got %#v", calls)
	}
	cmd, _ := calls[0].Input["command"].(string)
	if cmd != "echo a > out.txt" {
		t.Fatalf("expected html entities to be unescaped in command, got %q", cmd)
	}
}

func TestParseToolCallsIgnoresXMLInsideFencedCodeBlock(t *testing.T) {
	text := "Here is an example:\n```xml\n<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>\n```\nDo not execute it."
	res := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(res.Calls) != 0 {
		t.Fatalf("expected no parsed calls for fenced example, got %#v", res.Calls)
	}
}

func TestParseToolCallsParsesOnlyNonFencedXMLToolCall(t *testing.T) {
	text := "```xml\n<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>\n```\n<tool_calls><invoke name=\"search\"><parameter name=\"q\">golang</parameter></invoke></tool_calls>"
	res := ParseToolCallsDetailed(text, []string{"read_file", "search"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected exactly one parsed call outside fence, got %#v", res.Calls)
	}
	if res.Calls[0].Name != "search" {
		t.Fatalf("expected non-fenced tool call to be parsed, got %#v", res.Calls[0])
	}
}

func TestParseToolCallsParsesAfterFourBacktickFence(t *testing.T) {
	text := "````markdown\n```xml\n<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">README.md</parameter></invoke></tool_calls>\n```\n````\n<tool_calls><invoke name=\"search\"><parameter name=\"q\">outside</parameter></invoke></tool_calls>"
	res := ParseToolCallsDetailed(text, []string{"read_file", "search"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected exactly one parsed call outside four-backtick fence, got %#v", res.Calls)
	}
	if res.Calls[0].Name != "search" {
		t.Fatalf("expected non-fenced tool call to be parsed, got %#v", res.Calls[0])
	}
}
