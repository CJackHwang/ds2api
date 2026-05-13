package toolstream

import (
	"strings"
	"testing"
)

// ---- 错位工具块 ----

// 只有 </tool_calls> 没有 <tool_calls>
func TestSieve_MismatchedClose_OnlyClosingTag(t *testing.T) {
	var state State
	chunks := []string{
		"一些正文内容\n",
		"</tool_calls>\n",
		"后续内容",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var text strings.Builder
	tc := 0
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 0 {
		t.Fatalf("孤立闭合标签不应触发工具调用，got %d", tc)
	}
	if !strings.Contains(text.String(), "一些正文") || !strings.Contains(text.String(), "后续内容") {
		t.Fatalf("应保留所有文本, got %q", text.String())
	}
}

// <tool_calls> 打开后跟的不是 <invoke> 而是普通文本
func TestSieve_ToolCallsWrapperWithNoInvoke(t *testing.T) {
	var state State
	chunks := []string{
		"<tool_calls>\n",
		"这里没有 invoke 标签\n",
		"</tool_calls>\n",
		"后续内容",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var text strings.Builder
	tc := 0
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 0 {
		t.Fatalf("无 invoke 不应触发工具调用，got %d", tc)
	}
}

// 两个连续工具调用块
func TestSieve_TwoConsecutiveToolCallBlocks(t *testing.T) {
	var state State
	chunks := []string{
		`<tool_calls><invoke name="read_file"><parameter name="path">a.txt</parameter></invoke></tool_calls>`,
		"\n",
		`<tool_calls><invoke name="read_file"><parameter name="path">b.txt</parameter></invoke></tool_calls>`,
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	tc := 0
	for _, e := range events {
		tc += len(e.ToolCalls)
	}
	if tc != 2 {
		t.Fatalf("应解析出两个工具调用，got %d, events=%#v", tc, events)
	}
}

// ---- 围栏内的工具调用不应触发 ----

// 反引号围栏内有完整工具调用 + 围栏外有真正的工具调用
func TestSieve_FencedExampleThenRealToolCall(t *testing.T) {
	var state State
	chunks := []string{
		"示例：\n```xml\n",
		`<tool_calls><invoke name="fake"><parameter name="x">1</parameter></invoke></tool_calls>`,
		"\n```\n",
		`<tool_calls><invoke name="read_file"><parameter name="path">real.txt</parameter></invoke></tool_calls>`,
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file", "fake"})...)
	}
	events = append(events, Flush(&state, []string{"read_file", "fake"})...)

	var text strings.Builder
	tc := 0
	var names []string
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			tc++
			names = append(names, call.Name)
		}
	}
	if tc != 1 {
		t.Fatalf("应只触发围栏外的工具调用，got %d, names=%v", tc, names)
	}
	if names[0] != "read_file" {
		t.Fatalf("应触发 read_file，got %v", names)
	}
	if !strings.Contains(text.String(), "示例") {
		t.Fatalf("围栏前文本应保留, got %q", text.String())
	}
}

// 波浪线围栏包裹工具调用
func TestSieve_TildeFencedToolCallIgnored(t *testing.T) {
	var state State
	chunks := []string{
		"~~~\n",
		`<tool_calls><invoke name="read_file"><parameter name="path">x</parameter></invoke></tool_calls>`,
		"\n~~~\n",
		"结束",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	tc := 0
	var text strings.Builder
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 0 {
		t.Fatalf("波浪线围栏内工具调用不应触发，got %d", tc)
	}
	if !strings.Contains(text.String(), "结束") {
		t.Fatalf("围栏后文本应保留, got %q", text.String())
	}
}

// 4 反引号嵌套 3 反引号，内含工具标签
func TestSieve_FourBacktickNestedThreeWithToolCall(t *testing.T) {
	var state State
	chunks := []string{
		"````markdown\n",
		"```xml\n",
		`<tool_calls><invoke name="read_file"><parameter name="path">x</parameter></invoke></tool_calls>`,
		"\n```\n",
		"````\n",
		"外部文本",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	tc := 0
	var text strings.Builder
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 0 {
		t.Fatalf("4反引号嵌套内的工具调用不应触发，got %d", tc)
	}
	if !strings.Contains(text.String(), "外部文本") {
		t.Fatalf("围栏外文本应保留, got %q", text.String())
	}
}

// ---- DSML 变体在围栏内不触发 ----

func TestSieve_DSMLInsideFenceIgnored(t *testing.T) {
	var state State
	chunks := []string{
		"```\n",
		"<|DSML|tool_calls>\n",
		`<|DSML|invoke name="read_file">`,
		`<|DSML|parameter name="path">x</|DSML|parameter>`,
		"</|DSML|invoke>\n",
		"</|DSML|tool_calls>\n",
		"```\n",
		"结束",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	tc := 0
	for _, e := range events {
		tc += len(e.ToolCalls)
	}
	if tc != 0 {
		t.Fatalf("围栏内的 DSML 工具调用不应触发，got %d", tc)
	}
}

// ---- 工具调用前后有丰富文本 ----

func TestSieve_RichTextAroundToolCall(t *testing.T) {
	var state State
	chunks := []string{
		"我来帮你查看文件内容。\n\n",
		"首先读取 README：\n",
		`<tool_calls><invoke name="read_file"><parameter name="path">README.md</parameter></invoke></tool_calls>`,
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var text strings.Builder
	tc := 0
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 1 {
		t.Fatalf("应有一个工具调用，got %d", tc)
	}
	if !strings.Contains(text.String(), "帮你查看") {
		t.Fatalf("前置文本丢失, got %q", text.String())
	}
	if strings.Contains(text.String(), "<invoke") {
		t.Fatalf("工具标签泄漏, got %q", text.String())
	}
}

// ---- 工具调用在 CDATA 包含代码围栏 ----

func TestSieve_ToolCallWithCDATAContainingFence(t *testing.T) {
	var state State
	payload := "```python\nprint('hello')\n```"
	chunks := []string{
		"<tool_calls>\n",
		`<invoke name="write_file">` + "\n",
		`<parameter name="path">test.md</parameter>` + "\n",
		`<parameter name="content"><![CDATA[` + payload + `]]></parameter>` + "\n",
		"</invoke>\n",
		"</tool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"write_file"})...)
	}
	events = append(events, Flush(&state, []string{"write_file"})...)

	var text strings.Builder
	tc := 0
	var gotContent any
	for _, e := range events {
		text.WriteString(e.Content)
		if len(e.ToolCalls) > 0 {
			tc += len(e.ToolCalls)
			gotContent = e.ToolCalls[0].Input["content"]
		}
	}
	if tc != 1 {
		t.Fatalf("应有一个工具调用，got %d", tc)
	}
	content, _ := gotContent.(string)
	if content != payload {
		t.Fatalf("CDATA 内围栏内容应完整保留，got %q want %q", content, payload)
	}
	if text.Len() != 0 {
		t.Fatalf("不应有文本泄漏, got %q", text.String())
	}
}

// ---- 极端 token 拆分 ----

// 工具标签被拆成单字符流式到达
func TestSieve_CharByCharToolCall(t *testing.T) {
	var state State
	full := `<tool_calls><invoke name="read_file"><parameter name="path">go.mod</parameter></invoke></tool_calls>`
	var events []Event
	for _, ch := range full {
		events = append(events, ProcessChunk(&state, string(ch), []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var text strings.Builder
	tc := 0
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 1 {
		t.Fatalf("单字符流式应解析出工具调用，got %d", tc)
	}
	if strings.Contains(text.String(), "invoke") {
		t.Fatalf("标签泄漏, got %q", text.String())
	}
}

// ---- 混合格式变体 ----

// 全宽竖线 wrapper + DSML invoke
func TestSieve_FullwidthPipeWrapperDSMLInvoke(t *testing.T) {
	var state State
	chunks := []string{
		"<|tool_calls>\n",
		"<|DSML|invoke name=\"read_file\">\n",
		"<|DSML|parameter name=\"path\">README.md</|DSML|parameter>\n",
		"</|DSML|invoke>\n",
		"</|tool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var text strings.Builder
	tc := 0
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	if tc != 1 {
		t.Fatalf("全宽+DSML混合应解析成功，got %d", tc)
	}
	if strings.Contains(strings.ToLower(text.String()), "dsml") {
		t.Fatalf("DSML 标签泄漏, got %q", text.String())
	}
}

// ---- 未闭合工具块应回退为文本 ----

func TestSieve_UnclosedToolCallBlockFallsBack(t *testing.T) {
	var state State
	chunks := []string{
		"<tool_calls>\n",
		`<invoke name="read_file">` + "\n",
		`<parameter name="path">README.md</parameter>` + "\n",
		// 缺少 </invoke> 和 </tool_calls>
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"read_file"})...)
	}
	events = append(events, Flush(&state, []string{"read_file"})...)

	var text strings.Builder
	tc := 0
	for _, e := range events {
		text.WriteString(e.Content)
		tc += len(e.ToolCalls)
	}
	// 未闭合的应回退为文本，不应丢失
	if text.String() == "" {
		t.Fatalf("未闭合工具块不应丢失所有内容")
	}
	if tc != 0 {
		t.Fatalf("未闭合工具块不应解析出工具调用，got %d", tc)
	}
}

// ---- 文本中 mention 标签变体名 + 真正的工具调用 ----

// 模型输出 commit message 文本中包含 <dsml|tool_calls> 等 mention，
// 紧随其后是真正的 DSML 工具调用。mention 的变体和实际工具调用变体不同。
func TestSieve_TagMentionInTextThenRealToolCall(t *testing.T) {
	var state State
	chunks := []string{
		"建议的 commit message：\n\nfeat: expand DSML alias support\n\n",
		"Add support for <dsml|tool_calls>, ",
		"<|tool_calls> (pipe alias),\n",
		"and <|tool_calls> wrapper variants.\n\n",
		"<|DSML|tool_calls>\n",
		"<|DSML|invoke name=\"Bash\">\n",
		"<|DSML|parameter name=\"command\"><![CDATA[git status]]></|DSML|parameter>\n",
		"</|DSML|invoke>\n",
		"</|DSML|tool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"Bash"})...)
	}
	events = append(events, Flush(&state, []string{"Bash"})...)

	var text strings.Builder
	tc := 0
	var names []string
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			tc++
			names = append(names, call.Name)
		}
	}

	if tc != 1 {
		t.Fatalf("应解析出 1 个工具调用，got %d, text=%q", tc, text.String())
	}
	if names[0] != "Bash" {
		t.Fatalf("应解析出 Bash，got %v", names)
	}
	if !strings.Contains(text.String(), "commit message") {
		t.Fatalf("前置文本应保留, got %q", text.String())
	}
}

func TestSieve_SameVariantTagMentionInTextThenRealToolCall(t *testing.T) {
	var state State
	chunks := []string{
		"Summary: support canonical <tool_calls> and DSML <|DSML|tool_calls> wrappers.\n\n",
		"<|DSML|tool_calls>\n",
		"<|DSML|invoke name=\"Bash\">\n",
		"<|DSML|parameter name=\"command\"><![CDATA[git status]]></|DSML|parameter>\n",
		"</|DSML|invoke>\n",
		"</|DSML|tool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"Bash"})...)
	}
	events = append(events, Flush(&state, []string{"Bash"})...)

	var text strings.Builder
	var callName string
	var command string
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			callCount++
			callName = call.Name
			command, _ = call.Input["command"].(string)
		}
	}

	if callCount != 1 {
		t.Fatalf("应解析出 1 个工具调用，got %d, text=%q", callCount, text.String())
	}
	if callName != "Bash" {
		t.Fatalf("应解析出 Bash，got %q", callName)
	}
	if command != "git status" {
		t.Fatalf("应解析出 command，got %q", command)
	}
	if !strings.Contains(text.String(), "Summary:") {
		t.Fatalf("前置文本应保留, got %q", text.String())
	}
}

func TestSieve_ReviewSampleWithAliasMentionsPreservesBodyAndToolCalls(t *testing.T) {
	var state State
	chunks := []string{
		"Done reviewing the diff. Here's my analysis before we commit:\n\n",
		"Summary of Changes\n",
		"DSML wrapper variant support — recognize aliases (<dsml|tool_calls>, <|tool_calls>) alongside canonical <tool_calls> and <|DSML|tool_calls> wrappers.\n\n",
		"<|DSML|tool_calls>\n",
		"<|DSML|invoke name=\"Bash\">\n",
		"<|DSML|parameter name=\"command\"><![CDATA[git add docs/toolcall-semantics.md internal/toolstream/tool_sieve_xml.go]]></|DSML|parameter>\n",
		"<|DSML|parameter name=\"description\"><![CDATA[Stage all relevant changed files]]></|DSML|parameter>\n",
		"</|DSML|invoke>\n",
		"<|DSML|invoke name=\"Bash\">\n",
		"<|DSML|parameter name=\"command\"><![CDATA[git commit -m \"$(cat <<'EOF'\nfeat(toolstream): expand DSML wrapper detection\n\nSupport DSML wrapper aliases: <dsml|tool_calls> and <|tool_calls> alongside existing canonical wrappers.\nEOF\n)\"]]></|DSML|parameter>\n",
		"<|DSML|parameter name=\"description\"><![CDATA[Create commit with all staged changes]]></|DSML|parameter>\n",
		"</|DSML|invoke>\n",
		"</|DSML|tool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"Bash"})...)
	}
	events = append(events, Flush(&state, []string{"Bash"})...)

	var text strings.Builder
	var commands []string
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			if call.Name == "Bash" {
				cmd, _ := call.Input["command"].(string)
				commands = append(commands, cmd)
			}
		}
	}

	if len(commands) != 2 {
		t.Fatalf("应解析出 2 个 Bash 工具调用，got %d, text=%q", len(commands), text.String())
	}
	if !strings.Contains(text.String(), "<|DSML|tool_calls> wrappers") {
		t.Fatalf("正文中的 DSML mention 应保留, got %q", text.String())
	}
	if !strings.Contains(text.String(), "Summary of Changes") {
		t.Fatalf("前置正文应完整保留, got %q", text.String())
	}
	if strings.Contains(text.String(), "git add docs/toolcall-semantics.md") {
		t.Fatalf("真实工具参数不应泄漏到正文, got %q", text.String())
	}
	if !strings.Contains(commands[0], "git add") || !strings.Contains(commands[1], "git commit") {
		t.Fatalf("工具参数解析不符合预期, got %#v", commands)
	}
}

func TestSieve_ChineseReviewSamplePreservesInlineDSMLMention(t *testing.T) {
	var state State
	chunks := []string{
		"# Context from my IDE setup:\n\n## My request for Codex:\n",
		"基于我的审查，这是工作区更改的总结和提交。\n\n## 审查报告\n\n### 文档\n\nAPI.md 中的工具调用部分缺少针对新 DSML 别名的更新——它只提到了 `",
		"<|DSML|tool_calls>` 和 canonical `<tool_calls>`。由于这涉及 API 兼容性和文档准确性，需要在下游进行记录。\n\n",
		"### 代码\n\n所有更改现在一致地处理四个 DSML wrapper 变体。\n\n现在提交已暂存的更改。\n\n",
		"<|DSML|tool_calls>\n",
		"  <|DSML|invoke name=\"Bash\">\n",
		"    <|DSML|parameter name=\"command\"><![CDATA[git commit -m \"$(cat <<'EOF'\nfeat: expand DSML tool-call alias and fence handling\nEOF\n)\"]]></|DSML|parameter>\n",
		"    <|DSML|parameter name=\"description\"><![CDATA[Commit staged changes]]></|DSML|parameter>\n",
		"  </|DSML|invoke>\n",
		"</|DSML|tool_calls>\n\n补充",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"Bash"})...)
	}
	events = append(events, Flush(&state, []string{"Bash"})...)

	var text strings.Builder
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		callCount += len(e.ToolCalls)
	}

	if callCount != 1 {
		t.Fatalf("应解析出 1 个工具调用，got %d, text=%q", callCount, text.String())
	}
	want := "它只提到了 `<|DSML|tool_calls>` 和 canonical `<tool_calls>`。由于这涉及 API 兼容性"
	if !strings.Contains(text.String(), want) {
		t.Fatalf("正文不应在 inline DSML mention 处截断, want contains %q, got %q", want, text.String())
	}
	if !strings.Contains(text.String(), "补充") {
		t.Fatalf("工具块后的正文应保留, got %q", text.String())
	}
	if strings.Contains(text.String(), "<|DSML|invoke") {
		t.Fatalf("真实工具块不应泄漏到正文, got %q", text.String())
	}
}

func TestSieve_HyphenatedDSMLShellWithHereDocCDATA(t *testing.T) {
	var state State
	chunks := []string{
		"<dsml-tool-calls>\n",
		"<dsml-invoke name=\"Bash\">\n",
		"<dsml-parameter name=\"command\"><![CDATA[git commit -m \"$(cat <<'EOF'\n",
		"docs: add missing directory entries and package descriptions to architecture docs\n",
		"Fill gaps identified in architecture audit: add artifacts/ and static/ to\n",
		"directory tree, and document 7 auxiliary internal/ packages (textclean,\n",
		"claudeconv, compat, rawsample, devcapture, util, version) in Section 3.\n\n",
		"Co-Authored-By: Claude Opus 4.7 noreply@anthropic.com\n",
		"EOF\n",
		")\"]]></dsml-parameter>\n",
		"<dsml-parameter name=\"description\"><![CDATA[Create commit with architecture doc updates]]></dsml-parameter>\n",
		"</dsml-invoke>\n",
		"</dsml-tool-calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"Bash"})...)
	}
	events = append(events, Flush(&state, []string{"Bash"})...)

	var text strings.Builder
	var command string
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			callCount++
			command, _ = call.Input["command"].(string)
		}
	}

	if callCount != 1 {
		t.Fatalf("应解析出 1 个 hyphenated DSML 工具调用，got %d, text=%q", callCount, text.String())
	}
	if !strings.Contains(command, `git commit -m "$(cat <<'EOF'`) || !strings.Contains(command, "Co-Authored-By: Claude Opus 4.7") {
		t.Fatalf("here-doc command 未完整保留, got %q", command)
	}
	if strings.Contains(text.String(), "dsml-tool-calls") || strings.Contains(text.String(), "git commit -m") {
		t.Fatalf("真实工具块不应泄漏到正文, got %q", text.String())
	}
}

func TestSieve_ToleratesDSMLSpaceSeparatorTypo(t *testing.T) {
	var state State
	chunks := []string{
		"准备读取文件。\n",
		"<|DSML tool_calls>\n",
		"<|DSML invoke name=\"Read\">\n",
		"<|DSML parameter name=\"file_path\"><![CDATA[/tmp/input.txt]]></|DSML parameter>\n",
		"</|DSML invoke>\n",
		"</|DSML tool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"Read"})...)
	}
	events = append(events, Flush(&state, []string{"Read"})...)

	var text strings.Builder
	var filePath string
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			callCount++
			filePath, _ = call.Input["file_path"].(string)
		}
	}

	if callCount != 1 {
		t.Fatalf("应解析出 1 个工具调用，got %d, text=%q", callCount, text.String())
	}
	if filePath != "/tmp/input.txt" {
		t.Fatalf("应解析出 file_path，got %q", filePath)
	}
	if !strings.Contains(text.String(), "准备读取文件") {
		t.Fatalf("前置正文应保留, got %q", text.String())
	}
	if strings.Contains(text.String(), "<|DSML invoke") {
		t.Fatalf("真实工具块不应泄漏到正文, got %q", text.String())
	}
}

func TestSieve_DSMLSpaceLookalikeTagNameStaysText(t *testing.T) {
	var state State
	input := "<|DSML tool_calls_extra><|DSML invoke name=\"Read\"><|DSML parameter name=\"file_path\">/tmp/input.txt</|DSML parameter></|DSML invoke></|DSML tool_calls_extra>"
	events := ProcessChunk(&state, input, []string{"Read"})
	events = append(events, Flush(&state, []string{"Read"})...)

	var text strings.Builder
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		callCount += len(e.ToolCalls)
	}
	if callCount != 0 {
		t.Fatalf("相似标签名不应触发工具调用，got %d", callCount)
	}
	if text.String() != input {
		t.Fatalf("相似标签名应作为正文透传, got %q", text.String())
	}
}

func TestSieve_DSMLCollapsedTagNamesWithPrefixText(t *testing.T) {
	var state State
	todos := `[x] 检查 toolcalls_format.go 格式化逻辑
[x] 检查 toolcalls_parse.go 解析逻辑
[x] 检查 toolcalls_xml.go 和 toolcalls_dsml.go
[x] 检查 toolcalls_markup.go 和 toolcalls_json_repair.go
[x] 检查 prompt/tool_calls.go 注入逻辑
[x] 检查 toolstream 流式解析
[x] 查看测试文件确认预期行为
[x] 给出调查结论`
	chunks := []string{
		"[]\n",
		"<DSMLtool_calls>\n",
		"<DSMLinvoke name=\"update_todo_list\">\n",
		"<DSMLparameter name=\"todos\"><![CDATA[" + todos + "]]></DSMLparameter>\n",
		"</DSMLinvoke>\n",
		"</DSMLtool_calls>",
	}
	var events []Event
	for _, c := range chunks {
		events = append(events, ProcessChunk(&state, c, []string{"update_todo_list"})...)
	}
	events = append(events, Flush(&state, []string{"update_todo_list"})...)

	var text strings.Builder
	var gotTodos string
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		for _, call := range e.ToolCalls {
			callCount++
			gotTodos, _ = call.Input["todos"].(string)
		}
	}
	if callCount != 1 {
		t.Fatalf("应解析出 1 个工具调用，got %d, text=%q", callCount, text.String())
	}
	if gotTodos != todos {
		t.Fatalf("todos 应完整保留，got %q", gotTodos)
	}
	if text.String() != "[]\n" {
		t.Fatalf("前置正文应完整保留且不泄漏工具块, got %q", text.String())
	}
}

func TestSieve_DSMLCollapsedLookalikeTagNameStaysText(t *testing.T) {
	var state State
	input := "<DSMLtool_calls_extra><DSMLinvoke name=\"update_todo_list\"><DSMLparameter name=\"todos\">x</DSMLparameter></DSMLinvoke></DSMLtool_calls_extra>"
	events := ProcessChunk(&state, input, []string{"update_todo_list"})
	events = append(events, Flush(&state, []string{"update_todo_list"})...)

	var text strings.Builder
	callCount := 0
	for _, e := range events {
		text.WriteString(e.Content)
		callCount += len(e.ToolCalls)
	}
	if callCount != 0 {
		t.Fatalf("相似 collapsed 标签名不应触发工具调用，got %d", callCount)
	}
	if text.String() != input {
		t.Fatalf("相似 collapsed 标签名应作为正文透传, got %q", text.String())
	}
}
