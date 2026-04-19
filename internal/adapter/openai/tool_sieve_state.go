package openai

import (
	"ds2api/internal/toolcall"
	"strings"
)

type toolStreamSieveState struct {
	pending               strings.Builder
	capture               strings.Builder
	capturing             bool
	codeFenceStack        []int
	codeFencePendingTicks int
	codeFenceLineStart    bool
	recentTextTail        string
	pendingToolRaw        string
	pendingToolCalls      []toolcall.ParsedToolCall
	disableDeltas         bool
	toolNameSent          bool
	toolName              string
	toolArgsStart         int
	toolArgsSent          int
	toolArgsString        bool
	toolArgsDone          bool
}

type toolStreamEvent struct {
	Content        string
	ToolCalls      []toolcall.ParsedToolCall
	ToolCallDeltas []toolCallDelta
}

type toolCallDelta struct {
	Index     int
	Name      string
	Arguments string
}

// Keep in sync with JS TOOL_SIEVE_CONTEXT_TAIL_LIMIT.
const toolSieveContextTailLimit = 2048

func (s *toolStreamSieveState) resetIncrementalToolState() {
	s.disableDeltas = false
	s.toolNameSent = false
	s.toolName = ""
	s.toolArgsStart = -1
	s.toolArgsSent = -1
	s.toolArgsString = false
	s.toolArgsDone = false
}

func (s *toolStreamSieveState) noteText(content string) {
	if !hasMeaningfulText(content) {
		return
	}
	updateCodeFenceState(s, content)
	s.recentTextTail = appendTail(s.recentTextTail, content, toolSieveContextTailLimit)
}

func appendTail(prev, next string, max int) string {
	if max <= 0 {
		return ""
	}
	combined := prev + next
	if len(combined) <= max {
		return combined
	}
	return combined[len(combined)-max:]
}

func hasMeaningfulText(text string) bool {
	return strings.TrimSpace(text) != ""
}

func insideCodeFenceWithState(state *toolStreamSieveState, text string) bool {
	if state == nil {
		return insideCodeFence(text)
	}
	simulated := simulateCodeFenceState(
		state.codeFenceStack,
		state.codeFencePendingTicks,
		state.codeFenceLineStart,
		text,
	)
	return len(simulated.stack) > 0
}

func insideCodeFence(text string) bool {
	if text == "" {
		return false
	}
	return len(simulateCodeFenceState(nil, 0, true, text).stack) > 0
}

func updateCodeFenceState(state *toolStreamSieveState, text string) {
	if state == nil || !hasMeaningfulText(text) {
		return
	}
	next := simulateCodeFenceState(
		state.codeFenceStack,
		state.codeFencePendingTicks,
		state.codeFenceLineStart,
		text,
	)
	state.codeFenceStack = next.stack
	state.codeFencePendingTicks = next.pendingTicks
	state.codeFenceLineStart = next.lineStart
}

type codeFenceSimulation struct {
	stack        []int
	pendingTicks int
	lineStart    bool
}

func simulateCodeFenceState(stack []int, pendingTicks int, lineStart bool, text string) codeFenceSimulation {
	chunk := text
	nextStack := append([]int(nil), stack...)
	ticks := pendingTicks
	atLineStart := lineStart

	flushTicks := func() {
		if ticks > 0 {
			if atLineStart && ticks >= 3 {
				applyFenceMarker(&nextStack, ticks)
			}
			atLineStart = false
			ticks = 0
		}
	}

	for i := 0; i < len(chunk); i++ {
		ch := chunk[i]
		if ch == '`' {
			ticks++
			continue
		}
		flushTicks()
		switch ch {
		case '\n', '\r':
			atLineStart = true
		case ' ', '\t':
			if atLineStart {
				continue
			}
			atLineStart = false
		default:
			atLineStart = false
		}
	}

	return codeFenceSimulation{
		stack:        nextStack,
		pendingTicks: ticks,
		lineStart:    atLineStart,
	}
}

func applyFenceMarker(stack *[]int, ticks int) {
	if stack == nil || ticks <= 0 {
		return
	}
	if len(*stack) == 0 {
		*stack = append(*stack, ticks)
		return
	}
	top := (*stack)[len(*stack)-1]
	if ticks >= top {
		*stack = (*stack)[:len(*stack)-1]
		return
	}
	*stack = append(*stack, ticks)
}
