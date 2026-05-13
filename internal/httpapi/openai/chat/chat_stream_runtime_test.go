package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/promptcompat"
)

func TestChatStreamKeepAliveUsesCommentOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newChatStreamRuntime(
		rec,
		http.NewResponseController(rec),
		true,
		"chatcmpl-test",
		time.Now().Unix(),
		"deepseek-v4-flash",
		"prompt",
		false,
		false,
		true,
		nil,
		nil,
		promptcompat.DefaultToolChoicePolicy(),
		false,
		false,
	)

	runtime.sendKeepAlive()

	body := rec.Body.String()
	if !strings.Contains(body, ": keep-alive\n\n") {
		t.Fatalf("expected keep-alive comment, got %q", body)
	}
	frames, done := parseSSEDataFrames(t, body)
	if done {
		t.Fatalf("keep-alive must not emit [DONE], body=%q", body)
	}
	if len(frames) != 0 {
		t.Fatalf("keep-alive must not emit JSON data frames, got %#v body=%q", frames, body)
	}
}

func TestChatStreamFinalizeEnforcesRequiredToolChoice(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newChatStreamRuntime(
		rec,
		http.NewResponseController(rec),
		true,
		"chatcmpl-test",
		time.Now().Unix(),
		"deepseek-v4-flash",
		"prompt",
		false,
		false,
		true,
		[]string{"Write"},
		nil,
		promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired},
		true,
		false,
	)

	if !runtime.finalize("stop", false) {
		t.Fatalf("expected terminal error to be written")
	}
	if runtime.finalErrorCode != "tool_choice_violation" {
		t.Fatalf("expected tool_choice_violation, got %q body=%s", runtime.finalErrorCode, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tool_choice requires") {
		t.Fatalf("expected tool choice error in stream body, got %s", rec.Body.String())
	}
}
