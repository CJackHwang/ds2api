package toolcall

import "testing"

// ---------------------------------------------------------------------------
// namesHitWhitelist
// ---------------------------------------------------------------------------

func TestNamesHitWhitelist_EmptyNames(t *testing.T) {
	calls := []ParsedToolCall{{Name: "read_file"}}
	if namesHitWhitelist(calls, nil) {
		t.Error("expected false when availableNames is nil")
	}
	if namesHitWhitelist(calls, []string{}) {
		t.Error("expected false when availableNames is empty")
	}
}

func TestNamesHitWhitelist_EmptyCalls(t *testing.T) {
	if namesHitWhitelist(nil, []string{"read_file"}) {
		t.Error("expected false when calls is nil")
	}
}

func TestNamesHitWhitelist_Hit(t *testing.T) {
	calls := []ParsedToolCall{{Name: "read_file"}, {Name: "write_file"}}
	if !namesHitWhitelist(calls, []string{"read_file", "list_dir"}) {
		t.Error("expected true: read_file is in whitelist")
	}
}

func TestNamesHitWhitelist_Miss(t *testing.T) {
	calls := []ParsedToolCall{{Name: "unknown_tool"}}
	if namesHitWhitelist(calls, []string{"read_file", "write_file"}) {
		t.Error("expected false: unknown_tool not in whitelist")
	}
}

// ---------------------------------------------------------------------------
// buildParseCandidate – parsePath
// ---------------------------------------------------------------------------

func TestBuildParseCandidatePath_Empty(t *testing.T) {
	cand := buildParseCandidate("", nil)
	if cand.parsePath != parsePathEmpty {
		t.Errorf("want %q, got %q", parsePathEmpty, cand.parsePath)
	}
	if cand.sawToolCallSyntax {
		t.Error("sawToolCallSyntax should be false for empty input")
	}
}

func TestBuildParseCandidatePath_NoSyntax(t *testing.T) {
	cand := buildParseCandidate("hello world, no tool calls here", nil)
	if cand.parsePath != parsePathXMLFailed {
		t.Errorf("want %q, got %q", parsePathXMLFailed, cand.parsePath)
	}
	if cand.sawToolCallSyntax {
		t.Error("sawToolCallSyntax should be false for plain text")
	}
}

func TestBuildParseCandidatePath_XMLDirect(t *testing.T) {
	text := `<tool_calls><invoke name="read_file"><parameter name="path">/tmp/x</parameter></invoke></tool_calls>`
	cand := buildParseCandidate(text, nil)
	if cand.parsePath != parsePathXMLDirect {
		t.Errorf("want %q, got %q", parsePathXMLDirect, cand.parsePath)
	}
	if !cand.sawToolCallSyntax {
		t.Error("sawToolCallSyntax should be true")
	}
	if len(cand.calls) == 0 {
		t.Error("expected at least one call")
	}
}

func TestBuildParseCandidatePath_StrippedEmpty(t *testing.T) {
	text := "```xml\n<tool_calls><invoke name=\"read_file\"><parameter name=\"path\">/tmp</parameter></invoke></tool_calls>\n```"
	cand := buildParseCandidate(text, nil)
	if cand.parsePath != parsePathStrippedEmpty {
		t.Errorf("want %q, got %q", parsePathStrippedEmpty, cand.parsePath)
	}
}

// ---------------------------------------------------------------------------
// buildParseCandidate – ambiguous
// ---------------------------------------------------------------------------

func TestBuildParseCandidateAmbiguous_False(t *testing.T) {
	text := `<tool_calls><invoke name="fn"><parameter name="x">1</parameter></invoke></tool_calls>`
	cand := buildParseCandidate(text, nil)
	if cand.ambiguous {
		t.Error("single-syntax input should not be ambiguous")
	}
}

// ---------------------------------------------------------------------------
// buildParseCandidate – nameWhitelistHit
// ---------------------------------------------------------------------------

func TestBuildParseCandidateWhitelistHit(t *testing.T) {
	text := `<tool_calls><invoke name="read_file"><parameter name="path">/tmp/x</parameter></invoke></tool_calls>`
	cand := buildParseCandidate(text, []string{"read_file", "write_file"})
	if cand.parsePath != parsePathXMLDirect {
		t.Errorf("want xml_direct, got %q", cand.parsePath)
	}
	if !cand.nameWhitelistHit {
		t.Error("expected nameWhitelistHit=true for read_file in whitelist")
	}
}

func TestBuildParseCandidateWhitelistMiss(t *testing.T) {
	text := `<tool_calls><invoke name="read_file"><parameter name="path">/tmp/x</parameter></invoke></tool_calls>`
	cand := buildParseCandidate(text, []string{"other_tool"})
	if cand.nameWhitelistHit {
		t.Error("expected nameWhitelistHit=false when name not in whitelist")
	}
}

func TestBuildParseCandidateWhitelistNilNames(t *testing.T) {
	text := `<tool_calls><invoke name="read_file"><parameter name="path">/tmp</parameter></invoke></tool_calls>`
	cand := buildParseCandidate(text, nil)
	if cand.nameWhitelistHit {
		t.Error("expected nameWhitelistHit=false when availableNames is nil")
	}
}

// ---------------------------------------------------------------------------
// Public API: available names now threaded through
// ---------------------------------------------------------------------------

func TestParseToolCallsDetailedWhitelistHit(t *testing.T) {
	text := `<tool_calls><invoke name="search"><parameter name="q">go</parameter></invoke></tool_calls>`
	result := ParseToolCallsDetailed(text, []string{"search"})
	if len(result.Calls) == 0 {
		t.Fatal("expected at least one call")
	}
	if result.Calls[0].Name != "search" {
		t.Errorf("expected name=search, got %s", result.Calls[0].Name)
	}
}
