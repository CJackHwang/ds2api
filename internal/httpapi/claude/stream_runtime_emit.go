package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"ds2api/internal/util"
)

func (s *claudeStreamRuntime) send(event string, v any) {
	b, _ := json.Marshal(v)
	_, _ = s.w.Write([]byte("event: "))
	_, _ = s.w.Write([]byte(event))
	_, _ = s.w.Write([]byte("\n"))
	_, _ = s.w.Write([]byte("data: "))
	_, _ = s.w.Write(b)
	_, _ = s.w.Write([]byte("\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *claudeStreamRuntime) sendError(message string) {
	s.sendErrorWithCode(500, message, "internal_error")
}

func (s *claudeStreamRuntime) sendErrorWithCode(status int, message, code string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "upstream stream error"
	}
	if code == "" {
		code = "internal_error"
	}
	errType := "api_error"
	if status == 429 {
		errType = "rate_limit_error"
	}
	s.send("error", map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": msg,
			"code":    code,
			"param":   nil,
		},
	})
}

func (s *claudeStreamRuntime) sendPing() {
	s.send("ping", map[string]any{"type": "ping"})
}

func (s *claudeStreamRuntime) sendMessageStart() {
	inputTokens := countClaudeInputTokensFromText(s.promptTokenText, s.model)
	if inputTokens == 0 {
		inputTokens = util.CountPromptTokens(fmt.Sprintf("%v", s.messages), s.model)
	}
	s.send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
}
