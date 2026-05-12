package toolcall

import (
	"strings"
	"testing"
)

// Benchmark inputs.
var (
	benchDSML = `<|DSML|tool_calls>
<|DSML|invoke name="Read">
<|DSML|parameter name="file_path"><![CDATA[/Users/activer/developer/ds2api/internal/toolcall/toolcalls_parse.go]]></|DSML|parameter>
<|DSML|parameter name="limit"><![CDATA[60]]></|DSML|parameter>
<|DSML|parameter name="offset"><![CDATA[0]]></|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>`

	benchXML = `<tool_calls>
<invoke name="Bash">
<parameter name="command">git log --oneline -n 10</parameter>
<parameter name="description">Show recent commits</parameter>
</invoke>
</tool_calls>`

	benchMultiCall = `<|DSML|tool_calls>
<|DSML|invoke name="Read">
<|DSML|parameter name="file_path"><![CDATA[/tmp/a.go]]></|DSML|parameter>
</|DSML|invoke>
<|DSML|invoke name="Read">
<|DSML|parameter name="file_path"><![CDATA[/tmp/b.go]]></|DSML|parameter>
</|DSML|invoke>
<|DSML|invoke name="Write">
<|DSML|parameter name="file_path"><![CDATA[/tmp/out.go]]></|DSML|parameter>
<|DSML|parameter name="content"><![CDATA[package main]]></|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>`

	benchLargeBody = func() string {
		bigContent := strings.Repeat("line of code here\n", 500) // ~9 KB body
		return `<tool_calls><invoke name="Write"><parameter name="file_path">big.go</parameter><parameter name="content"><![CDATA[` +
			bigContent + `]]></parameter></invoke></tool_calls>`
	}()

	benchNoToolCall = `This is a regular assistant response with no tool calls. ` +
		strings.Repeat("The assistant discusses the topic at length. ", 20)

	benchMixed = `Some text before.
<|DSML|tool_calls>
<|DSML|invoke name="Bash">
<|DSML|parameter name="command"><![CDATA[npm run build --prefix webui]]></|DSML|parameter>
<|DSML|parameter name="description"><![CDATA[Build the webui]]></|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>
Some text after.`
)

var benchNames = []string{"Bash", "Read", "Write"}

func BenchmarkParseToolCallsDSML(b *testing.B) {
	for b.Loop() {
		ParseToolCalls(benchDSML, benchNames)
	}
}

func BenchmarkParseToolCallsXML(b *testing.B) {
	for b.Loop() {
		ParseToolCalls(benchXML, benchNames)
	}
}

func BenchmarkParseToolCallsMultiCall(b *testing.B) {
	for b.Loop() {
		ParseToolCalls(benchMultiCall, benchNames)
	}
}

func BenchmarkParseToolCallsLargeBody(b *testing.B) {
	for b.Loop() {
		ParseToolCalls(benchLargeBody, benchNames)
	}
}

func BenchmarkParseToolCallsNoToolCall(b *testing.B) {
	for b.Loop() {
		ParseToolCalls(benchNoToolCall, benchNames)
	}
}

func BenchmarkParseToolCallsMixed(b *testing.B) {
	for b.Loop() {
		ParseToolCalls(benchMixed, benchNames)
	}
}

func BenchmarkParseToolCallsDetailed(b *testing.B) {
	for b.Loop() {
		ParseToolCallsDetailed(benchDSML, benchNames)
	}
}
