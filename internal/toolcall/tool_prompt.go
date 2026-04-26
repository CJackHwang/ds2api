package toolcall

import "strings"

// BuildToolCallInstructions generates the unified tool-calling instruction block
// used by all adapters (OpenAI, Claude, Gemini). The prompt now aligns with the
// official DeepSeek DSML wrapper syntax while the parser normalizes DSML back
// into the existing executable XML representation internally.
func BuildToolCallInstructions(toolNames []string) string {
	return `TOOL CALL FORMAT — FOLLOW EXACTLY:

<|DSML|tool_calls>
  <|DSML|invoke name="TOOL_NAME_HERE">
    <|DSML|parameter name="PARAMETER_NAME" string="true|false">PARAMETER_VALUE</|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>

RULES:
1) Use the official <|DSML|tool_calls> wrapper format only.
2) Put one or more <|DSML|invoke> entries under a single <|DSML|tool_calls> root.
3) Put the tool name in the invoke name attribute: <|DSML|invoke name="TOOL_NAME">.
4) Every top-level argument must be a <|DSML|parameter name="ARG_NAME" string="...">...</|DSML|parameter> node.
5) For string parameters, write the value as-is and set string="true".
6) For numbers, booleans, arrays, objects, and null, encode the value in JSON and set string="false".
7) Use only the parameter names in the tool schema. Do not invent fields.
8) Do NOT wrap DSML in markdown fences. Do NOT output explanations, role markers, or internal monologue around the tool block.
9) If you call a tool, the first non-whitespace characters of that tool block must be exactly <|DSML|tool_calls>.
10) Never omit the opening <|DSML|tool_calls> tag, even if you already plan to close with </|DSML|tool_calls>.
11) If thinking is enabled, put all reasoning inside <think>...</think> before any DSML tool block or final response.

PARAMETER SHAPES:
- string => <|DSML|parameter name="x" string="true">value</|DSML|parameter>
- object => <|DSML|parameter name="x" string="false">{"k":"v"}</|DSML|parameter>
- array => <|DSML|parameter name="x" string="false">["a","b"]</|DSML|parameter>
- number/bool/null => <|DSML|parameter name="x" string="false">plain_json_literal</|DSML|parameter>

【WRONG — Do NOT do these】:

Wrong 1 — mixed text after DSML:
  <|DSML|tool_calls>...</|DSML|tool_calls> I hope this helps.
Wrong 2 — Markdown code fences:
  ` + "```xml" + `
  <|DSML|tool_calls>...</|DSML|tool_calls>
  ` + "```" + `
Wrong 3 — missing opening wrapper:
  <|DSML|invoke name="TOOL_NAME">...</|DSML|invoke>
  </|DSML|tool_calls>

Remember: The ONLY valid way to use tools is the <|DSML|tool_calls>...</|DSML|tool_calls> block at the end of your response.

` + buildCorrectToolExamples(toolNames)
}

type promptToolExample struct {
	name   string
	params string
}

func buildCorrectToolExamples(toolNames []string) string {
	names := uniqueToolNames(toolNames)
	examples := make([]string, 0, 4)

	if single, ok := firstBasicExample(names); ok {
		examples = append(examples, "Example A — Single tool:\n"+renderToolExampleBlock([]promptToolExample{single}))
	}

	if parallel := firstNBasicExamples(names, 2); len(parallel) >= 2 {
		examples = append(examples, "Example B — Two tools in parallel:\n"+renderToolExampleBlock(parallel))
	}

	if nested, ok := firstNestedExample(names); ok {
		examples = append(examples, "Example C — Tool with JSON object/array parameters:\n"+renderToolExampleBlock([]promptToolExample{nested}))
	}

	if script, ok := firstScriptExample(names); ok {
		examples = append(examples, "Example D — Tool with long script string:\n"+renderToolExampleBlock([]promptToolExample{script}))
	}

	if len(examples) == 0 {
		return ""
	}
	return "【CORRECT EXAMPLES】:\n\n" + strings.Join(examples, "\n\n") + "\n\n"
}

func uniqueToolNames(toolNames []string) []string {
	names := make([]string, 0, len(toolNames))
	seen := map[string]bool{}
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func firstBasicExample(names []string) (promptToolExample, bool) {
	for _, name := range names {
		if params, ok := exampleBasicParams(name); ok {
			return promptToolExample{name: name, params: params}, true
		}
	}
	return promptToolExample{}, false
}

func firstNBasicExamples(names []string, count int) []promptToolExample {
	out := make([]promptToolExample, 0, count)
	for _, name := range names {
		if params, ok := exampleBasicParams(name); ok {
			out = append(out, promptToolExample{name: name, params: params})
			if len(out) == count {
				return out
			}
		}
	}
	return out
}

func firstNestedExample(names []string) (promptToolExample, bool) {
	for _, name := range names {
		if params, ok := exampleNestedParams(name); ok {
			return promptToolExample{name: name, params: params}, true
		}
	}
	return promptToolExample{}, false
}

func firstScriptExample(names []string) (promptToolExample, bool) {
	for _, name := range names {
		if params, ok := exampleScriptParams(name); ok {
			return promptToolExample{name: name, params: params}, true
		}
	}
	return promptToolExample{}, false
}

func renderToolExampleBlock(calls []promptToolExample) string {
	var b strings.Builder
	b.WriteString("<|DSML|tool_calls>\n")
	for _, call := range calls {
		b.WriteString(`  <|DSML|invoke name="`)
		b.WriteString(call.name)
		b.WriteString("\">\n")
		b.WriteString(indentPromptParameters(call.params, "    "))
		b.WriteString("\n  </|DSML|invoke>\n")
	}
	b.WriteString("</|DSML|tool_calls>")
	return b.String()
}

func indentPromptParameters(body, indent string) string {
	if strings.TrimSpace(body) == "" {
		return indent + `<|DSML|parameter name="content" string="true"></|DSML|parameter>`
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = line
			continue
		}
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func wrapStringParameter(name, value string) string {
	return `<|DSML|parameter name="` + name + `" string="true">` + escapeDSMLText(value) + `</|DSML|parameter>`
}

func wrapJSONParameter(name, value string) string {
	return `<|DSML|parameter name="` + name + `" string="false">` + value + `</|DSML|parameter>`
}

func exampleBasicParams(name string) (string, bool) {
	switch strings.TrimSpace(name) {
	case "Read":
		return wrapStringParameter("file_path", "README.md"), true
	case "Glob":
		return wrapStringParameter("pattern", "**/*.go") + "\n" + wrapStringParameter("path", "."), true
	case "read_file":
		return wrapStringParameter("path", "src/main.go"), true
	case "list_files":
		return wrapStringParameter("path", "."), true
	case "search_files":
		return wrapStringParameter("query", "tool call parser"), true
	case "Bash", "execute_command":
		return wrapStringParameter("command", "pwd"), true
	case "exec_command":
		return wrapStringParameter("cmd", "pwd"), true
	case "Write":
		return wrapStringParameter("file_path", "notes.txt") + "\n" + wrapStringParameter("content", "Hello world"), true
	case "write_to_file":
		return wrapStringParameter("path", "notes.txt") + "\n" + wrapStringParameter("content", "Hello world"), true
	case "Edit":
		return wrapStringParameter("file_path", "README.md") + "\n" + wrapStringParameter("old_string", "foo") + "\n" + wrapStringParameter("new_string", "bar"), true
	case "MultiEdit":
		return wrapStringParameter("file_path", "README.md") + "\n" + wrapJSONParameter("edits", `[{"old_string":"foo","new_string":"bar"}]`), true
	}
	return "", false
}

func exampleNestedParams(name string) (string, bool) {
	switch strings.TrimSpace(name) {
	case "MultiEdit":
		return wrapStringParameter("file_path", "README.md") + "\n" + wrapJSONParameter("edits", `[{"old_string":"foo","new_string":"bar"}]`), true
	case "Task":
		return wrapStringParameter("description", "Investigate flaky tests") + "\n" + wrapStringParameter("prompt", "Run targeted tests and summarize failures"), true
	case "ask_followup_question":
		return wrapStringParameter("question", "Which approach do you prefer?") + "\n" + wrapJSONParameter("follow_up", `[{"text":"Option A"},{"text":"Option B"}]`), true
	}
	return "", false
}

func exampleScriptParams(name string) (string, bool) {
	scriptCommand := "cat > /tmp/test_escape.sh <<'EOF'\n#!/bin/bash\necho 'single \"double\"'\necho \"literal dollar: $HOME\"\nEOF\nbash /tmp/test_escape.sh"
	scriptContent := "#!/bin/bash\necho 'single \"double\"'\necho \"literal dollar: $HOME\""

	switch strings.TrimSpace(name) {
	case "Bash":
		return wrapStringParameter("command", scriptCommand) + "\n" + wrapStringParameter("description", "Test shell escaping"), true
	case "execute_command":
		return wrapStringParameter("command", scriptCommand), true
	case "exec_command":
		return wrapStringParameter("cmd", scriptCommand), true
	case "Write":
		return wrapStringParameter("file_path", "test_escape.sh") + "\n" + wrapStringParameter("content", scriptContent), true
	case "write_to_file":
		return wrapStringParameter("path", "test_escape.sh") + "\n" + wrapStringParameter("content", scriptContent), true
	}
	return "", false
}

func escapeDSMLText(text string) string {
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(text)
}
