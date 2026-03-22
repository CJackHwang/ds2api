package openai

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/util"
)

func TestHandleResponsesStreamToolCallsHideRawOutputTextInCompleted(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	rawToolJSON := `{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}`
	streamBody := sseLine(rawToolJSON) + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"read_file"}, util.DefaultToolChoicePolicy(), "")

	completed, ok := extractSSEEventPayload(rec.Body.String(), "response.completed")
	if !ok {
		t.Fatalf("expected response.completed event, body=%s", rec.Body.String())
	}
	responseObj, _ := completed["response"].(map[string]any)
	outputText, _ := responseObj["output_text"].(string)
	if outputText != "" {
		t.Fatalf("expected empty output_text for tool_calls response, got output_text=%q", outputText)
	}
	output, _ := responseObj["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("expected structured output entries, got %#v", responseObj["output"])
	}
	hasFunctionCall := false
	hasLegacyWrapper := false
	for _, item := range output {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if m["type"] == "function_call" {
			hasFunctionCall = true
		}
		if m["type"] == "tool_calls" {
			hasLegacyWrapper = true
		}
	}
	if !hasFunctionCall {
		t.Fatalf("expected function_call item, got %#v", responseObj["output"])
	}
	if hasLegacyWrapper {
		t.Fatalf("did not expect legacy tool_calls wrapper, got %#v", responseObj["output"])
	}
	if strings.Contains(outputText, `"tool_calls"`) {
		t.Fatalf("raw tool_calls JSON leaked in output_text: %q", outputText)
	}
}

func TestHandleResponsesStreamUsesOfficialOutputItemEvents(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine(`{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}`) + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"read_file"}, util.DefaultToolChoicePolicy(), "")
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_item.added") {
		t.Fatalf("expected response.output_item.added event, body=%s", body)
	}
	if !strings.Contains(body, "event: response.output_item.done") {
		t.Fatalf("expected response.output_item.done event, body=%s", body)
	}
	if !strings.Contains(body, "event: response.function_call_arguments.done") {
		t.Fatalf("expected response.function_call_arguments.done event, body=%s", body)
	}
	if strings.Contains(body, "event: response.output_tool_call.delta") || strings.Contains(body, "event: response.output_tool_call.done") {
		t.Fatalf("legacy response.output_tool_call.* event must not appear, body=%s", body)
	}

	addedPayloads := extractAllSSEEventPayloads(body, "response.output_item.added")
	hasFunctionCallAdded := false
	for _, payload := range addedPayloads {
		item, _ := payload["item"].(map[string]any)
		if item == nil || asString(item["type"]) != "function_call" {
			continue
		}
		hasFunctionCallAdded = true
		if asString(item["arguments"]) != "" {
			t.Fatalf("expected in-progress function_call.arguments to start empty string, got %#v", item["arguments"])
		}
	}
	if !hasFunctionCallAdded {
		t.Fatalf("expected function_call output_item.added payload, body=%s", body)
	}

	donePayload, ok := extractSSEEventPayload(body, "response.function_call_arguments.done")
	if !ok {
		t.Fatalf("expected to parse response.function_call_arguments.done payload, body=%s", body)
	}
	doneCallID := strings.TrimSpace(asString(donePayload["call_id"]))
	if doneCallID == "" {
		t.Fatalf("expected non-empty call_id in done payload, payload=%#v", donePayload)
	}
	completed, ok := extractSSEEventPayload(body, "response.completed")
	if !ok {
		t.Fatalf("expected response.completed payload, body=%s", body)
	}
	responseObj, _ := completed["response"].(map[string]any)
	output, _ := responseObj["output"].([]any)
	var completedCallID string
	for _, item := range output {
		m, _ := item.(map[string]any)
		if m == nil || m["type"] != "function_call" {
			continue
		}
		completedCallID = strings.TrimSpace(asString(m["call_id"]))
		if completedCallID != "" {
			break
		}
	}
	if completedCallID == "" {
		t.Fatalf("expected function_call.call_id in completed output, output=%#v", output)
	}
	if completedCallID != doneCallID {
		t.Fatalf("expected completed call_id to match stream done call_id, done=%q completed=%q", doneCallID, completedCallID)
	}
}

func TestHandleResponsesStreamDoesNotEmitReasoningTextCompatEvents(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	b, _ := json.Marshal(map[string]any{
		"p": "response/thinking_content",
		"v": "thought",
	})
	streamBody := "data: " + string(b) + "\n" + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-reasoner", "prompt", true, false, nil, util.DefaultToolChoicePolicy(), "")

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.reasoning.delta") {
		t.Fatalf("expected response.reasoning.delta event, body=%s", body)
	}
	if strings.Contains(body, "event: response.reasoning_text.delta") || strings.Contains(body, "event: response.reasoning_text.done") {
		t.Fatalf("did not expect response.reasoning_text.* compatibility events, body=%s", body)
	}
}

func TestHandleResponsesStreamMultiToolCallKeepsNameAndCallIDAligned(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine(`{"tool_calls":[{"name":"search_web","input":{"query":"latest ai news"}},`) +
		sseLine(`{"name":"eval_javascript","input":{"code":"1+1"}}]}`) +
		"data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"search_web", "eval_javascript"}, util.DefaultToolChoicePolicy(), "")

	body := rec.Body.String()
	donePayloads := extractAllSSEEventPayloads(body, "response.function_call_arguments.done")
	if len(donePayloads) != 2 {
		t.Fatalf("expected two response.function_call_arguments.done events, got %d body=%s", len(donePayloads), body)
	}
	seenNames := map[string]string{}
	for _, payload := range donePayloads {
		name := strings.TrimSpace(asString(payload["name"]))
		callID := strings.TrimSpace(asString(payload["call_id"]))
		if name != "search_web" && name != "eval_javascript" {
			t.Fatalf("unexpected tool name in done payload: %#v", payload)
		}
		if callID == "" {
			t.Fatalf("expected non-empty call_id in done payload: %#v", payload)
		}
		seenNames[name] = callID
	}
	if seenNames["search_web"] == seenNames["eval_javascript"] {
		t.Fatalf("expected distinct call_id per tool, got %#v", seenNames)
	}
}

func TestHandleResponsesStreamEmitsOutputTextDoneBeforeContentPartDone(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine("hello") + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, nil, util.DefaultToolChoicePolicy(), "")
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_text.done") {
		t.Fatalf("expected response.output_text.done payload, body=%s", body)
	}
	textDoneIdx := strings.Index(body, "event: response.output_text.done")
	partDoneIdx := strings.Index(body, "event: response.content_part.done")
	if textDoneIdx < 0 || partDoneIdx < 0 {
		t.Fatalf("expected output_text.done + content_part.done, body=%s", body)
	}
	if textDoneIdx > partDoneIdx {
		t.Fatalf("expected output_text.done before content_part.done, body=%s", body)
	}
}

func TestHandleResponsesStreamOutputTextDeltaCarriesItemIndexes(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine("hello") + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, nil, util.DefaultToolChoicePolicy(), "")
	body := rec.Body.String()

	deltaPayload, ok := extractSSEEventPayload(body, "response.output_text.delta")
	if !ok {
		t.Fatalf("expected response.output_text.delta payload, body=%s", body)
	}
	if strings.TrimSpace(asString(deltaPayload["item_id"])) == "" {
		t.Fatalf("expected non-empty item_id in output_text.delta, payload=%#v", deltaPayload)
	}
	if _, ok := deltaPayload["output_index"]; !ok {
		t.Fatalf("expected output_index in output_text.delta, payload=%#v", deltaPayload)
	}
	if _, ok := deltaPayload["content_index"]; !ok {
		t.Fatalf("expected content_index in output_text.delta, payload=%#v", deltaPayload)
	}
}

func TestHandleResponsesStreamThinkingAndMixedToolExampleEmitsFunctionCall(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(path, value string) string {
		b, _ := json.Marshal(map[string]any{
			"p": path,
			"v": value,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine("response/thinking_content", "thinking...") +
		sseLine("response/content", "先读取文件。") +
		sseLine("response/content", `{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}`) +
		"data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-reasoner", "prompt", true, false, []string{"read_file"}, util.DefaultToolChoicePolicy(), "")

	addedPayloads := extractAllSSEEventPayloads(rec.Body.String(), "response.output_item.added")
	if len(addedPayloads) < 1 {
		t.Fatalf("expected at least one output_item.added event, got %d body=%s", len(addedPayloads), rec.Body.String())
	}

	completedPayload, ok := extractSSEEventPayload(rec.Body.String(), "response.completed")
	if !ok {
		t.Fatalf("expected response.completed payload, body=%s", rec.Body.String())
	}
	responseObj, _ := completedPayload["response"].(map[string]any)
	output, _ := responseObj["output"].([]any)
	hasMessage := false
	hasFunctionCall := false
	for _, item := range output {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if asString(m["type"]) == "message" {
			hasMessage = true
		}
		if asString(m["type"]) == "function_call" {
			hasFunctionCall = true
		}
	}
	if !hasMessage {
		t.Fatalf("expected message output for mixed prose tool example, output=%#v", output)
	}
	if !hasFunctionCall {
		t.Fatalf("expected function_call output for mixed prose tool example, output=%#v", output)
	}
}

func TestHandleResponsesStreamToolChoiceNoneStillAllowsFunctionCall(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine(`{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}`) + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	policy := util.ToolChoicePolicy{Mode: util.ToolChoiceNone}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, nil, policy, "")
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.function_call_arguments.done") {
		t.Fatalf("expected function_call events for tool_choice=none, body=%s", body)
	}
}

func TestHandleResponsesStreamMalformedToolJSONFallsBackToText(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	// invalid JSON (NaN) should remain plain text in strict mode.
	streamBody := sseLine(`{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"},"x":NaN}]}`) + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"read_file"}, util.DefaultToolChoicePolicy(), "")
	body := rec.Body.String()
	if strings.Contains(body, "event: response.function_call_arguments.delta") || strings.Contains(body, "event: response.function_call_arguments.done") {
		t.Fatalf("did not expect function_call events for malformed payload in strict mode, body=%s", body)
	}
	if !strings.Contains(body, "event: response.output_text.delta") {
		t.Fatalf("expected response.output_text.delta for malformed payload, body=%s", body)
	}
	if !strings.Contains(body, "event: response.completed") {
		t.Fatalf("expected response.completed event, body=%s", body)
	}
}

func TestHandleResponsesStreamRequiredToolChoiceFailure(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine("plain text only") + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	policy := util.ToolChoicePolicy{
		Mode:    util.ToolChoiceRequired,
		Allowed: map[string]struct{}{"read_file": {}},
	}
	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"read_file"}, policy, "")

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.failed") {
		t.Fatalf("expected response.failed event for required tool_choice violation, body=%s", body)
	}
	if strings.Contains(body, "event: response.completed") {
		t.Fatalf("did not expect response.completed after failure, body=%s", body)
	}
}

func TestHandleResponsesStreamRequiredToolChoiceIgnoresThinkingToolPayload(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(path, value string) string {
		b, _ := json.Marshal(map[string]any{
			"p": path,
			"v": value,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine("response/thinking_content", `{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}`) +
		sseLine("response/content", "plain text only") +
		"data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	policy := util.ToolChoicePolicy{
		Mode:    util.ToolChoiceRequired,
		Allowed: map[string]struct{}{"read_file": {}},
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", true, false, []string{"read_file"}, policy, "")
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.failed") {
		t.Fatalf("expected response.failed event for required tool_choice violation, body=%s", body)
	}
	if strings.Contains(body, "event: response.completed") {
		t.Fatalf("did not expect response.completed after failure, body=%s", body)
	}
}

func TestHandleResponsesStreamRequiredMalformedToolPayloadFails(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine(`{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"},"x":NaN}]}`) + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	policy := util.ToolChoicePolicy{
		Mode:    util.ToolChoiceRequired,
		Allowed: map[string]struct{}{"read_file": {}},
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"read_file"}, policy, "")

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.failed") {
		t.Fatalf("expected response.failed event, body=%s", body)
	}
	if strings.Contains(body, "event: response.completed") {
		t.Fatalf("did not expect response.completed, body=%s", body)
	}
}

func TestHandleResponsesStreamAllowsUnknownToolName(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	sseLine := func(v string) string {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": v,
		})
		return "data: " + string(b) + "\n"
	}

	streamBody := sseLine(`{"tool_calls":[{"name":"not_in_schema","input":{"q":"go"}}]}`) + "data: [DONE]\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}

	h.handleResponsesStream(rec, req, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, false, []string{"read_file"}, util.DefaultToolChoicePolicy(), "")
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.function_call_arguments.done") {
		t.Fatalf("expected function_call events for unknown tool, body=%s", body)
	}
}

func TestHandleResponsesNonStreamRequiredToolChoiceViolation(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`data: {"p":"response/content","v":"plain text only"}` + "\n" +
				`data: [DONE]` + "\n",
		)),
	}
	policy := util.ToolChoicePolicy{
		Mode:    util.ToolChoiceRequired,
		Allowed: map[string]struct{}{"read_file": {}},
	}

	h.handleResponsesNonStream(rec, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, []string{"read_file"}, policy, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for required tool_choice violation, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "tool_choice_violation" {
		t.Fatalf("expected code=tool_choice_violation, got %#v", out)
	}
}

func TestHandleResponsesNonStreamRequiredToolChoiceIgnoresThinkingToolPayload(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`data: {"p":"response/thinking_content","v":"{\"tool_calls\":[{\"name\":\"read_file\",\"input\":{\"path\":\"README.MD\"}}]}"}` + "\n" +
				`data: {"p":"response/content","v":"plain text only"}` + "\n" +
				`data: [DONE]` + "\n",
		)),
	}
	policy := util.ToolChoicePolicy{
		Mode:    util.ToolChoiceRequired,
		Allowed: map[string]struct{}{"read_file": {}},
	}

	h.handleResponsesNonStream(rec, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", true, []string{"read_file"}, policy, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for required tool_choice violation, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "tool_choice_violation" {
		t.Fatalf("expected code=tool_choice_violation, got %#v", out)
	}
}

func TestHandleResponsesNonStreamToolChoiceNoneStillAllowsFunctionCall(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"read_file\",\"input\":{\"path\":\"README.MD\"}}]}"}` + "\n" +
				`data: [DONE]` + "\n",
		)),
	}
	policy := util.ToolChoicePolicy{Mode: util.ToolChoiceNone}

	h.handleResponsesNonStream(rec, resp, "owner-a", "resp_test", "deepseek-chat", "prompt", false, nil, policy, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for tool_choice=none handling, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	output, _ := out["output"].([]any)
	foundFunctionCall := false
	for _, item := range output {
		m, _ := item.(map[string]any)
		if m != nil && m["type"] == "function_call" {
			foundFunctionCall = true
		}
	}
	if !foundFunctionCall {
		t.Fatalf("expected function_call output item for tool_choice=none, got %#v", output)
	}
}

func extractSSEEventPayload(body, targetEvent string) (map[string]any, bool) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	matched := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "event: ") {
			evt := strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			matched = evt == targetEvent
			continue
		}
		if !matched || !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if raw == "" || raw == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, false
		}
		return payload, true
	}
	return nil, false
}

func extractAllSSEEventPayloads(body, targetEvent string) []map[string]any {
	scanner := bufio.NewScanner(strings.NewReader(body))
	matched := false
	out := make([]map[string]any, 0, 2)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "event: ") {
			evt := strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			matched = evt == targetEvent
			continue
		}
		if !matched || !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if raw == "" || raw == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			continue
		}
		out = append(out, payload)
	}
	return out
}
