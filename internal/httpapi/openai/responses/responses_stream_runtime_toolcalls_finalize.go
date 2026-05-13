package responses

import (
	"ds2api/internal/toolcall"
	"encoding/json"
	"sort"
	"strings"

	openaifmt "ds2api/internal/format/openai"
)

func (s *responsesStreamRuntime) closeIncompleteFunctionItems() {
	if len(s.functionAdded) == 0 {
		return
	}
	indices := make([]int, 0, len(s.functionAdded))
	for idx, added := range s.functionAdded {
		if !added || s.functionDone[idx] {
			continue
		}
		indices = append(indices, idx)
	}
	if len(indices) == 0 {
		return
	}
	sort.Ints(indices)
	for _, idx := range indices {
		name := strings.TrimSpace(s.functionNames[idx])
		if name == "" {
			continue
		}
		args := strings.TrimSpace(s.functionArgs[idx])
		if args == "" {
			args = "{}"
		}
		outputIndex := s.ensureFunctionOutputIndex(idx)
		itemID := s.ensureFunctionItemID(idx)
		callID := s.ensureToolCallID(idx)
		s.sendEvent(
			"response.function_call_arguments.done",
			openaifmt.BuildResponsesFunctionCallArgumentsDonePayload(s.responseID, itemID, outputIndex, callID, name, args),
		)
		item := map[string]any{
			"id":        itemID,
			"type":      "function_call",
			"call_id":   callID,
			"name":      name,
			"arguments": args,
			"status":    "completed",
		}
		s.sendEvent(
			"response.output_item.done",
			openaifmt.BuildResponsesOutputItemDonePayload(s.responseID, itemID, outputIndex, item),
		)
		s.functionDone[idx] = true
		s.toolCallsDoneEmitted = true
	}
}

func (s *responsesStreamRuntime) buildCompletedResponseObject(finalThinking, finalText string, calls []toolcall.ParsedToolCall) map[string]any {
	type indexedItem struct {
		index int
		item  map[string]any
	}
	indexed := make([]indexedItem, 0, len(calls)+1)

	if s.messageAdded {
		text := s.visibleText.String()
		indexed = append(indexed, indexedItem{
			index: s.ensureMessageOutputIndex(),
			item: map[string]any{
				"id":     s.ensureMessageItemID(),
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": text,
					},
				},
			},
		})
	} else if len(calls) > 0 && strings.TrimSpace(finalThinking) != "" {
		indexed = append(indexed, indexedItem{
			index: s.ensureMessageOutputIndex(),
			item: map[string]any{
				"id":     s.ensureMessageItemID(),
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{
						"type": "reasoning",
						"text": finalThinking,
					},
				},
			},
		})
	} else if len(calls) == 0 {
		content := make([]map[string]any, 0, 2)
		if finalThinking != "" {
			content = append(content, map[string]any{
				"type": "reasoning",
				"text": finalThinking,
			})
		}
		if finalText != "" {
			content = append(content, map[string]any{
				"type": "output_text",
				"text": finalText,
			})
		}
		if len(content) > 0 {
			indexed = append(indexed, indexedItem{
				index: s.ensureMessageOutputIndex(),
				item: map[string]any{
					"id":      s.ensureMessageItemID(),
					"type":    "message",
					"role":    "assistant",
					"status":  "completed",
					"content": content,
				},
			})
		}
	}

	normalizedCalls := toolcall.NormalizeParsedToolCallsForSchemas(calls, s.toolsRaw)
	for idx, tc := range normalizedCalls {
		if strings.TrimSpace(tc.Name) == "" {
			continue
		}
		argsBytes, _ := json.Marshal(tc.Input)
		indexed = append(indexed, indexedItem{
			index: s.ensureFunctionOutputIndex(idx),
			item: map[string]any{
				"id":        s.ensureFunctionItemID(idx),
				"type":      "function_call",
				"call_id":   s.ensureToolCallID(idx),
				"name":      tc.Name,
				"arguments": string(argsBytes),
				"status":    "completed",
			},
		})
	}

	sort.SliceStable(indexed, func(i, j int) bool {
		return indexed[i].index < indexed[j].index
	})
	output := make([]any, 0, len(indexed))
	for _, it := range indexed {
		output = append(output, it.item)
	}

	outputText := s.visibleText.String()
	if outputText == "" && len(calls) == 0 {
		if finalText != "" {
			outputText = finalText
		} else if finalThinking != "" {
			outputText = finalThinking
		}
	}

	obj := openaifmt.BuildResponseObjectFromItems(
		s.responseID,
		s.model,
		s.finalPrompt,
		finalThinking,
		finalText,
		output,
		outputText,
	)
	if s.refFileTokens > 0 {
		addRefFileTokensToUsage(obj, s.refFileTokens)
	}
	return obj
}
