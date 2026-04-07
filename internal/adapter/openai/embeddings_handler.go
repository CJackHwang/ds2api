package openai

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/usagestats"
	"ds2api/internal/util"
)

func (h *Handler) Embeddings(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if err == auth.ErrNoAccount {
			status = http.StatusTooManyRequests
		}
		writeOpenAIError(w, status, detail)
		return
	}
	defer h.Auth.Release(a)

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	model, _ := req["model"].(string)
	model = strings.TrimSpace(model)
	if model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "Request must include 'model'.")
		return
	}
	if _, ok := config.ResolveModel(h.Store, model); !ok {
		writeOpenAIError(w, http.StatusBadRequest, fmt.Sprintf("Model '%s' is not available.", model))
		return
	}
	h.recordUsage(a, usagestats.Event{
		Surface:        "openai_embeddings",
		RequestedModel: model,
		ResolvedModel:  model,
		ResponseModel:  model,
	})

	inputs := extractEmbeddingInputs(req["input"])
	if len(inputs) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "Request must include non-empty 'input'.")
		return
	}

	provider := ""
	if h.Store != nil {
		provider = strings.ToLower(strings.TrimSpace(h.Store.EmbeddingsProvider()))
	}
	if provider == "" {
		writeOpenAIError(w, http.StatusNotImplemented, "Embeddings provider is not configured. Set embeddings.provider in config.")
		return
	}
	switch provider {
	case "mock", "deterministic", "builtin":
		// supported local deterministic provider
	default:
		writeOpenAIError(w, http.StatusNotImplemented, fmt.Sprintf("Embeddings provider '%s' is not supported.", provider))
		return
	}

	data := make([]map[string]any, 0, len(inputs))
	totalTokens := 0
	for i, input := range inputs {
		totalTokens += util.EstimateTokens(input)
		data = append(data, map[string]any{
			"object":    "embedding",
			"index":     i,
			"embedding": deterministicEmbedding(input),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
		"model":  model,
		"usage": map[string]any{
			"prompt_tokens": totalTokens,
			"total_tokens":  totalTokens,
		},
	})
}

func extractEmbeddingInputs(raw any) []string {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			switch iv := item.(type) {
			case string:
				s := strings.TrimSpace(iv)
				if s != "" {
					out = append(out, s)
				}
			case []any:
				// Token array input support: convert to stable string form.
				out = append(out, fmt.Sprintf("%v", iv))
			default:
				s := strings.TrimSpace(fmt.Sprintf("%v", iv))
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func deterministicEmbedding(input string) []float64 {
	// Keep response shape stable without external dependencies.
	const dims = 64
	out := make([]float64, dims)
	seed := sha256.Sum256([]byte(input))
	buf := seed[:]
	for i := 0; i < dims; i++ {
		if len(buf) < 4 {
			next := sha256.Sum256(buf)
			buf = next[:]
		}
		v := binary.BigEndian.Uint32(buf[:4])
		buf = buf[4:]
		// map [0, 2^32) -> [-1, 1]
		out[i] = (float64(v)/2147483647.5 - 1.0)
	}
	return out
}
