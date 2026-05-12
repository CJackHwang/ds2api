// Package provider defines the minimal abstraction interface for upstream LLM
// completion services.
//
// # Responsibility boundary
//
// A Provider is responsible for: HTTP transport, streaming SSE parsing, account
// rotation, retry, and usage reporting. It is NOT responsible for: tool-call
// XML parsing, context compilation, prompt building, or history trimming. Those
// concerns live in [internal/toolcall], [internal/contextengine], and
// [internal/promptcompat] respectively.
//
// # Status
//
// This package contains the interface definition only (M3 Stage 7 design
// deliverable). Concrete implementations are deferred to M4.
// See docs/archive/2026-05-v2-execution/dev-plan-governance.md §1.3 for the
// original design rationale and migration constraints.
package provider

import "context"

// CompletionRequest is the normalised request payload delivered to a Provider.
// By the time it reaches the Provider the prompt has already been built by
// [promptcompat.BuildOpenAIPrompt]; the Provider never sees raw messages,
// tool schemas, or protocol-specific shapes.
type CompletionRequest struct {
	// Prompt is the final assembled prompt string, ready to send upstream.
	Prompt string

	// ToolNames is the list of tool names injected into the prompt.
	// The Provider uses this only for logging / metrics; it does not parse
	// or filter tool calls.
	ToolNames []string

	// MaxTokens is the upstream max_tokens / max_completion_tokens value.
	// 0 means use the upstream default.
	MaxTokens int

	// Temperature is the sampling temperature. 0 uses the upstream default.
	Temperature float64

	// ThinkingEnabled signals that the model should emit a reasoning /
	// thinking block before the answer. Concrete providers map this to
	// their native thinking parameter (e.g. DeepSeek extended_thinking).
	ThinkingEnabled bool

	// TraceID is an opaque request identifier for structured logging.
	TraceID string
}

// CompletionChunk is a single data unit emitted over the streaming channel
// returned by [Provider.Complete].
type CompletionChunk struct {
	// Text is the assistant text delta for this chunk. May be empty when
	// ToolCall is non-nil.
	Text string

	// ToolCall is non-nil when this chunk carries a partial or complete
	// tool-call argument fragment.
	ToolCall *ToolCallChunk

	// IsDone signals that the stream has finished. The Provider must send
	// exactly one IsDone=true chunk as the last emission before closing the
	// channel. No further chunks may be sent after IsDone.
	IsDone bool

	// UsageTokens is the total token count reported by the upstream for the
	// entire request. Populated only on the final (IsDone=true) chunk.
	UsageTokens int
}

// ToolCallChunk carries a partial or complete tool-call fragment. Because
// responses stream token-by-token, Arguments may be delivered in multiple
// chunks with the same ID; callers concatenate them.
type ToolCallChunk struct {
	// ID is the upstream tool-call identifier (stable across chunks for one call).
	ID string

	// Name is the function name (present on the first chunk for this call).
	Name string

	// Arguments is the partial JSON arguments delta for this chunk.
	Arguments string
}

// Provider is the minimal interface for an upstream LLM completion service.
//
// Implementations MUST:
//   - Send one or more CompletionChunk values over ch while the request is live.
//   - Send exactly one chunk with IsDone=true as the final emission.
//   - Close ch after sending the IsDone chunk.
//   - Honour ctx cancellation and stop sending / close ch promptly on cancel.
//
// Implementations MUST NOT:
//   - Parse or re-emit raw protocol messages (OpenAI / Claude / Gemini shape).
//   - Perform prompt construction, tool-call XML parsing, or history trimming.
//   - Block indefinitely; apply upstream timeouts internally.
type Provider interface {
	// Complete initiates a streaming completion request and delivers results
	// over ch. The caller owns the channel creation; Complete only sends to it.
	// Complete returns nil on clean stream termination, or an error if the
	// upstream request failed before IsDone was sent.
	Complete(ctx context.Context, req CompletionRequest, ch chan<- CompletionChunk) error

	// Name returns a stable identifier for this provider, used in structured
	// logs and metrics (e.g. "deepseek-web", "gemini", "openai").
	Name() string
}
