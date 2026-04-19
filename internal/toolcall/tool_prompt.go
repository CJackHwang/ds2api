package toolcall

import "strings"

// BuildToolCallInstructions generates the unified tool-calling instruction block
// used by all adapters (OpenAI, Claude, Gemini). It uses attention-optimized
// structure: rules → negative examples → positive examples → anchor.
//
// The toolNames slice should contain the actual tool names available in the
// current request; the function picks real names for examples.
func BuildToolCallInstructions(toolNames []string) string {
	// Pick real tool names for examples; fall back to generic names.
	ex1 := "read_file"
	ex2 := "write_to_file"
	ex3 := "ask_followup_question"
	used := map[string]bool{}
	for _, n := range toolNames {
		switch {
		// Read/query-type tools
		case !used["ex1"] && matchAny(n, "read_file", "list_files", "search_files", "Read", "Glob"):
			ex1 = n
			used["ex1"] = true
		// Write/execute-type tools
		case !used["ex2"] && matchAny(n, "write_to_file", "apply_diff", "execute_command", "exec_command", "Write", "Edit", "MultiEdit", "Bash"):
			ex2 = n
			used["ex2"] = true
		// Interactive/meta tools
		case !used["ex3"] && matchAny(n, "ask_followup_question", "attempt_completion", "update_todo_list", "Task"):
			ex3 = n
			used["ex3"] = true
		}
	}
	ex1Params := exampleReadParams(ex1)
	ex2Params := exampleWriteOrExecParams(ex2)
	ex3Params := exampleInteractiveParams(ex3)

	return `TOOL CALL FORMAT — FOLLOW EXACTLY:

<tool_calls>
  <tool_call>
    <tool_name>TOOL_NAME_HERE</tool_name>
    <parameters>
      <PARAMETER_NAME><![CDATA[PARAMETER_VALUE]]></PARAMETER_NAME>
    </parameters>
  </tool_call>
</tool_calls>

RULES:
1) Use the <tool_calls> XML format only. Never emit JSON or function-call syntax.
2) Put one or more <tool_call> entries under a single <tool_calls> root.
3) Parameters must be XML, not JSON.
4) All string values must use <![CDATA[...]]>, even short ones. This includes code, scripts, file contents, prompts, paths, names, and queries.
5) Objects use nested XML elements. Arrays may repeat the same tag or use <item> children.
6) Numbers, booleans, and null stay plain text.
7) Use only the parameter names in the tool schema. Do not invent fields.
8) Do NOT wrap XML in markdown fences. Do NOT output explanations, role markers, or internal monologue.

PARAMETER SHAPES:
- string => <name><![CDATA[value]]></name>
- object => nested XML elements
- array => repeated tags or <item> children
- number/bool/null => plain text

【WRONG — Do NOT do these】:

Wrong 1 — mixed text after XML:
  <tool_calls>...</tool_calls> I hope this helps.
Wrong 2 — function-call syntax:
  Grep({"pattern": "token"})
Wrong 3 — JSON parameters:
  <tool_call><tool_name>` + ex1 + `</tool_name><parameters>{"path":"x"}</parameters></tool_call>
Wrong 4 — Markdown code fences:
  ` + "```xml" + `
  <tool_calls>...</tool_calls>
  ` + "```" + `

Remember: The ONLY valid way to use tools is the <tool_calls> XML block at the end of your response.

【CORRECT EXAMPLES】:

Example A — Single tool:
<tool_calls>
  <tool_call>
    <tool_name>` + ex1 + `</tool_name>
    <parameters>` + ex1Params + `</parameters>
  </tool_call>
</tool_calls>

Example B — Two tools in parallel:
<tool_calls>
  <tool_call>
    <tool_name>` + ex1 + `</tool_name>
    <parameters>` + ex1Params + `</parameters>
  </tool_call>
  <tool_call>
    <tool_name>` + ex2 + `</tool_name>
    <parameters>` + ex2Params + `</parameters>
  </tool_call>
</tool_calls>

Example C — Tool with nested XML parameters:
<tool_calls>
  <tool_call>
    <tool_name>` + ex3 + `</tool_name>
    <parameters>` + ex3Params + `</parameters>
  </tool_call>
</tool_calls>
 
Example D — Tool with long script using CDATA (RELIABLE FOR CODE/SCRIPTS):
<tool_calls>
  <tool_call>
    <tool_name>` + ex2 + `</tool_name>
    <parameters>
      <path>` + promptCDATA("script.sh") + `</path>
      <content><![CDATA[
#!/bin/bash
if [ "$1" == "test" ]; then
  echo "Success!"
fi
]]></content>
    </parameters>
  </tool_call>
</tool_calls>

`
}

func matchAny(name string, candidates ...string) bool {
	for _, c := range candidates {
		if name == c {
			return true
		}
	}
	return false
}

func exampleReadParams(name string) string {
	switch strings.TrimSpace(name) {
	case "Read":
		return `<file_path>` + promptCDATA("README.md") + `</file_path>`
	case "Glob":
		return `<pattern>` + promptCDATA("**/*.go") + `</pattern><path>` + promptCDATA(".") + `</path>`
	default:
		return `<path>` + promptCDATA("src/main.go") + `</path>`
	}
}

func exampleWriteOrExecParams(name string) string {
	switch strings.TrimSpace(name) {
	case "Bash", "execute_command":
		return `<command>` + promptCDATA("pwd") + `</command>`
	case "exec_command":
		return `<cmd>` + promptCDATA("pwd") + `</cmd>`
	case "Edit":
		return `<file_path>` + promptCDATA("README.md") + `</file_path><old_string>` + promptCDATA("foo") + `</old_string><new_string>` + promptCDATA("bar") + `</new_string>`
	case "MultiEdit":
		return `<file_path>` + promptCDATA("README.md") + `</file_path><edits><old_string>` + promptCDATA("foo") + `</old_string><new_string>` + promptCDATA("bar") + `</new_string></edits>`
	default:
		return `<path>` + promptCDATA("output.txt") + `</path><content>` + promptCDATA("Hello world") + `</content>`
	}
}

func exampleInteractiveParams(name string) string {
	switch strings.TrimSpace(name) {
	case "Task":
		return `<description>` + promptCDATA("Investigate flaky tests") + `</description><prompt>` + promptCDATA("Run targeted tests and summarize failures") + `</prompt>`
	default:
		return `<question>` + promptCDATA("Which approach do you prefer?") + `</question><follow_up><text>` + promptCDATA("Option A") + `</text></follow_up><follow_up><text>` + promptCDATA("Option B") + `</text></follow_up>`
	}
}

func promptCDATA(text string) string {
	if text == "" {
		return ""
	}
	if strings.Contains(text, "]]>") {
		return "<![CDATA[" + strings.ReplaceAll(text, "]]>", "]]]]><![CDATA[>") + "]]>"
	}
	return "<![CDATA[" + text + "]]>"
}
