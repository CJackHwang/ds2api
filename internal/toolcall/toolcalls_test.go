package toolcall

import (
	"strings"
	"testing"
)

func TestFormatOpenAIToolCalls(t *testing.T) {
	formatted := FormatOpenAIToolCalls([]ParsedToolCall{{Name: "search", Input: map[string]any{"q": "x"}}}, nil)
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

func TestParseToolCallsSupportsDSMLShell(t *testing.T) {
	text := `<|DSML|tool_calls><|DSML|invoke name="Bash"><|DSML|parameter name="command"><![CDATA[pwd]]></|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 DSML call, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "pwd" {
		t.Fatalf("unexpected DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsSupportsHyphenatedDSMLShellWithHereDocCDATA(t *testing.T) {
	text := `<dsml-tool-calls>
<dsml-invoke name="Bash">
<dsml-parameter name="command"><![CDATA[git commit -m "$(cat <<'EOF'
docs: add missing directory entries and package descriptions to architecture docs
Fill gaps identified in architecture audit: add artifacts/ and static/ to
directory tree, and document 7 auxiliary internal/ packages (textclean,
claudeconv, compat, rawsample, devcapture, util, version) in Section 3.

Co-Authored-By: Claude Opus 4.7 noreply@anthropic.com
EOF
)"]]></dsml-parameter>
<dsml-parameter name="description"><![CDATA[Create commit with architecture doc updates]]></dsml-parameter>
</dsml-invoke>
</dsml-tool-calls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 hyphenated DSML call, got %#v", calls)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected Bash tool, got %#v", calls[0])
	}
	command, _ := calls[0].Input["command"].(string)
	if !strings.Contains(command, `git commit -m "$(cat <<'EOF'`) || !strings.Contains(command, "Co-Authored-By: Claude Opus 4.7") {
		t.Fatalf("expected here-doc CDATA command to be preserved, got %q", command)
	}
	if calls[0].Input["description"] != "Create commit with architecture doc updates" {
		t.Fatalf("expected description parameter, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsSupportsUnderscoredDSMLShell(t *testing.T) {
	text := `<dsml_tool_calls>
<dsml_invoke name="search_web">
<dsml_parameter name="query"><![CDATA[2026年5月 热点事件]]></dsml_parameter>
<dsml_parameter name="topic"><![CDATA[news]]></dsml_parameter>
</dsml_invoke>
<dsml_invoke name="eval_javascript">
<dsml_parameter name="code"><![CDATA[1 + 1]]></dsml_parameter>
</dsml_invoke>
</dsml_tool_calls>`
	calls := ParseToolCalls(text, []string{"search_web", "eval_javascript"})
	if len(calls) != 2 {
		t.Fatalf("expected two underscored DSML calls, got %#v", calls)
	}
	if calls[0].Name != "search_web" || calls[0].Input["query"] != "2026年5月 热点事件" || calls[0].Input["topic"] != "news" {
		t.Fatalf("unexpected first underscored DSML call: %#v", calls[0])
	}
	if calls[1].Name != "eval_javascript" || calls[1].Input["code"] != "1 + 1" {
		t.Fatalf("unexpected second underscored DSML call: %#v", calls[1])
	}
}

func TestParseToolCallsSupportsArbitraryPrefixedToolMarkup(t *testing.T) {
	cases := []string{
		`<abc|tool_calls><abc|invoke name="Read"><abc|parameter name="file_path">README.md</abc|parameter></abc|invoke></abc|tool_calls>`,
		`<vendor_tool_calls><vendor_invoke name="Read"><vendor_parameter name="file_path">README.md</vendor_parameter></vendor_invoke></vendor_tool_calls>`,
		`<agent - tool_calls><agent - invoke name="Read"><agent - parameter name="file_path">README.md</agent - parameter></agent - invoke></agent - tool_calls>`,
	}
	for _, text := range cases {
		calls := ParseToolCalls(text, []string{"Read"})
		if len(calls) != 1 {
			t.Fatalf("expected one arbitrary-prefixed tool call for %q, got %#v", text, calls)
		}
		if calls[0].Name != "Read" || calls[0].Input["file_path"] != "README.md" {
			t.Fatalf("unexpected arbitrary-prefixed parse result: %#v", calls[0])
		}
	}
}

func TestParseToolCallsSupportsCamelPrefixedToolMarkup(t *testing.T) {
	text := `<DSmartToolCalls><DSmartInvoke name="Bash"><DSmartParameter name="command"><![CDATA[git push]]></DSmartParameter><DSmartParameter name="description"><![CDATA[Push dev branch to origin]]></DSmartParameter></DSmartInvoke></DSmartToolCalls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one camel-prefixed tool call, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "git push" || calls[0].Input["description"] != "Push dev branch to origin" {
		t.Fatalf("unexpected camel-prefixed tool call: %#v", calls[0])
	}
}

func TestParseToolCallsRejectsCamelPrefixedToolMarkupLookalike(t *testing.T) {
	text := `<DSmartToolCallsExtra><DSmartInvoke name="Bash"><DSmartParameter name="command">git push</DSmartParameter></DSmartInvoke></DSmartToolCallsExtra>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 0 {
		t.Fatalf("expected camel-prefixed lookalike to be ignored, got %#v", calls)
	}
}

func TestParseToolCallsSupportsFullwidthDSMLShell(t *testing.T) {
	text := `<ｄＳＭＬ｜tool_calls>
  <ｄＳＭＬ｜invoke name="Read">
    <ｄＳＭＬ｜parameter name="file_path"＞<![CDATA[/Users/aq/Desktop/myproject/Personal_Blog/README.md]]＞</ｄＳＭＬ｜parameter>
  </ｄＳＭＬ｜invoke>
  <ｄＳＭＬ｜invoke name="Read">
    <ｄＳＭＬ｜parameter name="file_path"＞<![CDATA[/Users/aq/Desktop/myproject/Personal_Blog/index.html]]＞</ｄＳＭＬ｜parameter>
  </ｄＳＭＬ｜invoke>
</ｄＳＭＬ｜tool_calls>`
	calls := ParseToolCalls(text, []string{"Read"})
	if len(calls) != 2 {
		t.Fatalf("expected two fullwidth DSML calls, got %#v", calls)
	}
	if calls[0].Name != "Read" || calls[0].Input["file_path"] != "/Users/aq/Desktop/myproject/Personal_Blog/README.md" {
		t.Fatalf("unexpected first fullwidth DSML call: %#v", calls[0])
	}
	if calls[1].Name != "Read" || calls[1].Input["file_path"] != "/Users/aq/Desktop/myproject/Personal_Blog/index.html" {
		t.Fatalf("unexpected second fullwidth DSML call: %#v", calls[1])
	}
}

func TestParseToolCallsSupportsCJKAngleDSMDrift(t *testing.T) {
	text := `<DSM｜tool_calls>
<DSM｜invoke name="Bash">
<DSM｜parameter name="description"｜>〈![CDATA[Show commits on local dev not on origin/dev]]〉〈/DSM｜parameter〉
<DSM｜parameter name="command"｜>〈![CDATA[git log --oneline origin/dev..dev]]〉〈/DSM｜parameter〉
〈/DSM｜invoke〉
<DSM｜invoke name="Bash">
<DSM｜parameter name="description"｜>〈![CDATA[Show commits on origin/dev not on local dev]]〉〈/DSM｜parameter〉
<DSM｜parameter name="command"｜>〈![CDATA[git log --oneline dev..origin/dev]]〉〈/DSM｜parameter〉
〈/DSM｜invoke〉
<DSM｜invoke name="Bash">
<DSM｜parameter name="description"｜>〈![CDATA[Check tracking branch status]]〉〈/DSM｜parameter〉
<DSM｜parameter name="command"｜>〈![CDATA[git status -b --short]]〉〈/DSM｜parameter〉
〈/DSM｜invoke〉
〈/DSM｜tool_calls〉`

	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 3 {
		t.Fatalf("expected three CJK-angle DSM drift calls, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "git log --oneline origin/dev..dev" {
		t.Fatalf("unexpected first CJK-angle DSM drift call: %#v", calls[0])
	}
	if calls[1].Name != "Bash" || calls[1].Input["description"] != "Show commits on origin/dev not on local dev" {
		t.Fatalf("unexpected second CJK-angle DSM drift call: %#v", calls[1])
	}
	if calls[2].Name != "Bash" || calls[2].Input["command"] != "git status -b --short" {
		t.Fatalf("unexpected third CJK-angle DSM drift call: %#v", calls[2])
	}
}

func TestParseToolCallsSupportsFullwidthBangDSMLDrift(t *testing.T) {
	text := `<！DSML！tool_calls>
  <！DSML！invoke name=“Bash”>
  <！DSML！parameter name=“command”><！[CDATA[lsof -i :4321 -t]]><！/DSML！parameter>
  <！DSML！parameter name=“description”><！[CDATA[Verify port 4321 is free]]><！/DSML！parameter>
  <！/DSML！invoke>
  <！/DSML！tool_calls>`

	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one fullwidth-bang DSML drift call, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "lsof -i :4321 -t" || calls[0].Input["description"] != "Verify port 4321 is free" {
		t.Fatalf("unexpected fullwidth-bang DSML drift call: %#v", calls[0])
	}
}

func TestParseToolCallsSupportsIdeographicCommaDSMLDrift(t *testing.T) {
	text := `<、DSML、tool_calls>
  <、DSML、invoke name="Bash">
    <、DSML、parameter name="command"><、[CDATA[git commit -m "$(cat <<'EOF'
feat: expand fullwidth bang separator and curly quote tolerance in DSML tool parsing

Co-Authored-By: Claude Opus 4.6 noreply@anthropic.com
EOF
)"]]><、/DSML、parameter>
    <、DSML、parameter name="description"><、[CDATA[Create commit with staged changes]]><、/DSML、parameter>
  <、/DSML、invoke>
<、/DSML、tool_calls>`

	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one ideographic-comma DSML drift call, got %#v", calls)
	}
	command, _ := calls[0].Input["command"].(string)
	if calls[0].Name != "Bash" || !strings.Contains(command, `git commit -m "$(cat <<'EOF'`) || !strings.Contains(command, "Co-Authored-By: Claude Opus 4.6 noreply@anthropic.com") {
		t.Fatalf("unexpected ideographic-comma DSML drift call: %#v", calls[0])
	}
	if calls[0].Input["description"] != "Create commit with staged changes" {
		t.Fatalf("unexpected ideographic-comma description: %#v", calls[0])
	}
}

func TestParseToolCallsIgnoresBareHyphenatedToolCallsLookalike(t *testing.T) {
	text := `<tool-calls><invoke name="Bash"><parameter name="command">pwd</parameter></invoke></tool-calls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 0 {
		t.Fatalf("expected bare hyphenated lookalike to be ignored, got %#v", calls)
	}
}

func TestParseToolCallsToleratesDSMLTrailingPipeTagTerminator(t *testing.T) {
	text := strings.Join([]string{
		`<|DSML|tool_calls| `,
		`  <|DSML|invoke name="terminal">`,
		`    <|DSML|parameter name="command"><![CDATA[find "/home" -type d]]></|DSML|parameter>`,
		`    <|DSML|parameter name="timeout"><![CDATA[10]]></|DSML|parameter>`,
		`  </|DSML|invoke>`,
		`</|DSML|tool_calls>`,
	}, "\n")
	calls := ParseToolCalls(text, []string{"terminal"})
	if len(calls) != 1 {
		t.Fatalf("expected one trailing-pipe DSML call, got %#v", calls)
	}
	if calls[0].Name != "terminal" {
		t.Fatalf("expected terminal tool, got %#v", calls[0])
	}
	if calls[0].Input["command"] != `find "/home" -type d` {
		t.Fatalf("expected command argument, got %#v", calls[0].Input)
	}
	if calls[0].Input["timeout"] != float64(10) {
		t.Fatalf("expected numeric timeout, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsToleratesDSMLTrailingNovelSeparatorTagTerminator(t *testing.T) {
	text := strings.Join([]string{
		`<DSMLtool_calls※>`,
		`  <DSMLinvoke name="Bash"※>`,
		`    <DSMLparameter name="command"※><![CDATA[pwd]]></DSMLparameter※>`,
		`  </DSMLinvoke※>`,
		`</DSMLtool_calls※>`,
	}, "\n")
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one trailing-separator DSML call, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "pwd" {
		t.Fatalf("unexpected trailing-separator DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsToleratesExtraLeadingLessThanBeforeDSML(t *testing.T) {
	text := `<<|DSML|tool_calls><<|DSML|invoke name="Bash"><<|DSML|parameter name="command"><![CDATA[pwd]]></|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one extra-leading-less-than DSML call, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "pwd" {
		t.Fatalf("unexpected extra-leading-less-than DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsToleratesRepeatedDSMLPrefixNoise(t *testing.T) {
	text := `<<DSML|DSML|tool_calls><<DSML|DSML|invoke name="Bash"><<DSML|DSML|parameter name="command"><![CDATA[git status]]></DSML|DSML|parameter></DSML|DSML|invoke></DSML|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one repeated-prefix DSML call, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "git status" {
		t.Fatalf("unexpected repeated-prefix DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsSupportsDSMLShellWithCanonicalExampleInCDATA(t *testing.T) {
	content := `<tool_calls><invoke name="demo"><parameter name="value">x</parameter></invoke></tool_calls>`
	text := `<|DSML|tool_calls><|DSML|invoke name="Write"><|DSML|parameter name="file_path">notes.md</|DSML|parameter><|DSML|parameter name="content"><![CDATA[` + content + `]]></|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"Write"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 DSML call with XML-looking CDATA, got %#v", calls)
	}
	if calls[0].Name != "Write" || calls[0].Input["content"] != content {
		t.Fatalf("unexpected DSML CDATA parse result: %#v", calls[0])
	}
}

func TestParseToolCallsKeepsHereDocCDATAWithFencedDSMLAndLiteralCDATAEnd(t *testing.T) {
	command := strings.Join([]string{
		"cat > docs/project-value.md << 'ENDOFFILE'",
		"# DS2API project value",
		"",
		"```xml",
		`<|DSML|tool_calls>`,
		`  <|DSML|invoke name="Bash">`,
		`    <|DSML|parameter name="command"><![CDATA[grep -E "error|fail" < input.log 2>&1]]></|DSML|parameter>`,
		`  </|DSML|invoke>`,
		`</|DSML|tool_calls>`,
		"```",
		"",
		"Only the literal `]]>` needs special handling.",
		"",
		"ENDOFFILE",
		`echo "Done. Lines: $(wc -l < docs/project-value.md)"`,
	}, "\n")
	text := `<|DSML|tool_calls><|DSML|invoke name="Bash"><|DSML|parameter name="command"><![CDATA[` + command + `]]></|DSML|parameter><|DSML|parameter name="description"><![CDATA[Write project value doc]]></|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`

	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected one DSML call with extreme heredoc CDATA, got %#v", calls)
	}
	got, _ := calls[0].Input["command"].(string)
	if got != command {
		t.Fatalf("expected full heredoc command to survive, got:\n%q\nwant:\n%q", got, command)
	}
	if calls[0].Input["description"] != "Write project value doc" {
		t.Fatalf("expected sibling parameter after command, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsKeepsCompactCDATAWithImmediateFencedDSML(t *testing.T) {
	content := strings.Join([]string{
		"```xml",
		`<|DSML|tool_calls>`,
		`  <|DSML|invoke name="Bash">`,
		`    <|DSML|parameter name="command"><![CDATA[echo compact]]></|DSML|parameter>`,
		`  </|DSML|invoke>`,
		`</|DSML|tool_calls>`,
		"```",
		"tail",
	}, "\n")
	text := `<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[` + content + `]]></parameter></invoke></tool_calls>`

	calls := ParseToolCalls(text, []string{"Write"})
	if len(calls) != 1 {
		t.Fatalf("expected one compact CDATA call, got %#v", calls)
	}
	if calls[0].Input["content"] != content {
		t.Fatalf("expected compact CDATA content to survive, got %#v", calls[0].Input["content"])
	}
}

func TestParseToolCallsPreservesSimpleCDATAInlineMarkupAsText(t *testing.T) {
	text := `<tool_calls><invoke name="Write"><parameter name="description"><![CDATA[<b>urgent</b>]]></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"Write"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	got, ok := calls[0].Input["description"].(string)
	if !ok {
		t.Fatalf("expected description to remain a string, got %#v", calls[0].Input["description"])
	}
	if got != "<b>urgent</b>" {
		t.Fatalf("expected inline markup CDATA to stay raw, got %q", got)
	}
}

func TestParseToolCallsTreatsUnclosedCDATAAsText(t *testing.T) {
	text := `<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[hello world</parameter></invoke></tool_calls>`
	res := ParseToolCallsDetailed(text, []string{"Write"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected unclosed CDATA to still parse via outer wrapper, got %#v", res.Calls)
	}
	got, _ := res.Calls[0].Input["content"].(string)
	if got != "hello world" {
		t.Fatalf("expected recovered CDATA payload, got %q", got)
	}
}

func TestParseToolCallsNormalizesMixedDSMLAndCanonicalToolTags(t *testing.T) {
	// Models commonly mix DSML wrapper tags with canonical inner tags.
	// These should be normalized and parsed, not rejected.
	text := `<|DSML|tool_calls><invoke name="Bash"><|DSML|parameter name="command">pwd</|DSML|parameter></invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"Bash"})
	if len(calls) != 1 {
		t.Fatalf("expected mixed DSML/XML tool tags to be normalized and parsed, got %#v", calls)
	}
	if calls[0].Name != "Bash" || calls[0].Input["command"] != "pwd" {
		t.Fatalf("unexpected mixed DSML parse result: %#v", calls[0])
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

func TestParseToolCallsSupportsJSONScalarParameters(t *testing.T) {
	text := `<tool_calls><invoke name="configure"><parameter name="count">123</parameter><parameter name="max_tokens"><![CDATA[256]]></parameter><parameter name="enabled">true</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"configure"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if got, ok := calls[0].Input["count"].(float64); !ok || got != 123 {
		t.Fatalf("expected numeric count, got %#v", calls[0].Input["count"])
	}
	if got, ok := calls[0].Input["max_tokens"].(float64); !ok || got != 256 {
		t.Fatalf("expected numeric max_tokens, got %#v", calls[0].Input["max_tokens"])
	}
	if got, ok := calls[0].Input["enabled"].(bool); !ok || !got {
		t.Fatalf("expected boolean enabled, got %#v", calls[0].Input["enabled"])
	}
}

func TestParseToolCallsTreatsItemOnlyParameterBodyAsArray(t *testing.T) {
	text := strings.Join([]string{
		`<|DSML|tool_calls>`,
		`<|DSML|invoke name="AskUserQuestion">`,
		`<|DSML|parameter name="questions">`,
		`<item>`,
		`<question><![CDATA[What would you like to do next?]]></question>`,
		`<header><![CDATA[Next step]]></header>`,
		`<options>`,
		`<item><label><![CDATA[Run tests]]></label><description><![CDATA[Run the test suite]]></description></item>`,
		`<item><label><![CDATA[Other task]]></label><description><![CDATA[Something else entirely]]></description></item>`,
		`</options>`,
		`<multiSelect>false</multiSelect>`,
		`</item>`,
		`</|DSML|parameter>`,
		`</|DSML|invoke>`,
		`</|DSML|tool_calls>`,
	}, "\n")
	calls := ParseToolCalls(text, []string{"AskUserQuestion"})
	if len(calls) != 1 {
		t.Fatalf("expected one AskUserQuestion call, got %#v", calls)
	}
	questions, ok := calls[0].Input["questions"].([]any)
	if !ok || len(questions) != 1 {
		t.Fatalf("expected questions to parse as array, got %#v", calls[0].Input["questions"])
	}
	first, ok := questions[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first question object, got %#v", questions[0])
	}
	if first["question"] != "What would you like to do next?" || first["header"] != "Next step" || first["multiSelect"] != false {
		t.Fatalf("unexpected question payload: %#v", first)
	}
	options, ok := first["options"].([]any)
	if !ok || len(options) != 2 {
		t.Fatalf("expected options to parse as array, got %#v", first["options"])
	}
}

func TestParseToolCallsTreatsCDATAItemOnlyBodyAsArray(t *testing.T) {
	todos := `<br>  <item><br>    <activeForm>Testing EnterWorktree tool</activeForm><br>    <content>Test EnterWorktree tool</content><br>    <status>in_progress</status><br>  </item><br>  <item><br>    <activeForm>Testing TodoWrite tool</activeForm><br>    <content>Test TodoWrite tool</content><br>    <status>completed</status><br>  </item><br>`
	text := `<|DSML|tool_calls><|DSML|invoke name="TodoWrite"><|DSML|parameter name="todos"><![CDATA[` + todos + `]]></|DSML|parameter></|DSML|invoke></|DSML|tool_calls>`
	calls := ParseToolCalls(text, []string{"TodoWrite"})
	if len(calls) != 1 {
		t.Fatalf("expected one TodoWrite call, got %#v", calls)
	}
	items, ok := calls[0].Input["todos"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected todos CDATA item body to parse as array, got %#v", calls[0].Input["todos"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first todo object, got %#v", items[0])
	}
	if first["activeForm"] != "Testing EnterWorktree tool" || first["content"] != "Test EnterWorktree tool" || first["status"] != "in_progress" {
		t.Fatalf("unexpected first todo: %#v", first)
	}
}

func TestParseToolCallsTreatsSingleItemCDATAAsArray(t *testing.T) {
	text := `<tool_calls><invoke name="TodoWrite"><parameter name="todos"><![CDATA[<item>one</item>]]></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"TodoWrite"})
	if len(calls) != 1 {
		t.Fatalf("expected one TodoWrite call, got %#v", calls)
	}
	items, ok := calls[0].Input["todos"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected single-item CDATA body to parse as array, got %#v", calls[0].Input["todos"])
	}
	if got, ok := items[0].(string); !ok || got != "one" {
		t.Fatalf("expected single item value to stay intact, got %#v", items[0])
	}
}

func TestParseToolCallsTreatsLooseJSONListAsArray(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "plain text",
			body: `{"content":"Test TodoWrite tool","status":"completed"}, {"content":"Another task","status":"pending"}`,
		},
		{
			name: "cdata",
			body: `<![CDATA[{"content":"Test TodoWrite tool","status":"completed"}, {"content":"Another task","status":"pending"}]]>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := `<tool_calls><invoke name="TodoWrite"><parameter name="todos">` + tt.body + `</parameter></invoke></tool_calls>`
			calls := ParseToolCalls(text, []string{"TodoWrite"})
			if len(calls) != 1 {
				t.Fatalf("expected one TodoWrite call, got %#v", calls)
			}
			items, ok := calls[0].Input["todos"].([]any)
			if !ok || len(items) != 2 {
				t.Fatalf("expected loose JSON list to parse as array, got %#v", calls[0].Input["todos"])
			}
			first, ok := items[0].(map[string]any)
			if !ok {
				t.Fatalf("expected first todo object, got %#v", items[0])
			}
			if first["content"] != "Test TodoWrite tool" || first["status"] != "completed" {
				t.Fatalf("unexpected first todo: %#v", first)
			}
		})
	}
}

func TestParseToolCallsKeepsPreservedTextParametersAsText(t *testing.T) {
	text := `<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[{"content":"Test TodoWrite tool","status":"completed"}, {"content":"Another task","status":"pending"}]]></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"Write"})
	if len(calls) != 1 {
		t.Fatalf("expected one Write call, got %#v", calls)
	}
	got, ok := calls[0].Input["content"].(string)
	if !ok {
		t.Fatalf("expected content to stay a string, got %#v", calls[0].Input["content"])
	}
	want := `{"content":"Test TodoWrite tool","status":"completed"}, {"content":"Another task","status":"pending"}`
	if got != want {
		t.Fatalf("expected content to stay raw, got %q", got)
	}
}

func TestParseToolCallsTreatsCDATAObjectFragmentAsObject(t *testing.T) {
	payload := `<question><![CDATA[Pick one]]></question><options><item><label><![CDATA[A]]></label></item><item><label><![CDATA[B]]></label></item></options>`
	text := `<tool_calls><invoke name="AskUserQuestion"><parameter name="questions"><![CDATA[` + payload + `]]></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(text, []string{"AskUserQuestion"})
	if len(calls) != 1 {
		t.Fatalf("expected one AskUserQuestion call, got %#v", calls)
	}
	question, ok := calls[0].Input["questions"].(map[string]any)
	if !ok {
		t.Fatalf("expected CDATA XML object fragment to parse as object, got %#v", calls[0].Input["questions"])
	}
	options, ok := question["options"].([]any)
	if question["question"] != "Pick one" || !ok || len(options) != 2 {
		t.Fatalf("unexpected parsed question: %#v", question)
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

func TestParseToolCallsAllowsAllEmptyParameterPayload(t *testing.T) {
	text := `<tool_calls><invoke name="Bash"><parameter name="command"></parameter><parameter name="description">   </parameter><parameter name="timeout"></parameter></invoke></tool_calls>`
	res := ParseToolCallsDetailed(text, []string{"Bash"})
	if !res.SawToolCallSyntax {
		t.Fatalf("expected tool syntax to be detected, got %#v", res)
	}
	if len(res.Calls) != 1 {
		t.Fatalf("expected all-empty payload to be parsed, got %#v", res.Calls)
	}
	if res.Calls[0].Input["command"] != "" || res.Calls[0].Input["description"] != "" || res.Calls[0].Input["timeout"] != "" {
		t.Fatalf("expected empty parameters to be preserved, got %#v", res.Calls[0].Input)
	}
}

func TestParseToolCallsPreservesExplicitZeroArgToolCall(t *testing.T) {
	text := `<tool_calls><invoke name="noop"></invoke></tool_calls>`
	res := ParseToolCallsDetailed(text, []string{"noop"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected zero-arg tool call to remain valid, got %#v", res.Calls)
	}
	if len(res.Calls[0].Input) != 0 {
		t.Fatalf("expected empty input map for zero-arg tool call, got %#v", res.Calls[0].Input)
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

func TestParseToolCallsToleratesDSMLSpaceSeparatorTypo(t *testing.T) {
	text := strings.Join([]string{
		"<|DSML tool_calls>",
		"<|DSML invoke name=\"Read\">",
		"<|DSML parameter name=\"file_path\"><![CDATA[/tmp/input.txt]]></|DSML parameter>",
		"</|DSML invoke>",
		"</|DSML tool_calls>",
	}, "\n")
	calls := ParseToolCalls(text, []string{"Read"})
	if len(calls) != 1 {
		t.Fatalf("expected one call from DSML space-separator typo, got %#v", calls)
	}
	if calls[0].Name != "Read" {
		t.Fatalf("expected Read call, got %#v", calls[0])
	}
	if got, _ := calls[0].Input["file_path"].(string); got != "/tmp/input.txt" {
		t.Fatalf("expected file_path to parse, got %q", got)
	}
}

func TestParseToolCallsDoesNotAcceptDSMLSpaceLookalikeTagName(t *testing.T) {
	text := strings.Join([]string{
		"<|DSML tool_calls_extra>",
		"<|DSML invoke name=\"Read\">",
		"<|DSML parameter name=\"file_path\">/tmp/input.txt</|DSML parameter>",
		"</|DSML invoke>",
		"</|DSML tool_calls_extra>",
	}, "\n")
	calls := ParseToolCalls(text, []string{"Read"})
	if len(calls) != 0 {
		t.Fatalf("expected no calls from lookalike tag, got %#v", calls)
	}
}

func TestParseToolCallsToleratesDSMLCollapsedTagNames(t *testing.T) {
	todos := `[x] 检查 toolcalls_format.go 格式化逻辑
[x] 检查 toolcalls_parse.go 解析逻辑
[x] 检查 toolcalls_xml.go 和 toolcalls_dsml.go
[x] 检查 toolcalls_markup.go 和 toolcalls_json_repair.go
[x] 检查 prompt/tool_calls.go 注入逻辑
[x] 检查 toolstream 流式解析
[x] 查看测试文件确认预期行为
[x] 给出调查结论`
	text := strings.Join([]string{
		"[]",
		"<DSMLtool_calls>",
		"<DSMLinvoke name=\"update_todo_list\">",
		"<DSMLparameter name=\"todos\"><![CDATA[" + todos + "]]></DSMLparameter>",
		"</DSMLinvoke>",
		"</DSMLtool_calls>",
	}, "\n")
	calls := ParseToolCalls(text, []string{"update_todo_list"})
	if len(calls) != 1 {
		t.Fatalf("expected one call from collapsed DSML tags, got %#v", calls)
	}
	if calls[0].Name != "update_todo_list" {
		t.Fatalf("expected update_todo_list call, got %#v", calls[0])
	}
	if got, _ := calls[0].Input["todos"].(string); got != todos {
		t.Fatalf("expected todos to round-trip, got %q", got)
	}
}

func TestParseToolCallsDoesNotAcceptDSMLCollapsedLookalikeTagName(t *testing.T) {
	text := strings.Join([]string{
		"<DSMLtool_calls_extra>",
		"<DSMLinvoke name=\"update_todo_list\">",
		"<DSMLparameter name=\"todos\">x</DSMLparameter>",
		"</DSMLinvoke>",
		"</DSMLtool_calls_extra>",
	}, "\n")
	calls := ParseToolCalls(text, []string{"update_todo_list"})
	if len(calls) != 0 {
		t.Fatalf("expected no calls from collapsed lookalike tag, got %#v", calls)
	}
}

func TestParseToolCallsSkipsProseMentionOfSameWrapperVariant(t *testing.T) {
	text := strings.Join([]string{
		"Summary: support canonical <tool_calls> and DSML <|DSML|tool_calls> wrappers.",
		"",
		"<|DSML|tool_calls>",
		"<|DSML|invoke name=\"Bash\">",
		"<|DSML|parameter name=\"command\"><![CDATA[git status]]></|DSML|parameter>",
		"</|DSML|invoke>",
		"</|DSML|tool_calls>",
	}, "\n")
	res := ParseToolCallsDetailed(text, []string{"Bash"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected one parsed call after prose mention, got %#v", res.Calls)
	}
	if res.Calls[0].Name != "Bash" {
		t.Fatalf("expected Bash call, got %#v", res.Calls[0])
	}
	if got, _ := res.Calls[0].Input["command"].(string); got != "git status" {
		t.Fatalf("expected command to parse, got %q", got)
	}
}

func TestTurkishILowercaseMapping(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		start    int
		wantOk   bool
		wantName string
	}{
		{"turkish_i_at_name_start", "İ<tool>", 0, false, ""},
		{"turkish_i_at_name_end", "<toolİ>", 0, false, ""},
		{"turkish_i_before_tag", "İ<tool>", 0, false, ""},
		{"normal_tool_calls", "<tool_calls>", 0, true, "tool_calls"},
		{"normal_invoke", "<invoke name=\"test\">", 0, true, "invoke"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FindToolMarkupTagOutsideIgnored(tt.text, tt.start)
			if ok != tt.wantOk {
				t.Errorf("FindToolMarkupTagOutsideIgnored(%q, %d) ok = %v, want %v", tt.text, tt.start, ok, tt.wantOk)
				return
			}
			if ok && got.Name != tt.wantName {
				t.Errorf("FindToolMarkupTagOutsideIgnored(%q, %d) name = %q, want %q", tt.text, tt.start, got.Name, tt.wantName)
			}
		})
	}
}

func TestSkipXMLIgnoredSectionBoundaryConditions(t *testing.T) {
	text := "hello"

	tests := []struct {
		name     string
		i        int
		wantNext int
		wantAdv  bool
		wantBlk  bool
	}{
		{"valid_index", 2, 2, false, false},
		{"at_end_equal_len", 5, 5, false, false},
		{"beyond_end", 6, 6, false, false},
		{"negative", -1, -1, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, adv, blk := skipXMLIgnoredSection(text, tt.i)
			if next != tt.wantNext || adv != tt.wantAdv || blk != tt.wantBlk {
				t.Errorf("skipXMLIgnoredSection(%q, %d) = (%d, %v, %v), want (%d, %v, %v)",
					text, tt.i, next, adv, blk, tt.wantNext, tt.wantAdv, tt.wantBlk)
			}
		})
	}
}

func TestSkipXMLIgnoredSectionCommentWithUnicodeKeepsByteOffset(t *testing.T) {
	text := "<!-- İ -->x<tool_calls>"

	next, adv, blk := skipXMLIgnoredSection(text, 0)
	if blk || !adv {
		t.Fatalf("skipXMLIgnoredSection() = (%d, %v, %v), want advanced unblocked comment", next, adv, blk)
	}
	if want := len("<!-- İ -->"); next != want {
		t.Fatalf("skipXMLIgnoredSection() next = %d, want %d", next, want)
	}
}

func TestSkipXMLIgnoredSectionMatchesCDATAWithoutAllocatingTail(t *testing.T) {
	text := "<![cDaTa[<tool_calls>]]><tool_calls>"

	next, adv, blk := skipXMLIgnoredSection(text, 0)
	if blk || !adv {
		t.Fatalf("skipXMLIgnoredSection() = (%d, %v, %v), want advanced unblocked CDATA", next, adv, blk)
	}
	if want := len("<![cDaTa[<tool_calls>]]>"); next != want {
		t.Fatalf("skipXMLIgnoredSection() next = %d, want %d", next, want)
	}

	tag, ok := FindToolMarkupTagOutsideIgnored(text, 0)
	if !ok {
		t.Fatal("expected tool tag after skipped CDATA")
	}
	if tag.Start != next {
		t.Fatalf("FindToolMarkupTagOutsideIgnored() start = %d, want %d", tag.Start, next)
	}
}

func TestFindToolCDATAEndBoundaryConditions(t *testing.T) {
	text := "<![CDATA[hello]]>"

	tests := []struct {
		name       string
		from       int
		wantResult int
	}{
		{"valid", 12, 14},
		{"at_end", 17, -1},
		{"beyond_end", 18, -1},
		{"negative", -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findToolCDATAEnd(text, tt.from)
			if got != tt.wantResult {
				t.Errorf("findToolCDATAEnd(%q, %d) = %d, want %d",
					text, tt.from, got, tt.wantResult)
			}
		})
	}
}

func TestFindMatchingToolMarkupCloseBoundaryConditions(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		open   ToolMarkupTag
		wantOk bool
	}{
		{"empty_text", "", ToolMarkupTag{Name: "tool_calls", End: 0}, false},
		{"open_end_beyond_text", "hello", ToolMarkupTag{Name: "tool_calls", End: 100}, false},
		{"open_end_equals_len", "hello", ToolMarkupTag{Name: "tool_calls", End: 5}, false},
		{"valid_simple", "<tool_calls></tool_calls>", ToolMarkupTag{Name: "tool_calls", End: 11}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := FindMatchingToolMarkupClose(tt.text, tt.open)
			if ok != tt.wantOk {
				t.Errorf("FindMatchingToolMarkupClose(%q, %+v) ok = %v, want %v", tt.text, tt.open, ok, tt.wantOk)
			}
		})
	}
}

func TestParseToolCallsSupportsDSMLShellWithFullwidthClosingSlash(t *testing.T) {
	text := `<｜DSML｜tool_calls><｜DSML｜invoke name="execute_code"><｜DSML｜parameter name="code"><![CDATA[print("hi")]]></｜DSML｜parameter></｜DSML｜invoke><／DSML｜tool_calls>`
	calls := ParseToolCalls(text, []string{"execute_code"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 DSML call with fullwidth closing slash, got %#v", calls)
	}
	if calls[0].Name != "execute_code" || calls[0].Input["code"] != `print("hi")` {
		t.Fatalf("unexpected fullwidth-closing-slash DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsSupportsDSMLShellWithSentencePieceSeparatorAndFullwidthGT(t *testing.T) {
	text := `<｜DSML▁tool_calls｜><｜DSML▁invoke▁name="execute_code"><｜DSML▁parameter▁name="code"><![CDATA[print("hi")]]></｜DSML▁parameter></｜DSML▁invoke></｜DSML▁tool_calls＞`
	calls := ParseToolCalls(text, []string{"execute_code"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 DSML call with sentencepiece separator and fullwidth terminator, got %#v", calls)
	}
	if calls[0].Name != "execute_code" || calls[0].Input["code"] != `print("hi")` {
		t.Fatalf("unexpected sentencepiece/fullwidth-terminator DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsSupportsDSMLShellWithFullwidthLTUnicodeSpaceAndFullwidthAttributes(t *testing.T) {
	text := `＜｜DSML　tool_calls＞＜｜DSML　invoke　name＝“execute_code”＞＜｜DSML　parameter　name＝“code”＞<![CDATA[print("hi")]]>＜／DSML｜parameter＞＜／DSML｜invoke＞＜／DSML｜tool_calls＞`
	calls := ParseToolCalls(text, []string{"execute_code"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 DSML call with fullwidth opening delimiter and Unicode attribute confusables, got %#v", calls)
	}
	if calls[0].Name != "execute_code" || calls[0].Input["code"] != `print("hi")` {
		t.Fatalf("unexpected fullwidth-opening/Unicode-attr DSML parse result: %#v", calls[0])
	}
}

func TestParseToolCallsCanonicalizesConfusableCandidateShellOnly(t *testing.T) {
	text := "<|\u200b\uff24\u0405\u039cL|to\u03bfl\uff3fcalls>" +
		"<|\ufeffDSML|inv\u03bfk\u0435 n\u0430me\uff1d\u201cexecute_code\u201d>" +
		"<|\u200bDSML|par\u0430meter n\u0430me\uff1d\u201ccode\u201d><![\ufeff\u0421D\u0410T\u0410[print(\"hi\")]]>" +
		"</|\u200bDSML|par\u0430meter></|\u200bDSML|inv\u03bfk\u0435></|\u200b\uff24\u0405\u039cL|to\u03bfl\uff3fcalls>"
	calls := ParseToolCalls(text, []string{"execute_code"})
	if len(calls) != 1 {
		t.Fatalf("expected one confusable-shell call, got %#v", calls)
	}
	if calls[0].Name != "execute_code" || calls[0].Input["code"] != `print("hi")` {
		t.Fatalf("unexpected confusable-shell parse result: %#v", calls[0])
	}
}

func TestParseToolCallsKeepsConfusableMarkupInsideCDATAAsText(t *testing.T) {
	value := "<inv\u03bfke>literal</inv\u03bfke>"
	text := "<tool_calls><invoke name=\"Write\"><parameter name=\"description\"><![\u200b\u0421D\u0410T\u0410[" + value + "]]></parameter></invoke></tool_calls>"
	calls := ParseToolCalls(text, []string{"Write"})
	if len(calls) != 1 {
		t.Fatalf("expected one Write call, got %#v", calls)
	}
	if got, _ := calls[0].Input["description"].(string); got != value {
		t.Fatalf("expected confusable markup example inside CDATA to stay raw, got %q", got)
	}
}

func TestParseToolCallsRepairsMissingOpeningWrapperWithConfusableShell(t *testing.T) {
	text := "Before tool call\n" +
		"<inv\u03bfk\u0435 n\u0430me=\"read_file\"><par\u0430meter n\u0430me=\"path\"><![\u200b\u0421D\u0410T\u0410[README.md]]></par\u0430meter></inv\u03bfk\u0435>\n" +
		"</to\u03bfl_calls>\n" +
		"after"
	res := ParseToolCallsDetailed(text, []string{"read_file"})
	if len(res.Calls) != 1 {
		t.Fatalf("expected repaired confusable wrapper to parse one call, got %#v", res)
	}
	if got, _ := res.Calls[0].Input["path"].(string); got != "README.md" {
		t.Fatalf("expected repaired confusable wrapper to preserve args, got %#v", res.Calls[0].Input)
	}
	if !res.SawToolCallSyntax {
		t.Fatalf("expected repaired confusable wrapper to mark tool syntax seen, got %#v", res)
	}
}

func TestParseToolCallsDoesNotAcceptConfusableNearMissTagName(t *testing.T) {
	text := "<tool_calls><inv\u03bfker name=\"execute_code\"><parameter name=\"code\">pwd</parameter></inv\u03bfker></tool_calls>"
	calls := ParseToolCalls(text, []string{"execute_code"})
	if len(calls) != 0 {
		t.Fatalf("expected confusable near-miss tag name to remain non-executable, got %#v", calls)
	}
}

func TestFindMatchingToolMarkupCloseBoundaryConditionsSupportsConfusableDelimiters(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		open   ToolMarkupTag
		wantOk bool
	}{
		{"valid_fullwidth_closing_slash", "<tool_calls><／tool_calls>", ToolMarkupTag{Name: "tool_calls", End: 11}, true},
		{"valid_fullwidth_opening_delimiter", "＜tool_calls＞＜／tool_calls＞", ToolMarkupTag{Name: "tool_calls", End: len("＜tool_calls＞") - 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := FindMatchingToolMarkupClose(tt.text, tt.open)
			if ok != tt.wantOk {
				t.Errorf("FindMatchingToolMarkupClose(%q, %+v) ok = %v, want %v", tt.text, tt.open, ok, tt.wantOk)
			}
		})
	}
}
