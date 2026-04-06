package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func (c Config) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range c.AdditionalFields {
		m[k] = v
	}
	if len(c.Keys) > 0 {
		m["keys"] = c.Keys
	}
	if len(c.Accounts) > 0 {
		m["accounts"] = c.Accounts
	}
	if len(c.Proxies) > 0 {
		m["proxies"] = c.Proxies
	}
	if len(c.ClaudeMapping) > 0 {
		m["claude_mapping"] = c.ClaudeMapping
	}
	if len(c.ClaudeModelMap) > 0 {
		m["claude_model_mapping"] = c.ClaudeModelMap
	}
	if len(c.ModelAliases) > 0 {
		m["model_aliases"] = c.ModelAliases
	}
	if strings.TrimSpace(c.Admin.PasswordHash) != "" || c.Admin.JWTExpireHours > 0 || c.Admin.JWTValidAfterUnix > 0 {
		m["admin"] = c.Admin
	}
	if c.Runtime.AccountMaxInflight > 0 || c.Runtime.AccountMaxQueue > 0 || c.Runtime.GlobalMaxInflight > 0 || c.Runtime.TokenRefreshIntervalHours > 0 {
		m["runtime"] = c.Runtime
	}
	if c.Compat.WideInputStrictOutput != nil || c.Compat.StripReferenceMarkers != nil {
		m["compat"] = c.Compat
	}
	if c.Responses.StoreTTLSeconds > 0 {
		m["responses"] = c.Responses
	}
	if strings.TrimSpace(c.Embeddings.Provider) != "" {
		m["embeddings"] = c.Embeddings
	}
	m["auto_delete"] = c.AutoDelete
	if c.VercelSyncHash != "" {
		m["_vercel_sync_hash"] = c.VercelSyncHash
	}
	if c.VercelSyncTime != 0 {
		m["_vercel_sync_time"] = c.VercelSyncTime
	}
	return json.Marshal(m)
}

func (c *Config) UnmarshalJSON(b []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.AdditionalFields = map[string]any{}
	for k, v := range raw {
		switch k {
		case "keys":
			if err := json.Unmarshal(v, &c.Keys); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "accounts":
			if err := json.Unmarshal(v, &c.Accounts); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "proxies":
			if err := json.Unmarshal(v, &c.Proxies); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "claude_mapping":
			if err := json.Unmarshal(v, &c.ClaudeMapping); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "claude_model_mapping":
			if err := json.Unmarshal(v, &c.ClaudeModelMap); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "model_aliases":
			if err := json.Unmarshal(v, &c.ModelAliases); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "admin":
			if err := json.Unmarshal(v, &c.Admin); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "runtime":
			if err := json.Unmarshal(v, &c.Runtime); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "compat":
			if err := json.Unmarshal(v, &c.Compat); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "toolcall":
			// Legacy field ignored. Toolcall policy is fixed and no longer configurable.
		case "responses":
			if err := json.Unmarshal(v, &c.Responses); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "embeddings":
			if err := json.Unmarshal(v, &c.Embeddings); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "auto_delete":
			if err := json.Unmarshal(v, &c.AutoDelete); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "_vercel_sync_hash":
			if err := json.Unmarshal(v, &c.VercelSyncHash); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		case "_vercel_sync_time":
			if err := json.Unmarshal(v, &c.VercelSyncTime); err != nil {
				return fmt.Errorf("invalid field %q: %w", k, err)
			}
		default:
			var anyVal any
			if err := json.Unmarshal(v, &anyVal); err == nil {
				c.AdditionalFields[k] = anyVal
			}
		}
	}
	return nil
}

func (c Config) Clone() Config {
	clone := Config{
		Keys:           slices.Clone(c.Keys),
		Accounts:       slices.Clone(c.Accounts),
		Proxies:        slices.Clone(c.Proxies),
		ClaudeMapping:  cloneStringMap(c.ClaudeMapping),
		ClaudeModelMap: cloneStringMap(c.ClaudeModelMap),
		ModelAliases:   cloneStringMap(c.ModelAliases),
		Admin:          c.Admin,
		Runtime:        c.Runtime,
		Compat: CompatConfig{
			WideInputStrictOutput: cloneBoolPtr(c.Compat.WideInputStrictOutput),
			StripReferenceMarkers: cloneBoolPtr(c.Compat.StripReferenceMarkers),
		},
		Responses:        c.Responses,
		Embeddings:       c.Embeddings,
		AutoDelete:       c.AutoDelete,
		VercelSyncHash:   c.VercelSyncHash,
		VercelSyncTime:   c.VercelSyncTime,
		AdditionalFields: map[string]any{},
	}
	for k, v := range c.AdditionalFields {
		clone.AdditionalFields[k] = v
	}
	return clone
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBoolPtr(in *bool) *bool {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func parseConfigString(raw string) (Config, error) {
	var cfg Config
	candidates := []string{raw}
	if normalized := normalizeConfigInput(raw); normalized != raw {
		candidates = append(candidates, normalized)
	}
	for _, candidate := range candidates {
		if err := json.Unmarshal([]byte(candidate), &cfg); err == nil {
			return cfg, nil
		}
	}

	base64Input := candidates[len(candidates)-1]
	decoded, err := decodeConfigBase64(base64Input)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DS2API_CONFIG_JSON: %w", err)
	}
	if err := json.Unmarshal(decoded, &cfg); err != nil {
		return Config{}, fmt.Errorf("invalid DS2API_CONFIG_JSON decoded JSON: %w", err)
	}
	return cfg, nil
}

func normalizeConfigInput(raw string) string {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return normalized
	}
	for {
		changed := false
		if len(normalized) >= 2 {
			first := normalized[0]
			last := normalized[len(normalized)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				normalized = strings.TrimSpace(normalized[1 : len(normalized)-1])
				changed = true
			}
		}
		if strings.HasPrefix(strings.ToLower(normalized), "base64:") {
			normalized = strings.TrimSpace(normalized[len("base64:"):])
			changed = true
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(normalized)
}

func decodeConfigBase64(raw string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(raw)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("base64 decode failed")
}
