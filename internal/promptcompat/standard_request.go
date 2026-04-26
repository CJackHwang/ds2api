package promptcompat

import "ds2api/internal/config"

type StandardRequest struct {
	Surface        string
	RequestedModel string
	ResolvedModel  string
	ResponseModel  string
	Messages       []any
	HistoryText    string
	ToolsRaw       any
	FinalPrompt    string
	ToolNames      []string
	ToolChoice     ToolChoicePolicy
	Stream         bool
	Thinking       bool
	Search         bool
	RefFileIDs     []string
	Diagnostics    RequestDiagnostics
	PassThrough    map[string]any
}

type RequestDiagnostics struct {
	CurrentInputUpload *FileUploadDiagnostic
	HistoryUpload      *FileUploadDiagnostic
	CurrentInputReason string
	HistoryReason      string
	RefFileIDs         []string
}

type FileUploadDiagnostic struct {
	Filename string
	Bytes    int
	FileID   string
}

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceForced   ToolChoiceMode = "forced"
)

type ToolChoicePolicy struct {
	Mode       ToolChoiceMode
	ForcedName string
	Allowed    map[string]struct{}
}

func DefaultToolChoicePolicy() ToolChoicePolicy {
	return ToolChoicePolicy{Mode: ToolChoiceAuto}
}

func (p ToolChoicePolicy) IsNone() bool {
	return p.Mode == ToolChoiceNone
}

func (p ToolChoicePolicy) IsRequired() bool {
	return p.Mode == ToolChoiceRequired || p.Mode == ToolChoiceForced
}

func (p ToolChoicePolicy) Allows(name string) bool {
	if len(p.Allowed) == 0 {
		return true
	}
	_, ok := p.Allowed[name]
	return ok
}

func (r StandardRequest) CompletionPayload(sessionID string) map[string]any {
	modelID := r.ResolvedModel
	if modelID == "" {
		modelID = r.RequestedModel
	}
	modelType := "default"
	if resolvedType, ok := config.GetModelType(modelID); ok {
		modelType = resolvedType
	}
	refFileIDs := make([]any, 0, len(r.RefFileIDs))
	for _, fileID := range r.RefFileIDs {
		if fileID == "" {
			continue
		}
		refFileIDs = append(refFileIDs, fileID)
	}
	payload := map[string]any{
		"chat_session_id":   sessionID,
		"model_type":        modelType,
		"parent_message_id": nil,
		"prompt":            r.FinalPrompt,
		"ref_file_ids":      refFileIDs,
		"thinking_enabled":  r.Thinking,
		"search_enabled":    r.Search,
	}
	for k, v := range r.PassThrough {
		payload[k] = v
	}
	return payload
}

func (d RequestDiagnostics) Clone() RequestDiagnostics {
	return RequestDiagnostics{
		CurrentInputUpload: cloneFileUploadDiagnostic(d.CurrentInputUpload),
		HistoryUpload:      cloneFileUploadDiagnostic(d.HistoryUpload),
		CurrentInputReason: d.CurrentInputReason,
		HistoryReason:      d.HistoryReason,
		RefFileIDs:         CloneStringSlice(d.RefFileIDs),
	}
}

func cloneFileUploadDiagnostic(in *FileUploadDiagnostic) *FileUploadDiagnostic {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func CloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
