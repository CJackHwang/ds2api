package toolcall

import (
	"strings"
	"testing"
)

func TestParseToolCalls(t *testing.T) {
	text := `prefix {"tool_calls":[{"name":"search","input":{"q":"golang"}}]} suffix`
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "search" {
		t.Fatalf("unexpected tool name: %s", calls[0].Name)
	}
	if calls[0].Input["q"] != "golang" {
		t.Fatalf("unexpected args: %#v", calls[0].Input)
	}
}

func TestParseToolCallsIgnoresFencedJSON(t *testing.T) {
	text := "I will call tools now\n```json\n{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"news\"}}]}\n```"
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 0 {
		t.Fatalf("expected fenced tool_call payload to be ignored, got %#v", calls)
	}
}

func TestParseToolCallsWithFunctionArgumentsString(t *testing.T) {
	text := `{"tool_calls":[{"function":{"name":"get_weather","arguments":"{\"city\":\"beijing\"}"}}]}`
	calls := ParseToolCalls(text, []string{"get_weather"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "get_weather" {
		t.Fatalf("unexpected tool name: %s", calls[0].Name)
	}
	if calls[0].Input["city"] != "beijing" {
		t.Fatalf("unexpected args: %#v", calls[0].Input)
	}
}

func TestParseToolCallsKeepsUnknownToolName(t *testing.T) {
	text := `{"tool_calls":[{"name":"unknown","input":{}}]}`
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 1 || calls[0].Name != "unknown" {
		t.Fatalf("expected unknown tool to be preserved, got %#v", calls)
	}
}

func TestParseToolCallsKeepsOriginalToolNameCase(t *testing.T) {
	text := `{"tool_calls":[{"name":"Bash","input":{"command":"ls -al"}}]}`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected original tool name Bash, got %q", calls[0].Name)
	}
}

func TestParseToolCallsDetailedDoesNotRejectByPolicy(t *testing.T) {
	text := `{"tool_calls":[{"name":"unknown","input":{}}]}`
	res := ParseToolCallsDetailed(text, []string{"search"})
	if !res.SawToolCallSyntax {
		t.Fatalf("expected SawToolCallSyntax=true, got %#v", res)
	}
	if res.RejectedByPolicy {
		t.Fatalf("expected RejectedByPolicy=false, got %#v", res)
	}
	if len(res.Calls) != 1 || res.Calls[0].Name != "unknown" {
		t.Fatalf("expected call to be preserved, got %#v", res.Calls)
	}
}

func TestParseToolCallsDetailedAllowsWhenAllowListEmpty(t *testing.T) {
	text := `{"tool_calls":[{"name":"search","input":{"q":"go"}}]}`
	res := ParseToolCallsDetailed(text, nil)
	if !res.SawToolCallSyntax {
		t.Fatalf("expected SawToolCallSyntax=true, got %#v", res)
	}
	if res.RejectedByPolicy {
		t.Fatalf("expected RejectedByPolicy=false, got %#v", res)
	}
	if len(res.Calls) != 1 || res.Calls[0].Name != "search" {
		t.Fatalf("expected calls when allow-list is empty, got %#v", res.Calls)
	}
}

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

func TestParseStandaloneToolCallsSupportsMixedProsePayload(t *testing.T) {
	mixed := `这里是示例：{"tool_calls":[{"name":"search","input":{"q":"go"}}]}`
	if calls := ParseStandaloneToolCalls(mixed, []string{"search"}); len(calls) != 1 {
		t.Fatalf("expected standalone parser to parse mixed prose payload, got %#v", calls)
	}

	standalone := `{"tool_calls":[{"name":"search","input":{"q":"go"}}]}`
	calls := ParseStandaloneToolCalls(standalone, []string{"search"})
	if len(calls) != 1 {
		t.Fatalf("expected standalone parser to match, got %#v", calls)
	}
}

func TestParseStandaloneToolCallsIgnoresFencedCodeBlock(t *testing.T) {
	fenced := "```json\n{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}\n```"
	if calls := ParseStandaloneToolCalls(fenced, []string{"search"}); len(calls) != 0 {
		t.Fatalf("expected fenced tool_call payload to be ignored, got %#v", calls)
	}
}

func TestParseStandaloneToolCallsIgnoresChatTranscriptEnvelope(t *testing.T) {
	transcript := `[{"role":"user","content":"请展示完整会话"},{"role":"assistant","content":null,"tool_calls":[{"function":{"name":"search","arguments":"{\"q\":\"go\"}"}}]}]`
	if calls := ParseStandaloneToolCalls(transcript, []string{"search"}); len(calls) != 0 {
		t.Fatalf("expected transcript envelope not to trigger tool call parse, got %#v", calls)
	}
}

func TestParseToolCallsAllowsQualifiedToolName(t *testing.T) {
	text := `{"tool_calls":[{"name":"mcp.search_web","input":{"q":"golang"}}]}`
	calls := ParseToolCalls(text, []string{"search_web"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "mcp.search_web" {
		t.Fatalf("expected original tool name mcp.search_web, got %q", calls[0].Name)
	}
}

func TestParseToolCallsAllowsPunctuationVariantToolName(t *testing.T) {
	text := `{"tool_calls":[{"name":"read-file","input":{"path":"README.md"}}]}`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "read-file" {
		t.Fatalf("expected original tool name read-file, got %q", calls[0].Name)
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

func TestParseToolCallsPrefersJSONPayloadOverIncidentalXMLInString(t *testing.T) {
	text := `{"tool_calls":[{"name":"search","input":{"q":"latest <tool_call><tool_name>wrong</tool_name><parameters>{\"x\":1}</parameters></tool_call>"}}]}`
	calls := ParseToolCallsDetailed(text, []string{"search"}).Calls
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "search" {
		t.Fatalf("expected tool name search, got %q", calls[0].Name)
	}
	if calls[0].Input["q"] == nil {
		t.Fatalf("expected q argument from json payload, got %#v", calls[0].Input)
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

func TestParseToolCallsSupportsXMLNamedParameterStyle(t *testing.T) {
	text := `<tool_calls><tool_call><tool_name>write_to_file</tool_name><parameters><parameter name="path">novel_chapter1.md</parameter><parameter name="content"><![CDATA[# 第一章：启航

这里是正文
]]></parameter></parameters></tool_call></tool_calls>`
	calls := ParseToolCalls(text, []string{"write_to_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "write_to_file" {
		t.Fatalf("expected tool name write_to_file, got %q", calls[0].Name)
	}
	if calls[0].Input["path"] != "novel_chapter1.md" {
		t.Fatalf("expected path argument, got %#v", calls[0].Input)
	}
	content, _ := calls[0].Input["content"].(string)
	if !strings.Contains(content, "第一章") || !strings.Contains(content, "这里是正文") {
		t.Fatalf("expected content to preserve text, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsGeminiFunctionCallJSON(t *testing.T) {
	text := `{"functionCall":{"name":"search_web","args":{"query":"latest"}}}`
	calls := ParseToolCalls(text, []string{"search_web"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "search_web" {
		t.Fatalf("expected search_web, got %q", calls[0].Name)
	}
	if calls[0].Input["query"] != "latest" {
		t.Fatalf("expected query argument, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsClaudeToolUseJSON(t *testing.T) {
	text := `{"type":"tool_use","name":"read_file","input":{"path":"README.md"}}`
	calls := ParseToolCalls(text, []string{"read_file"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0].Name != "read_file" {
		t.Fatalf("expected read_file, got %q", calls[0].Name)
	}
	if calls[0].Input["path"] != "README.md" {
		t.Fatalf("expected path argument, got %#v", calls[0].Input)
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

func TestParseToolCallsWithUnquotedKeys(t *testing.T) {
	text := `这里是列表：{tool_calls: [{"name": "todowrite", "input": {"todos": "test"}}]}`
	availableTools := []string{"todowrite"}

	parsed := ParseToolCalls(text, availableTools)
	if len(parsed) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(parsed))
	}
	if parsed[0].Name != "todowrite" {
		t.Errorf("expected tool todowrite, got %s", parsed[0].Name)
	}
}

func TestParseToolCallsWithInvalidBackslashes(t *testing.T) {
	// DeepSeek sometimes outputs Windows paths with single backslashes in JSON strings
	// Note: using raw string to simulate what AI actually sends in the stream
	text := `好的，执行以下命令：{"name": "execute_command", "input": "{\"command\": \"cd D:\git_codes && dir\"}"}`
	availableTools := []string{"execute_command"}

	parsed := ParseToolCalls(text, availableTools)
	// If standard JSON fails, buildToolCallCandidates should still extract the object,
	// and parseToolCallsPayload should repair it.
	if len(parsed) != 1 {
		// If it still fails, let's see why
		candidates := buildToolCallCandidates(text)
		t.Logf("Candidates: %v", candidates)
		t.Fatalf("expected 1 tool call, got %d", len(parsed))
	}

	cmd, ok := parsed[0].Input["command"].(string)
	if !ok {
		t.Fatalf("expected command string in input, got %v", parsed[0].Input)
	}

	expected := "cd D:\\git_codes && dir"
	if cmd != expected {
		t.Errorf("expected command %q, got %q", expected, cmd)
	}
}

func TestParseToolCallsWithDeepSeekHallucination(t *testing.T) {
	// 模拟 DeepSeek 典型的幻觉输出：未加引号的键名 + 包含 Windows 路径的嵌套 JSON 字符串 + 漏掉列表的方括号
	text := `检测到实施意图——实现经典算法。需在misc/目录创建Python文件。
关键约束:
1. Windows UTF-8编码处理
2. 必须用绝对路径导入
3. 禁止write覆盖已有文件（misc/目录允许创建新文件）
将任务分解并委托：
- 研究8皇后算法模式（并行探索）
- 实现带可视化输出的解决方案（unspecified-high）
先创建todo列表追踪步骤。
{tool_calls: [{"name": "todowrite", "input": {"todos": {"content": "研究8皇后问题算法模式（回溯法）和输出格式", "status": "pending", "priority": "high"}, {"content": "在misc/目录创建8皇后Python脚本，包含完整解决方案和可视化输出", "status": "pending", "priority": "high"}, {"content": "验证脚本正确性（运行测试）", "status": "pending", "priority": "medium"}}}]}`

	availableTools := []string{"todowrite"}
	parsed := ParseToolCalls(text, availableTools)

	if len(parsed) != 1 {
		cands := buildToolCallCandidates(text)
		for i, c := range cands {
			t.Logf("CAND %d: %s", i, c)
			repaired := RepairLooseJSON(c)
			t.Logf("  REPAIRED: %s", repaired)
		}
		t.Fatalf("expected 1 tool call, got %d. Candidates: %v", len(parsed), buildToolCallCandidates(text))
	}

	if parsed[0].Name != "todowrite" {
		t.Errorf("expected tool name 'todowrite', got %q", parsed[0].Name)
	}

	todos, ok := parsed[0].Input["todos"].([]any)
	if !ok {
		t.Fatalf("expected 'todos' to be parsed as a list, got %T: %#v", parsed[0].Input["todos"], parsed[0].Input["todos"])
	}
	if len(todos) != 3 {
		t.Errorf("expected 3 todo items, got %d", len(todos))
	}
}

func TestParseToolCallsWithMixedWindowsPaths(t *testing.T) {
	// 更复杂的案例：嵌套 JSON 字符串中的反斜杠未转义
	text := `关键约束: 1. Windows UTF-8编码处理 2. 必须用绝对路径导入 D:\git_codes\ds2api\misc
{tool_calls: [{"name": "write_file", "input": "{\"path\": \"D:\\git_codes\\ds2api\\misc\\queens.py\", \"content\": \"print('hello')\"}"}]}`

	availableTools := []string{"write_file"}
	parsed := ParseToolCalls(text, availableTools)

	if len(parsed) != 1 {
		t.Fatalf("expected 1 tool call from mixed text with paths, got %d", len(parsed))
	}

	path, _ := parsed[0].Input["path"].(string)
	// 在解析后的 Go map 中，反斜杠应该被还原
	if !strings.Contains(path, "D:\\git_codes") && !strings.Contains(path, "D:/git_codes") {
		t.Errorf("expected path to contain Windows style separators, got %q", path)
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

func TestParseToolCallsJSONPayloadKeepsLiteralEntities(t *testing.T) {
	text := `{"tool_calls":[{"name":"bash","input":{"command":"echo &gt; literally"}}]}`
	calls := ParseToolCalls(text, []string{"bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one call, got %#v", calls)
	}
	cmd, _ := calls[0].Input["command"].(string)
	if cmd != "echo &gt; literally" {
		t.Fatalf("expected json payload to keep literal entities, got %q", cmd)
	}
}
