package config

import (
	"fmt"
	"strings"
)

func ValidateConfig(c Config) error {
	c.Normalize()
	if err := ValidateProxyConfig(c.Proxies); err != nil {
		return err
	}
	if err := ValidateAdminConfig(c.Admin); err != nil {
		return err
	}
	if err := ValidateRuntimeConfig(c.Runtime); err != nil {
		return err
	}
	if err := ValidateResponsesConfig(c.Responses); err != nil {
		return err
	}
	if err := ValidateEmbeddingsConfig(c.Embeddings); err != nil {
		return err
	}
	if err := ValidateAutoDeleteConfig(c.AutoDelete); err != nil {
		return err
	}
	if err := ValidateAccountProxyReferences(c.Accounts, c.Proxies); err != nil {
		return err
	}
	return nil
}

func ValidateProxyConfig(proxies []Proxy) error {
	seen := make(map[string]struct{}, len(proxies))
	for _, proxy := range proxies {
		proxy = NormalizeProxy(proxy)
		if err := ValidateTrimmedString("proxies.id", proxy.ID, true); err != nil {
			return err
		}
		switch proxy.Type {
		case "socks5", "socks5h":
		default:
			return fmt.Errorf("proxies.type must be one of socks5, socks5h")
		}
		if err := ValidateTrimmedString("proxies.host", proxy.Host, true); err != nil {
			return err
		}
		if err := ValidateIntRange("proxies.port", proxy.Port, 1, 65535, true); err != nil {
			return err
		}
		if _, ok := seen[proxy.ID]; ok {
			return fmt.Errorf("duplicate proxy id: %s", proxy.ID)
		}
		seen[proxy.ID] = struct{}{}
	}
	return nil
}

func ValidateAccountProxyReferences(accounts []Account, proxies []Proxy) error {
	if len(accounts) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(proxies))
	for _, proxy := range proxies {
		ids[NormalizeProxy(proxy).ID] = struct{}{}
	}
	for _, acc := range accounts {
		proxyID := strings.TrimSpace(acc.ProxyID)
		if proxyID == "" {
			continue
		}
		if _, ok := ids[proxyID]; !ok {
			return fmt.Errorf("account proxy_id references unknown proxy: %s", proxyID)
		}
	}
	return nil
}

func ValidateAdminConfig(admin AdminConfig) error {
	return ValidateIntRange("admin.jwt_expire_hours", admin.JWTExpireHours, 1, 720, false)
}

func ValidateRuntimeConfig(runtime RuntimeConfig) error {
	if err := ValidateIntRange("runtime.account_max_inflight", runtime.AccountMaxInflight, 1, 256, false); err != nil {
		return err
	}
	if err := ValidateIntRange("runtime.account_max_queue", runtime.AccountMaxQueue, 1, 200000, false); err != nil {
		return err
	}
	if err := ValidateIntRange("runtime.global_max_inflight", runtime.GlobalMaxInflight, 1, 200000, false); err != nil {
		return err
	}
	if err := ValidateIntRange("runtime.token_refresh_interval_hours", runtime.TokenRefreshIntervalHours, 1, 720, false); err != nil {
		return err
	}
	if runtime.AccountMaxInflight > 0 && runtime.GlobalMaxInflight > 0 && runtime.GlobalMaxInflight < runtime.AccountMaxInflight {
		return fmt.Errorf("runtime.global_max_inflight must be >= runtime.account_max_inflight")
	}
	return nil
}

func ValidateResponsesConfig(responses ResponsesConfig) error {
	return ValidateIntRange("responses.store_ttl_seconds", responses.StoreTTLSeconds, 30, 86400, false)
}

func ValidateEmbeddingsConfig(embeddings EmbeddingsConfig) error {
	return ValidateTrimmedString("embeddings.provider", embeddings.Provider, false)
}

func ValidateAutoDeleteConfig(autoDelete AutoDeleteConfig) error {
	return ValidateAutoDeleteMode(autoDelete.Mode)
}

func ValidateIntRange(name string, value, min, max int, required bool) error {
	if value == 0 && !required {
		return nil
	}
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return nil
}

func ValidateTrimmedString(name, value string, required bool) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if !required && value == "" {
			return nil
		}
		return fmt.Errorf("%s cannot be empty", name)
	}
	return nil
}

func ValidateAutoDeleteMode(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "none", "single", "all":
		return nil
	default:
		return fmt.Errorf("auto_delete.mode must be one of none, single, all")
	}
}
