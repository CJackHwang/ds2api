package sse

import "testing"

func TestParseDeepSeekContentLineDone(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte("data: [DONE]"), false, "text")
	if !res.Parsed || !res.Stop {
		t.Fatalf("expected parsed stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineError(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"error":"boom"}`), false, "text")
	if !res.Parsed || !res.Stop {
		t.Fatalf("expected stop on error: %#v", res)
	}
	if res.ErrorMessage == "" {
		t.Fatalf("expected non-empty error message")
	}
}

func TestParseDeepSeekContentLineContentFilter(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"code":"content_filter"}`), false, "text")
	if !res.Parsed || !res.Stop || !res.ContentFilter {
		t.Fatalf("expected content-filter stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineContentFilterCodeIncludesOutputTokens(t *testing.T) {
	res := ParseDeepSeekContentLine(
		[]byte(`data: {"code":"content_filter","accumulated_token_usage":99}`),
		false, "text",
	)
	if !res.Parsed || !res.Stop || !res.ContentFilter {
		t.Fatalf("expected content-filter stop result: %#v", res)
	}
	if res.OutputTokens != 99 {
		t.Fatalf("expected output token usage 99, got %d", res.OutputTokens)
	}
}

func TestParseDeepSeekContentLineContentFilterStatus(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/status","v":"CONTENT_FILTER"}`), false, "text")
	if !res.Parsed || !res.Stop || !res.ContentFilter {
		t.Fatalf("expected status-based content-filter stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineCapturesAccumulatedTokenUsage(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":1383},{"p":"quasi_status","v":"FINISHED"}]}`), false, "text")
	if res.OutputTokens != 1383 {
		t.Fatalf("expected output token usage 1383, got %d", res.OutputTokens)
	}
}

func TestParseDeepSeekContentLineContent(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"hi"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "hi" || res.Parts[0].Type != "text" {
		t.Fatalf("unexpected parts: %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLineStripsLeakedContentFilterSuffix(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"正常输出CONTENT_FILTER你好，这个问题我暂时无法回答"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "正常输出" {
		t.Fatalf("unexpected parts after filter: %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLineDropsPureLeakedContentFilterChunk(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"CONTENT_FILTER你好，这个问题我暂时无法回答"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 0 {
		t.Fatalf("expected empty parts, got %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLineTrimsFromContentFilterKeyword(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"模型会在命中 CONTENT_FILTER 时返回拒绝原因。"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "模型会在命中" {
		t.Fatalf("unexpected parts after filter: %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLineContentTextEqualContentFilterDoesNotStop(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"content_filter"}`), false, "text")
	if !res.Parsed {
		t.Fatalf("expected parsed result: %#v", res)
	}
	if res.Stop || res.ContentFilter {
		t.Fatalf("did not expect content-filter stop for content text: %#v", res)
	}
}

func TestParseDeepSeekContentLinePreservesTrailingNewlineBeforeLeakedContentFilter(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte("data: {\"p\":\"response/content\",\"v\":\"line1\\nCONTENT_FILTERblocked\"}"), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "line1\n" {
		t.Fatalf("expected trailing newline preserved, got %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLineKeepsNewlineOnlyChunkBeforeLeakedContentFilter(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte("data: {\"p\":\"response/content\",\"v\":\"\\nCONTENT_FILTERblocked\"}"), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "\n" {
		t.Fatalf("expected newline-only chunk preserved, got %#v", res.Parts)
	}
}
