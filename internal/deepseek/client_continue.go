package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

const defaultAutoContinueLimit = 8

type continueOpenFunc func(context.Context, string, int) (*http.Response, error)

type continueState struct {
	sessionID         string
	responseMessageID int
	lastStatus        string
	finished          bool
}

// wrapCompletionWithAutoContinue wraps the completion response body so that
// if the upstream indicates the response is incomplete (WIP / INCOMPLETE /
// AUTO_CONTINUE), ds2api will automatically call the DeepSeek continue
// endpoint and splice the continuation SSE stream onto the original.
// The caller sees a single, seamless SSE stream.
func (c *Client) wrapCompletionWithAutoContinue(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, resp *http.Response) *http.Response {
	if resp == nil || resp.Body == nil {
		return resp
	}
	sessionID, _ := payload["chat_session_id"].(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return resp
	}
	config.Logger.Debug("[auto_continue] wrapping completion response", "session_id", sessionID)
	resp.Body = newAutoContinueBody(ctx, resp.Body, sessionID, defaultAutoContinueLimit, func(ctx context.Context, sessionID string, responseMessageID int) (*http.Response, error) {
		return c.callContinue(ctx, a, sessionID, responseMessageID, powResp)
	})
	return resp
}

// callContinue sends a continue request to DeepSeek to resume generation.
func (c *Client) callContinue(ctx context.Context, a *auth.RequestAuth, sessionID string, responseMessageID int, powResp string) (*http.Response, error) {
	if strings.TrimSpace(sessionID) == "" || responseMessageID <= 0 {
		return nil, errors.New("missing continue identifiers")
	}
	clients := c.requestClientsForAuth(ctx, a)
	headers := c.authHeaders(a.DeepSeekToken)
	headers["x-ds-pow-response"] = powResp
	payload := map[string]any{
		"chat_session_id":    sessionID,
		"message_id":         responseMessageID,
		"fallback_to_resume": true,
	}
	config.Logger.Info("[auto_continue] calling continue", "session_id", sessionID, "message_id", responseMessageID)
	captureSession := c.capture.Start("deepseek_continue", DeepSeekContinueURL, a.AccountID, payload)
	resp, err := c.streamPost(ctx, clients.stream, DeepSeekContinueURL, headers, payload)
	if err != nil {
		return nil, err
	}
	if captureSession != nil {
		resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, errors.New("continue failed")
	}
	return resp, nil
}

// newAutoContinueBody returns a new ReadCloser that transparently pumps
// continuation rounds via an io.Pipe.
func newAutoContinueBody(ctx context.Context, initial io.ReadCloser, sessionID string, maxRounds int, openContinue continueOpenFunc) io.ReadCloser {
	if initial == nil || strings.TrimSpace(sessionID) == "" || openContinue == nil {
		return initial
	}
	if maxRounds <= 0 {
		maxRounds = defaultAutoContinueLimit
	}
	pr, pw := io.Pipe()
	go pumpAutoContinue(ctx, pw, initial, continueState{sessionID: sessionID}, maxRounds, openContinue)
	return pr
}

// pumpAutoContinue is the goroutine that drives the auto-continue loop.
// It reads the initial SSE body, checks whether a continue is required,
// and if so opens a new continue stream and splices it onto the pipe writer.
func pumpAutoContinue(ctx context.Context, pw *io.PipeWriter, initial io.ReadCloser, state continueState, maxRounds int, openContinue continueOpenFunc) {
	defer func() { _ = pw.Close() }()
	current := initial
	rounds := 0
	for {
		hadDone, err := streamBodyWithContinueState(ctx, pw, current, &state)
		_ = current.Close()
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if state.shouldContinue() && rounds < maxRounds {
			rounds++
			config.Logger.Info("[auto_continue] continuing", "round", rounds, "session_id", state.sessionID, "message_id", state.responseMessageID, "status", state.lastStatus)
			nextResp, err := openContinue(ctx, state.sessionID, state.responseMessageID)
			if err != nil {
				config.Logger.Warn("[auto_continue] continue request failed", "round", rounds, "error", err)
				_ = pw.CloseWithError(err)
				return
			}
			current = nextResp.Body
			state.prepareForNextRound()
			continue
		}
		// Emit the final [DONE] sentinel if the upstream had one.
		if hadDone {
			if _, err := io.Copy(pw, bytes.NewBufferString("data: [DONE]\n")); err != nil {
				_ = pw.CloseWithError(err)
			}
		}
		return
	}
}

// streamBodyWithContinueState scans an SSE body line-by-line, writing each
// line through to pw while observing state signals. Intermediate [DONE]
// sentinels are consumed (not forwarded) so that the downstream only sees
// one final [DONE] at the very end.
func streamBodyWithContinueState(ctx context.Context, pw *io.PipeWriter, body io.Reader, state *continueState) (bool, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	hadDone := false
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return hadDone, ctx.Err()
		default:
		}
		line := append([]byte{}, scanner.Bytes()...)
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data == "[DONE]" {
				hadDone = true
				continue
			}
			state.observe(data)
		}
		if _, err := io.Copy(pw, bytes.NewReader(append(line, '\n'))); err != nil {
			return hadDone, err
		}
	}
	return hadDone, scanner.Err()
}

// observe extracts continue-relevant signals from an SSE JSON chunk.
func (s *continueState) observe(data string) {
	if s == nil || strings.TrimSpace(data) == "" {
		return
	}
	var chunk map[string]any
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}
	// Top-level response_message_id
	if id := intFrom(chunk["response_message_id"]); id > 0 {
		s.responseMessageID = id
	}
	// Path-based status: {"p": "response/status", "v": "FINISHED"}
	if p, _ := chunk["p"].(string); p == "response/status" {
		if status, _ := chunk["v"].(string); status != "" {
			s.lastStatus = strings.TrimSpace(status)
			if strings.EqualFold(s.lastStatus, "FINISHED") {
				s.finished = true
			}
		}
	}
	// Nested v.response
	v, _ := chunk["v"].(map[string]any)
	if response, _ := v["response"].(map[string]any); response != nil {
		if id := intFrom(response["message_id"]); id > 0 {
			s.responseMessageID = id
		}
		if status, _ := response["status"].(string); status != "" {
			s.lastStatus = strings.TrimSpace(status)
			if strings.EqualFold(s.lastStatus, "FINISHED") {
				s.finished = true
			}
		}
		if autoContinue, ok := response["auto_continue"].(bool); ok && autoContinue {
			s.lastStatus = "AUTO_CONTINUE"
		}
	}
	// Nested message.response
	if message, _ := chunk["message"].(map[string]any); message != nil {
		if response, _ := message["response"].(map[string]any); response != nil {
			if id := intFrom(response["message_id"]); id > 0 {
				s.responseMessageID = id
			}
			if status, _ := response["status"].(string); status != "" {
				s.lastStatus = strings.TrimSpace(status)
				if strings.EqualFold(s.lastStatus, "FINISHED") {
					s.finished = true
				}
			}
		}
	}
}

// shouldContinue returns true when the upstream indicates the response is
// not yet finished and we have enough information to issue a continue request.
func (s *continueState) shouldContinue() bool {
	if s == nil {
		return false
	}
	if s.finished || s.responseMessageID <= 0 || strings.TrimSpace(s.sessionID) == "" {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(s.lastStatus)) {
	case "WIP", "INCOMPLETE", "AUTO_CONTINUE":
		return true
	default:
		return false
	}
}

// prepareForNextRound resets ephemeral state before processing the next
// continuation stream.
func (s *continueState) prepareForNextRound() {
	if s == nil {
		return
	}
	s.finished = false
	s.lastStatus = ""
}
