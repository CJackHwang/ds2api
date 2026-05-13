package completionruntime

import (
	"context"
	"io"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
)

type StreamRetryOptions struct {
	Surface          string
	Stream           bool
	RetryEnabled     bool
	RetryMaxAttempts int
	MaxAttempts      int
	UsagePrompt      string
	Request          promptcompat.StandardRequest
	CurrentInputFile history.CurrentInputConfigReader
}

type StreamRetryHooks struct {
	ConsumeAttempt  func(resp *http.Response, allowDeferEmpty bool) (terminalWritten bool, retryable bool)
	Finalize        func(attempts int)
	ParentMessageID func() int
	OnRetry         func(attempts int)
	OnRetryPrompt   func(prompt string)
	OnRetryFailure  func(status int, message, code string)
	OnAccountSwitch func(sessionID string)
	OnTerminal      func(attempts int)
}

func ExecuteStreamWithRetry(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, initialResp *http.Response, payload map[string]any, pow string, opts StreamRetryOptions, hooks StreamRetryHooks) {
	if hooks.ConsumeAttempt == nil {
		return
	}
	surface := strings.TrimSpace(opts.Surface)
	if surface == "" {
		surface = "completion"
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	retryMax := opts.RetryMaxAttempts
	if retryMax <= 0 {
		retryMax = shared.EmptyOutputRetryMaxAttempts()
	}

	attempts := 0
	accountSwitchAttempted := false
	currentResp := initialResp
	currentPayload := clonePayload(payload)
	for {
		allowAccountSwitch := opts.RetryEnabled && attempts >= retryMax && !accountSwitchAttempted && a != nil && a.UseConfigToken
		terminalWritten, retryable := hooks.ConsumeAttempt(currentResp, opts.RetryEnabled && (attempts < retryMax || allowAccountSwitch))
		if terminalWritten {
			if hooks.OnTerminal != nil {
				hooks.OnTerminal(attempts)
			}
			return
		}
		if !retryable || !opts.RetryEnabled {
			if hooks.Finalize != nil {
				hooks.Finalize(attempts)
			}
			return
		}

		if attempts >= retryMax {
			if canRetryOnAlternateAccount(ctx, a, &assistantturn.OutputError{Status: http.StatusTooManyRequests}, opts.RetryEnabled, &accountSwitchAttempted) {
				switched, switchErr := startPayloadCompletionOnAlternateAccount(ctx, ds, a, payload, opts, maxAttempts)
				if switchErr != nil {
					if hooks.OnRetryFailure != nil {
						hooks.OnRetryFailure(switchErr.Status, switchErr.Message, switchErr.Code)
					}
					return
				}
				if switched.Response != nil {
					config.Logger.Info("[completion_runtime_account_switch_retry] retrying after 429", "surface", surface, "stream", opts.Stream, "account", a.AccountID)
					currentResp = switched.Response
					currentPayload = switched.Payload
					pow = switched.Pow
					if hooks.OnAccountSwitch != nil {
						hooks.OnAccountSwitch(switched.SessionID)
					}
					if hooks.OnRetryPrompt != nil {
						hooks.OnRetryPrompt(opts.UsagePrompt)
					}
					continue
				}
			}
			if hooks.Finalize != nil {
				hooks.Finalize(attempts)
			}
			return
		}

		attempts++
		parentMessageID := 0
		if hooks.ParentMessageID != nil {
			parentMessageID = hooks.ParentMessageID()
		}
		config.Logger.Info("[completion_runtime_empty_retry] attempting synthetic retry", "surface", surface, "stream", opts.Stream, "retry_attempt", attempts, "parent_message_id", parentMessageID)
		retryPow, powErr := ds.GetPow(ctx, a, maxAttempts)
		if powErr != nil {
			config.Logger.Warn("[completion_runtime_empty_retry] retry PoW fetch failed, falling back to original PoW", "surface", surface, "stream", opts.Stream, "retry_attempt", attempts, "error", powErr)
			retryPow = pow
		}
		nextResp, err := ds.CallCompletion(ctx, a, shared.ClonePayloadForEmptyOutputRetry(currentPayload, parentMessageID), retryPow, maxAttempts)
		if err != nil {
			if hooks.OnRetryFailure != nil {
				hooks.OnRetryFailure(http.StatusInternalServerError, "Failed to get completion.", "error")
			}
			config.Logger.Warn("[completion_runtime_empty_retry] retry request failed", "surface", surface, "stream", opts.Stream, "retry_attempt", attempts, "error", err)
			return
		}
		if nextResp.StatusCode != http.StatusOK {
			body, readErr := io.ReadAll(nextResp.Body)
			if readErr != nil {
				config.Logger.Warn("[completion_runtime_empty_retry] retry error body read failed", "surface", surface, "stream", opts.Stream, "retry_attempt", attempts, "error", readErr)
			}
			closeRetryBody(surface, nextResp.Body)
			msg := strings.TrimSpace(string(body))
			if msg == "" {
				msg = http.StatusText(nextResp.StatusCode)
			}
			if hooks.OnRetryFailure != nil {
				hooks.OnRetryFailure(nextResp.StatusCode, msg, "error")
			}
			return
		}
		if hooks.OnRetry != nil {
			hooks.OnRetry(attempts)
		}
		if hooks.OnRetryPrompt != nil {
			hooks.OnRetryPrompt(shared.UsagePromptWithEmptyOutputRetry(opts.UsagePrompt, attempts))
		}
		currentResp = nextResp
	}
}

func startPayloadCompletionOnAlternateAccount(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, payload map[string]any, opts StreamRetryOptions, maxAttempts int) (StartResult, *assistantturn.OutputError) {
	sessionID, err := ds.CreateSession(ctx, a, maxAttempts)
	if err != nil {
		return StartResult{}, authOutputError(a)
	}
	pow, err := ds.GetPow(ctx, a, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID}, &assistantturn.OutputError{Status: http.StatusUnauthorized, Message: "Failed to get PoW (invalid token or unknown error).", Code: "error"}
	}
	nextPayload := clonePayload(payload)
	if opts.CurrentInputFile != nil && opts.Request.CurrentInputFileApplied {
		stdReq, prepErr := reuploadCurrentInputFileForAccount(ctx, ds, a, opts.Request, Options{CurrentInputFile: opts.CurrentInputFile})
		if prepErr != nil {
			return StartResult{SessionID: sessionID}, prepErr
		}
		nextPayload = stdReq.CompletionPayload(sessionID)
	}
	nextPayload["chat_session_id"] = sessionID
	delete(nextPayload, "parent_message_id")
	resp, err := ds.CallCompletion(ctx, a, nextPayload, pow, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID, Payload: nextPayload, Pow: pow}, &assistantturn.OutputError{Status: http.StatusInternalServerError, Message: "Failed to get completion.", Code: "error"}
	}
	return StartResult{SessionID: sessionID, Payload: nextPayload, Pow: pow, Response: resp}, nil
}

func clonePayload(payload map[string]any) map[string]any {
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	return clone
}

func closeRetryBody(surface string, body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		config.Logger.Warn("[completion_runtime_empty_retry] retry response body close failed", "surface", surface, "error", err)
	}
}
