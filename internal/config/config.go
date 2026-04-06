package config

type Config struct {
	Keys             []string          `json:"keys,omitempty"`
	Accounts         []Account         `json:"accounts,omitempty"`
	Proxies          []Proxy           `json:"proxies,omitempty"`
	ClaudeMapping    map[string]string `json:"claude_mapping,omitempty"`
	ClaudeModelMap   map[string]string `json:"claude_model_mapping,omitempty"`
	ModelAliases     map[string]string `json:"model_aliases,omitempty"`
	Admin            AdminConfig       `json:"admin,omitempty"`
	Runtime          RuntimeConfig     `json:"runtime,omitempty"`
	Compat           CompatConfig      `json:"compat,omitempty"`
	Responses        ResponsesConfig   `json:"responses,omitempty"`
	Embeddings       EmbeddingsConfig  `json:"embeddings,omitempty"`
	AutoDelete       AutoDeleteConfig  `json:"auto_delete"`
	VercelSyncHash   string            `json:"_vercel_sync_hash,omitempty"`
	VercelSyncTime   int64             `json:"_vercel_sync_time,omitempty"`
	AdditionalFields map[string]any    `json:"-"`
}

type Account struct {
	Email    string `json:"email,omitempty"`
	Mobile   string `json:"mobile,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	ProxyID  string `json:"proxy_id,omitempty"`
}

type Proxy struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (c *Config) ClearAccountTokens() {
	if c == nil {
		return
	}
	for i := range c.Accounts {
		c.Accounts[i].Token = ""
	}
}

// DropInvalidAccounts removes accounts that cannot be addressed by admin APIs
// (no email and no normalizable mobile). This prevents legacy token-only
// records from becoming orphaned empty entries after token stripping.
func (c *Config) DropInvalidAccounts() {
	if c == nil || len(c.Accounts) == 0 {
		return
	}
	kept := make([]Account, 0, len(c.Accounts))
	for _, acc := range c.Accounts {
		if acc.Identifier() == "" {
			continue
		}
		kept = append(kept, acc)
	}
	c.Accounts = kept
}

type CompatConfig struct {
	WideInputStrictOutput *bool `json:"wide_input_strict_output,omitempty"`
	StripReferenceMarkers *bool `json:"strip_reference_markers,omitempty"`
}

type AdminConfig struct {
	PasswordHash      string `json:"password_hash,omitempty"`
	JWTExpireHours    int    `json:"jwt_expire_hours,omitempty"`
	JWTValidAfterUnix int64  `json:"jwt_valid_after_unix,omitempty"`
}

type RuntimeConfig struct {
	AccountMaxInflight        int `json:"account_max_inflight,omitempty"`
	AccountMaxQueue           int `json:"account_max_queue,omitempty"`
	GlobalMaxInflight         int `json:"global_max_inflight,omitempty"`
	TokenRefreshIntervalHours int `json:"token_refresh_interval_hours,omitempty"`
}

type ResponsesConfig struct {
	StoreTTLSeconds int `json:"store_ttl_seconds,omitempty"`
}

type EmbeddingsConfig struct {
	Provider string `json:"provider,omitempty"`
}

type AutoDeleteConfig struct {
	Mode     string `json:"mode,omitempty"`
	Sessions bool   `json:"sessions,omitempty"`
}
