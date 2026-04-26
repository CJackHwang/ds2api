package openai

import (
	"encoding/json"

	openaifmt "ds2api/internal/format/openai"
)

func (s *responsesStreamRuntime) nextSequence() int {
	s.sequence++
	return s.sequence
}

func (s *responsesStreamRuntime) sendEvent(event string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["sequence_number"]; !ok {
		payload["sequence_number"] = s.nextSequence()
	}
	b, _ := json.Marshal(payload)
	_, _ = s.w.Write([]byte("event: " + event + "\n"))
	_, _ = s.w.Write([]byte("data: "))
	_, _ = s.w.Write(b)
	_, _ = s.w.Write([]byte("\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *responsesStreamRuntime) sendCreated() {
	s.sendEvent("response.created", openaifmt.BuildResponsesCreatedPayload(s.responseID, s.model))
}

func (s *responsesStreamRuntime) sendDone() {
	_, _ = s.w.Write([]byte("data: [DONE]\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *responsesStreamRuntime) processToolStreamEvents(events []toolStreamEvent, emitContent bool, resetAfterToolCalls bool) {
	for _, evt := range events {
		if emitContent && evt.Content != "" { cleaned := stripVisibleToolCallMarkup(cleanVisibleOutput(evt.Content, s.stripReferenceMarkers)); if cleaned != "" { s.emitTextDelta(cleaned) } }
			s.emitTextDelta(evt.Content)
		}
		if len(evt.ToolCallDeltas) > 0 {
			if !s.emitEarlyToolDeltas {
				continue
			}
			filtered := filterIncrementalToolCallDeltasByAllowed(evt.ToolCallDeltas, s.functionNames)
			if len(filtered) == 0 {
				continue
			}
			s.emitFunctionCallDeltaEvents(filtered)
		}
		if len(evt.ToolCalls) > 0 { filteredCalls := filterNewParsedToolCalls(evt.ToolCalls, s.emittedToolCallKeys); if len(filteredCalls) == 0 { continue } s.emitFunctionCallDoneEvents(filteredCalls); if resetAfterToolCalls { s.resetStreamToolCallState() } }
			s.emitFunctionCallDoneEvents(evt.ToolCalls)
			if resetAfterToolCalls {
				s.resetStreamToolCallState()
			}
		}
	}
}
