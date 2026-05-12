package config

import (
	"os"
	"strconv"
	"strings"
)

func (s *Store) ModelAliases() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := DefaultModelAliases()
	for k, v := range s.cfg.ModelAliases {
		key := strings.TrimSpace(lower(k))
		val := strings.TrimSpace(lower(v))
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	return out
}

func (s *Store) ToolcallMode() string {
	return "feature_match"
}

func (s *Store) ToolcallEarlyEmitConfidence() string {
	return "high"
}

func (s *Store) ResponsesStoreTTLSeconds() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Responses.StoreTTLSeconds > 0 {
		return s.cfg.Responses.StoreTTLSeconds
	}
	return 900
}

func (s *Store) EmbeddingsProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Embeddings.Provider)
}

func (s *Store) AutoDeleteMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mode := strings.ToLower(strings.TrimSpace(s.cfg.AutoDelete.Mode))
	switch mode {
	case "none", "single", "all":
		return mode
	}
	if s.cfg.AutoDelete.Sessions {
		return "all"
	}
	return "none"
}

func (s *Store) AdminPasswordHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Admin.PasswordHash)
}

func (s *Store) AdminJWTExpireHours() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Admin.JWTExpireHours > 0 {
		return s.cfg.Admin.JWTExpireHours
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_JWT_EXPIRE_HOURS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 24
}

func (s *Store) AdminJWTValidAfterUnix() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Admin.JWTValidAfterUnix
}

func (s *Store) RuntimeAccountMaxInflight() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.AccountMaxInflight > 0 {
		return s.cfg.Runtime.AccountMaxInflight
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_INFLIGHT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 2
}

func (s *Store) RuntimeAccountMaxQueue(defaultSize int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.AccountMaxQueue > 0 {
		return s.cfg.Runtime.AccountMaxQueue
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_QUEUE")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return n
		}
	}
	if defaultSize < 0 {
		return 0
	}
	return defaultSize
}

func (s *Store) RuntimeGlobalMaxInflight(defaultSize int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.GlobalMaxInflight > 0 {
		return s.cfg.Runtime.GlobalMaxInflight
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_GLOBAL_MAX_INFLIGHT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	if defaultSize < 0 {
		return 0
	}
	return defaultSize
}

func (s *Store) RuntimeTokenRefreshIntervalHours() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.TokenRefreshIntervalHours > 0 {
		return s.cfg.Runtime.TokenRefreshIntervalHours
	}
	return 6
}

func (s *Store) AutoDeleteSessions() bool {
	return s.AutoDeleteMode() != "none"
}

func (s *Store) CurrentInputFileEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.CurrentInputFile.Enabled == nil {
		return true
	}
	return *s.cfg.CurrentInputFile.Enabled
}

func (s *Store) CurrentInputFileMinChars() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.CurrentInputFile.MinChars
}

func (s *Store) ThinkingInjectionEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.ThinkingInjection.Enabled == nil {
		return true
	}
	return *s.cfg.ThinkingInjection.Enabled
}

func (s *Store) ThinkingInjectionPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.ThinkingInjection.Prompt)
}

// ContextEngineMode returns the context engine feature flag value.
// Valid values: "off" (default) | "shadow" | "enforce".
// The DS2API_CONTEXT_ENGINE environment variable takes precedence over the
// config file value.
func (s *Store) ContextEngineMode() string {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_CONTEXT_ENGINE"))); raw != "" {
		switch raw {
		case "shadow", "enforce":
			return raw
		case "off":
			return "off"
		default:
			return "off"
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch strings.ToLower(strings.TrimSpace(s.cfg.ContextEngine.Mode)) {
	case "shadow", "enforce":
		return strings.ToLower(strings.TrimSpace(s.cfg.ContextEngine.Mode))
	}
	return "off"
}

// ParserV2Mode returns the tool-call parser v2 feature flag value.
// Valid values: "off" (default) | "shadow" | "enforce".
// The DS2API_PARSER_V2 environment variable takes precedence over the
// config file value.
func (s *Store) ParserV2Mode() string {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_PARSER_V2"))); raw != "" {
		switch raw {
		case "shadow", "enforce":
			return raw
		case "off":
			return "off"
		default:
			return "off"
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch strings.ToLower(strings.TrimSpace(s.cfg.ParserV2.Mode)) {
	case "shadow", "enforce":
		return strings.ToLower(strings.TrimSpace(s.cfg.ParserV2.Mode))
	}
	return "off"
}

// AllowDefaultAdminKey reports whether the server is permitted to start when
// no admin credential is configured (i.e. UsingDefaultAdminKey is true).
//
// Resolution order (first match wins):
//  1. DS2API_ALLOW_DEFAULT_ADMIN_KEY environment variable.
//     Truthy:  "1" / "true" / "yes" / "on"  → true (allow start, print warning).
//     Falsy:   "0" / "false" / "no" / "off" → false (fail-closed).
//  2. config.admin.allow_default_admin_key (explicit *bool).
//  3. Default: false (fail-closed; requires explicit opt-in).
func (s *Store) AllowDefaultAdminKey() bool {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY"))); raw != "" {
		switch raw {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Admin.AllowDefaultAdminKey != nil {
		return *s.cfg.Admin.AllowDefaultAdminKey
	}
	return false
}

// AllowGeminiQueryKey reports whether the Gemini-compatible
// `?key=` / `?api_key=` query parameters are accepted as caller credential
// fallbacks when no header-based credential is present.
//
// Resolution order (first match wins):
//  1. DS2API_ALLOW_GEMINI_QUERY_KEY environment variable.
//     Truthy values:  "1" / "true" / "yes" / "on"  → true.
//     Falsy values:   "0" / "false" / "no" / "off" → false.
//     Any other non-empty value is ignored.
//  2. config.auth.allow_gemini_query_key (explicit *bool).
//  3. Default: true (preserves legacy AI Studio compatibility).
func (s *Store) AllowGeminiQueryKey() bool {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_ALLOW_GEMINI_QUERY_KEY"))); raw != "" {
		switch raw {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Auth.AllowGeminiQueryKey != nil {
		return *s.cfg.Auth.AllowGeminiQueryKey
	}
	return true
}

func (s *Store) CORSAllowOrigins() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw := s.cfg.CORS.AllowOrigins
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, o := range raw {
		if v := strings.ToLower(strings.TrimSpace(o)); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Store) Log() LogConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Log
}

func (s *Store) LogFile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Log.File != "" {
		return s.cfg.Log.File
	}
	return "logs/ds2api.log" // default
}

func (s *Store) LogFileEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Log.FileEnabled
}

func (s *Store) LogLevel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	level := strings.TrimSpace(s.cfg.Log.Level)
	if level != "" {
		return level
	}
	// Fallback to LOG_LEVEL env var
	if envLevel := strings.TrimSpace(os.Getenv("LOG_LEVEL")); envLevel != "" {
		return strings.ToLower(envLevel)
	}
	return "info" // default
}
