package openai

import (
	"fmt"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/util"
)

func normalizeOpenAIChatRequest(store ConfigReader, req map[string]any, traceID string) (util.StandardRequest, error) {
	model, _ := req["model"].(string)
	messagesRaw, _ := req["messages"].([]any)
	if strings.TrimSpace(model) == "" || len(messagesRaw) == 0 {
		return util.StandardRequest{}, fmt.Errorf("Request must include 'model' and 'messages'.")
	}
	resolvedModel, ok := config.ResolveModel(store, model)
	if !ok {
		return util.StandardRequest{}, fmt.Errorf("Model '%s' is not available.", model)
	}
	thinkingEnabled, searchEnabled, _ := config.GetModelConfig(resolvedModel)
	responseModel := strings.TrimSpace(model)
	if responseModel == "" {
		responseModel = resolvedModel
	}
	toolPolicy := util.DefaultToolChoicePolicy()
	finalPrompt, toolNames := buildOpenAIFinalPromptWithPolicy(messagesRaw, req["tools"], traceID, toolPolicy)
	toolNames = ensureToolDetectionEnabled(toolNames, req["tools"])
	passThrough := collectOpenAIChatPassThrough(req)

	return util.StandardRequest{
		Surface:        "openai_chat",
		RequestedModel: strings.TrimSpace(model),
		ResolvedModel:  resolvedModel,
		ResponseModel:  responseModel,
		Messages:       messagesRaw,
		FinalPrompt:    finalPrompt,
		ToolNames:      toolNames,
		ToolChoice:     toolPolicy,
		Stream:         util.ToBool(req["stream"]),
		Thinking:       thinkingEnabled,
		Search:         searchEnabled,
		PassThrough:    passThrough,
	}, nil
}

func normalizeOpenAIResponsesRequest(store ConfigReader, req map[string]any, traceID string) (util.StandardRequest, error) {
	model, _ := req["model"].(string)
	model = strings.TrimSpace(model)
	if model == "" {
		return util.StandardRequest{}, fmt.Errorf("Request must include 'model'.")
	}
	resolvedModel, ok := config.ResolveModel(store, model)
	if !ok {
		return util.StandardRequest{}, fmt.Errorf("Model '%s' is not available.", model)
	}
	thinkingEnabled, searchEnabled, _ := config.GetModelConfig(resolvedModel)

	// Keep width-control as an explicit policy hook even if current default is true.
	allowWideInput := true
	if store != nil {
		allowWideInput = store.CompatWideInputStrictOutput()
	}
	var messagesRaw []any
	if allowWideInput {
		messagesRaw = responsesMessagesFromRequest(req)
	} else if msgs, ok := req["messages"].([]any); ok && len(msgs) > 0 {
		messagesRaw = msgs
	}
	if len(messagesRaw) == 0 {
		return util.StandardRequest{}, fmt.Errorf("Request must include 'input' or 'messages'.")
	}
	toolPolicy, err := parseToolChoicePolicy(req["tool_choice"], req["tools"])
	if err != nil {
		return util.StandardRequest{}, err
	}
	finalPrompt, toolNames := buildOpenAIFinalPromptWithPolicy(messagesRaw, req["tools"], traceID, toolPolicy)
	toolNames = ensureToolDetectionEnabled(toolNames, req["tools"])
	if !toolPolicy.IsNone() {
		toolPolicy.Allowed = namesToSet(toolNames)
	}
	passThrough := collectOpenAIChatPassThrough(req)

	return util.StandardRequest{
		Surface:        "openai_responses",
		RequestedModel: model,
		ResolvedModel:  resolvedModel,
		ResponseModel:  model,
		Messages:       messagesRaw,
		FinalPrompt:    finalPrompt,
		ToolNames:      toolNames,
		ToolChoice:     toolPolicy,
		Stream:         util.ToBool(req["stream"]),
		Thinking:       thinkingEnabled,
		Search:         searchEnabled,
		PassThrough:    passThrough,
	}, nil
}

func ensureToolDetectionEnabled(toolNames []string, toolsRaw any) []string {
	if len(toolNames) > 0 {
		return toolNames
	}
	tools, _ := toolsRaw.([]any)
	if len(tools) == 0 {
		return toolNames
	}
	// Keep stream sieve/tool buffering enabled even when client tool schemas
	// are malformed or lack explicit names; parsed tool payload names are no
	// longer filtered by this list.
	return []string{"__any_tool__"}
}

func collectOpenAIChatPassThrough(req map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"presence_penalty",
		"frequency_penalty",
		"stop",
	} {
		if v, ok := req[k]; ok {
			out[k] = v
		}
	}
	return out
}

func parseToolChoicePolicy(toolChoiceRaw any, toolsRaw any) (util.ToolChoicePolicy, error) {
	policy := util.DefaultToolChoicePolicy()
	declaredNames := extractDeclaredToolNames(toolsRaw)
	declaredSet := namesToSet(declaredNames)
	if len(declaredNames) > 0 {
		policy.Allowed = declaredSet
	}

	if toolChoiceRaw == nil {
		return policy, nil
	}

	switch v := toolChoiceRaw.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "auto":
			policy.Mode = util.ToolChoiceAuto
		case "none":
			policy.Mode = util.ToolChoiceNone
			policy.Allowed = nil
		case "required":
			policy.Mode = util.ToolChoiceRequired
		default:
			return util.ToolChoicePolicy{}, fmt.Errorf("Unsupported tool_choice: %q", v)
		}
	case map[string]any:
		allowedOverride, hasAllowedOverride, err := parseAllowedToolNames(v["allowed_tools"])
		if err != nil {
			return util.ToolChoicePolicy{}, err
		}
		if hasAllowedOverride {
			filtered := make([]string, 0, len(allowedOverride))
			for _, name := range allowedOverride {
				if _, ok := declaredSet[name]; !ok {
					return util.ToolChoicePolicy{}, fmt.Errorf("tool_choice.allowed_tools contains undeclared tool %q", name)
				}
				filtered = append(filtered, name)
			}
			policy.Allowed = namesToSet(filtered)
		}

		typ := strings.ToLower(strings.TrimSpace(asString(v["type"])))
		switch typ {
		case "", "auto":
			if hasFunctionSelector(v) {
				name, err := parseForcedToolName(v)
				if err != nil {
					return util.ToolChoicePolicy{}, err
				}
				policy.Mode = util.ToolChoiceForced
				policy.ForcedName = name
				policy.Allowed = namesToSet([]string{name})
			} else {
				policy.Mode = util.ToolChoiceAuto
			}
		case "none":
			policy.Mode = util.ToolChoiceNone
			policy.Allowed = nil
		case "required":
			policy.Mode = util.ToolChoiceRequired
		case "function":
			name, err := parseForcedToolName(v)
			if err != nil {
				return util.ToolChoicePolicy{}, err
			}
			policy.Mode = util.ToolChoiceForced
			policy.ForcedName = name
			policy.Allowed = namesToSet([]string{name})
		default:
			return util.ToolChoicePolicy{}, fmt.Errorf("Unsupported tool_choice.type: %q", typ)
		}
	default:
		return util.ToolChoicePolicy{}, fmt.Errorf("tool_choice must be a string or object")
	}

	if policy.Mode == util.ToolChoiceRequired || policy.Mode == util.ToolChoiceForced {
		if len(declaredNames) == 0 {
			return util.ToolChoicePolicy{}, fmt.Errorf("tool_choice=%s requires non-empty tools.", policy.Mode)
		}
	}
	if policy.Mode == util.ToolChoiceForced {
		if _, ok := declaredSet[policy.ForcedName]; !ok {
			return util.ToolChoicePolicy{}, fmt.Errorf("tool_choice forced function %q is not declared in tools", policy.ForcedName)
		}
	}
	if len(policy.Allowed) == 0 && (policy.Mode == util.ToolChoiceRequired || policy.Mode == util.ToolChoiceForced) {
		return util.ToolChoicePolicy{}, fmt.Errorf("tool_choice policy resolved to empty allowed tool set")
	}
	return policy, nil
}

func parseForcedToolName(v map[string]any) (string, error) {
	if name := strings.TrimSpace(asString(v["name"])); name != "" {
		return name, nil
	}
	if fn, ok := v["function"].(map[string]any); ok {
		if name := strings.TrimSpace(asString(fn["name"])); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("tool_choice function requires name")
}

func parseAllowedToolNames(raw any) ([]string, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	collectName := func(v any) string {
		if name := strings.TrimSpace(asString(v)); name != "" {
			return name
		}
		if m, ok := v.(map[string]any); ok {
			if name := strings.TrimSpace(asString(m["name"])); name != "" {
				return name
			}
			if fn, ok := m["function"].(map[string]any); ok {
				if name := strings.TrimSpace(asString(fn["name"])); name != "" {
					return name
				}
			}
		}
		return ""
	}

	names := []string{}
	switch x := raw.(type) {
	case []any:
		for _, item := range x {
			name := collectName(item)
			if name == "" {
				return nil, true, fmt.Errorf("tool_choice.allowed_tools contains invalid item")
			}
			names = append(names, name)
		}
	case []string:
		for _, item := range x {
			name := strings.TrimSpace(item)
			if name == "" {
				return nil, true, fmt.Errorf("tool_choice.allowed_tools contains empty name")
			}
			names = append(names, name)
		}
	default:
		return nil, true, fmt.Errorf("tool_choice.allowed_tools must be an array")
	}

	if len(names) == 0 {
		return nil, true, fmt.Errorf("tool_choice.allowed_tools must not be empty")
	}
	return names, true, nil
}

func hasFunctionSelector(v map[string]any) bool {
	if strings.TrimSpace(asString(v["name"])) != "" {
		return true
	}
	if fn, ok := v["function"].(map[string]any); ok {
		return strings.TrimSpace(asString(fn["name"])) != ""
	}
	return false
}

func extractDeclaredToolNames(toolsRaw any) []string {
	tools, ok := toolsRaw.([]any)
	if !ok || len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	seen := map[string]struct{}{}
	for _, t := range tools {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if len(fn) == 0 {
			fn = tool
		}
		name := strings.TrimSpace(asString(fn["name"]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func namesToSet(names []string) map[string]struct{} {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
