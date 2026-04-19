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

func TestParseToolCallsSupportsClaudeXMLToolCall(t *testing.T) {
	text := `<tool_call><tool_name>Bash</tool_name><parameters><command>pwd</command><description>show cwd</description></parameters></tool_call>`
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

func TestParseToolCallsSupportsMultilineCDATAAndRepeatedXMLTags(t *testing.T) {
	text := `<tool_call><tool_name>write_file</tool_name><parameters><path>script.sh</path><content><![CDATA[#!/bin/bash
echo "hello"
]]></content><item>first</item><item>second</item></parameters></tool_call>`
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

func TestParseToolCallsSupportsCanonicalXMLParametersJSON(t *testing.T) {
	text := `<tool_call><tool_name>get_weather</tool_name><parameters>{"city":"beijing","unit":"c"}</parameters></tool_call>`
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

func TestParseToolCallsPreservesRawMalformedXMLParameters(t *testing.T) {
	text := `<tool_call><tool_name>execute_command</tool_name><parameters>cd /root && git status</parameters></tool_call>`
	calls := ParseToolCalls(text, []string{"execute_command"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "execute_command" {
		t.Fatalf("expected tool name execute_command, got %q", calls[0].Name)
	}
	raw, ok := calls[0].Input["_raw"].(string)
	if !ok {
		t.Fatalf("expected raw argument tracking, got %#v", calls[0].Input)
	}
	if raw != "cd /root && git status" {
		t.Fatalf("expected raw arguments to be preserved, got %q", raw)
	}
}

func TestParseToolCallsSupportsXMLParametersJSONWithAmpersandCommand(t *testing.T) {
	text := `<tool_calls><tool_call><tool_name>execute_command</tool_name><parameters>{"command":"sshpass -p 'xxx' ssh -o StrictHostKeyChecking=no -p 1111 root@111.111.111.111 'cd /root && git clone https://github.com/ericc-ch/copilot-api.git'","cwd":null,"timeout":null}</parameters></tool_call></tool_calls>`
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

func TestParseToolCallsDoesNotTreatParameterNameTagAsToolName(t *testing.T) {
	text := `<tool_call><tool name="execute_command"><parameters><name>file.txt</name><command>pwd</command></parameters></tool></tool_call>`
	calls := ParseToolCalls(text, []string{"execute_command"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "execute_command" {
		t.Fatalf("expected tool name execute_command, got %q", calls[0].Name)
	}
	if calls[0].Input["name"] != "file.txt" {
		t.Fatalf("expected parameter name preserved, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsDetailedMarksXMLToolCallSyntax(t *testing.T) {
	text := `<tool_call><tool_name>Bash</tool_name><parameters><command>pwd</command></parameters></tool_call>`
	res := ParseToolCallsDetailed(text, []string{"bash"})
	if !res.SawToolCallSyntax {
		t.Fatalf("expected SawToolCallSyntax=true, got %#v", res)
	}
	if len(res.Calls) != 1 {
		t.Fatalf("expected one parsed call, got %#v", res)
	}
}

func TestParseToolCallsSupportsClaudeXMLJSONToolCall(t *testing.T) {
	text := `<tool_call>{"tool":"Bash","params":{"command":"pwd","description":"show cwd"}}</tool_call>`
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

func TestParseToolCallsSupportsFunctionCallTagStyle(t *testing.T) {
	text := `<function_call>Bash</function_call><function parameter name="command">ls -la</function parameter><function parameter name="description">list</function parameter>`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected original tool name Bash, got %q", calls[0].Name)
	}
	if calls[0].Input["command"] != "ls -la" {
		t.Fatalf("expected command argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsAntmlFunctionCallStyle(t *testing.T) {
	text := `<antml:function_calls><antml:function_call name="Bash">{"command":"pwd","description":"x"}</antml:function_call></antml:function_calls>`
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

func TestParseToolCallsSupportsAntmlArgumentStyle(t *testing.T) {
	text := `<antml:function_calls><antml:function_call id="1" name="Bash"><antml:argument name="command">pwd</antml:argument><antml:argument name="description">x</antml:argument></antml:function_call></antml:function_calls>`
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

func TestParseToolCallsSupportsInvokeFunctionCallStyle(t *testing.T) {
	text := `<function_calls><invoke name="Bash"><parameter name="command">pwd</parameter><parameter name="description">d</parameter></invoke></function_calls>`
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

func TestParseToolCallsSupportsToolUseFunctionParameterStyle(t *testing.T) {
	text := `<tool_use><function name="search_web"><parameter name="query">test</parameter></function></tool_use>`
	calls := ParseToolCalls(text, []string{"search_web"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "search_web" {
		t.Fatalf("expected canonical tool name search_web, got %q", calls[0].Name)
	}
	if calls[0].Input["query"] != "test" {
		t.Fatalf("expected query argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsToolUseNameParametersStyle(t *testing.T) {
	text := `<tool_use><tool_name>write_file</tool_name><parameters>{"path":"/tmp/a.txt","content":"abc"}</parameters></tool_use>`
	calls := ParseToolCalls(text, []string{"write_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "write_file" {
		t.Fatalf("expected tool name write_file, got %q", calls[0].Name)
	}
	if calls[0].Input["path"] != "/tmp/a.txt" {
		t.Fatalf("expected path argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsToolUseFunctionNameParametersStyle(t *testing.T) {
	text := `<tool_use><function_name>write_file</function_name><parameters>{"path":"/tmp/b.txt","content":"xyz"}</parameters></tool_use>`
	calls := ParseToolCalls(text, []string{"write_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "write_file" {
		t.Fatalf("expected tool name write_file, got %q", calls[0].Name)
	}
	if calls[0].Input["content"] != "xyz" {
		t.Fatalf("expected content argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsToolUseToolNameBodyStyle(t *testing.T) {
	text := `<tool_use><tool_name>write_file</tool_name><path>/tmp/c.txt</path><content>hello</content></tool_use>`
	calls := ParseToolCalls(text, []string{"write_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "write_file" {
		t.Fatalf("expected tool name write_file, got %q", calls[0].Name)
	}
	if calls[0].Input["path"] != "/tmp/c.txt" {
		t.Fatalf("expected path argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsNestedToolTagStyle(t *testing.T) {
	text := `<tool_call><tool name="Bash"><command>pwd</command><description>show cwd</description></tool></tool_call>`
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

func TestParseToolCallsSupportsAntmlFunctionAttributeWithParametersTag(t *testing.T) {
	text := `<antml:function_calls><antml:function_call id="x" function="Bash"><antml:parameters>{"command":"pwd"}</antml:parameters></antml:function_call></antml:function_calls>`
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

func TestParseToolCallsSupportsMultipleAntmlFunctionCalls(t *testing.T) {
	text := `<antml:function_calls><antml:function_call id="1" function="Bash"><antml:parameters>{"command":"pwd"}</antml:parameters></antml:function_call><antml:function_call id="2" function="Read"><antml:parameters>{"file_path":"README.md"}</antml:parameters></antml:function_call></antml:function_calls>`
	calls := ParseToolCalls(text, []string{"bash", "read"})
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[1].Name != "Read" {
		t.Fatalf("expected original names [Bash Read], got %#v", calls)
	}
}

func TestParseToolCallsDoesNotAcceptMismatchedMarkupTags(t *testing.T) {
	text := `<tool_call><name>read_file</function><arguments>{"path":"README.md"}</arguments></tool_call>`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 0 {
		t.Fatalf("expected mismatched tags to be rejected, got %#v", calls)
	}
}

func TestParseToolCallsDoesNotTreatParametersFunctionNameAsToolName(t *testing.T) {
	text := `<tool_call><parameters><function_name>data_only</function_name><path>README.md</path></parameters></tool_call>`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 0 {
		t.Fatalf("expected no tool call when function_name appears only under parameters, got %#v", calls)
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
	text := `<tool_call><tool_name>Bash</tool_name><parameters>{"command":"echo a &gt; out.txt"}</parameters></tool_call>`
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
	text := "Here is an example:\n```xml\n<tool_call><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\nDo not execute it."
	res := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(res.Calls) != 0 {
		t.Fatalf("expected no parsed calls for fenced example, got %#v", res.Calls)
	}
}

func TestParseToolCallsParsesOnlyNonFencedXMLToolCall(t *testing.T) {
	text := "```xml\n<tool_call><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\n<tool_call><tool_name>search</tool_name><parameters>{\"q\":\"golang\"}</parameters></tool_call>"
	res := ParseToolCallsDetailed(text, []string{"read_file", "search"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected exactly one parsed call outside fence, got %#v", res.Calls)
	}
	if res.Calls[0].Name != "search" {
		t.Fatalf("expected non-fenced tool call to be parsed, got %#v", res.Calls[0])
	}
}

func TestParseToolCallsParsesAfterFourBacktickFence(t *testing.T) {
	text := "````markdown\n```xml\n<tool_call><tool_name>read_file</tool_name><parameters>{\"path\":\"README.md\"}</parameters></tool_call>\n```\n````\n<tool_call><tool_name>search</tool_name><parameters>{\"q\":\"outside\"}</parameters></tool_call>"
	res := ParseToolCallsDetailed(text, []string{"read_file", "search"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected exactly one parsed call outside four-backtick fence, got %#v", res.Calls)
	}
	if res.Calls[0].Name != "search" {
		t.Fatalf("expected non-fenced tool call to be parsed, got %#v", res.Calls[0])
	}
}
