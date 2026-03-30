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

func TestParseDeepSeekContentLineKeepsLegitContentFilterMentions(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"字符串 CONTENT_FILTER 用于说明上游审核信号"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "字符串 CONTENT_FILTER 用于说明上游审核信号" {
		t.Fatalf("unexpected parts for legit mention: %#v", res.Parts)
	}
}
