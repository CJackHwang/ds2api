package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestAccountIdentifierRequiresEmailOrMobile(t *testing.T) {
	acc := Account{Token: "example-token-value"}
	id := acc.Identifier()
	if id != "" {
		t.Fatalf("expected empty identifier when only token is present, got %q", id)
	}
}

func TestLoadStoreClearsTokensFromConfigInput(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"u@example.com","password":"p","token":"token-only-account"}]
	}`)

	store := LoadStore()
	accounts := store.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Token != "" {
		t.Fatalf("expected token to be cleared after loading, got %q", accounts[0].Token)
	}
}

func TestLoadStorePreservesProxiesAndAccountProxyAssignment(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"proxies":[
			{
				"id":"proxy-sh-1",
				"name":"Shanghai Exit",
				"type":"socks5h",
				"host":"127.0.0.1",
				"port":1080,
				"username":"demo",
				"password":"secret"
			}
		],
		"accounts":[
			{
				"email":"u@example.com",
				"password":"p",
				"proxy_id":"proxy-sh-1"
			}
		]
	}`)

	store := LoadStore()
	snap := store.Snapshot()
	if len(snap.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(snap.Proxies))
	}
	if snap.Proxies[0].ID != "proxy-sh-1" {
		t.Fatalf("unexpected proxy id: %#v", snap.Proxies[0])
	}
	if snap.Proxies[0].Type != "socks5h" {
		t.Fatalf("unexpected proxy type: %#v", snap.Proxies[0])
	}
	if len(snap.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(snap.Accounts))
	}
	if snap.Accounts[0].ProxyID != "proxy-sh-1" {
		t.Fatalf("expected account proxy assignment preserved, got %#v", snap.Accounts[0])
	}
}

func TestLoadStoreDropsLegacyTokenOnlyAccounts(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"accounts":[
			{"token":"legacy-token-only"},
			{"email":"u@example.com","password":"p","token":"runtime-token"}
		]
	}`)

	store := LoadStore()
	accounts := store.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("expected token-only account to be dropped, got %d accounts", len(accounts))
	}
	if accounts[0].Identifier() != "u@example.com" {
		t.Fatalf("unexpected remaining account: %#v", accounts[0])
	}
	if accounts[0].Token != "" {
		t.Fatalf("expected persisted token to be cleared, got %q", accounts[0].Token)
	}
}

func TestLoadStorePreservesFileBackedTokensForRuntime(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(`{
		"accounts":[{"email":"u@example.com","password":"p","token":"persisted-token"}]
	}`); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("DS2API_CONFIG_PATH", tmp.Name())

	store := LoadStore()
	accounts := store.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Token != "persisted-token" {
		t.Fatalf("expected file-backed token preserved for runtime use, got %q", accounts[0].Token)
	}
}

func TestLoadStoreIgnoresLegacyConfigJSONEnv(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("CONFIG_JSON", `{"keys":["legacy-key"],"accounts":[{"email":"legacy@example.com","password":"p"}]}`)
	t.Setenv("DS2API_CONFIG_PATH", path)

	store := LoadStore()
	if store.HasEnvConfigSource() {
		t.Fatal("expected legacy CONFIG_JSON to be ignored")
	}
	if store.IsEnvBacked() {
		t.Fatal("expected store to remain file-backed/empty when only CONFIG_JSON is set")
	}
	if len(store.Keys()) != 0 || len(store.Accounts()) != 0 {
		t.Fatalf("expected ignored legacy env to leave store empty, got keys=%d accounts=%d", len(store.Keys()), len(store.Accounts()))
	}
}

func TestEnvBackedStoreWritebackBootstrapsMissingConfigFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"],"accounts":[{"email":"seed@example.com","password":"p"}]}`)
	t.Setenv("DS2API_CONFIG_PATH", path)
	t.Setenv("DS2API_ENV_WRITEBACK", "1")

	store := LoadStore()
	if store.IsEnvBacked() {
		t.Fatalf("expected writeback bootstrap to become file-backed immediately")
	}
	if err := store.Update(func(c *Config) error {
		c.Accounts = append(c.Accounts, Account{Email: "new@example.com", Password: "p2"})
		return nil
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if !strings.Contains(string(content), "seed@example.com") {
		t.Fatalf("expected bootstrapped config to contain seed account, got: %s", content)
	}
	if !strings.Contains(string(content), "new@example.com") {
		t.Fatalf("expected persisted config to contain added account, got: %s", content)
	}

	reloaded := LoadStore()
	if reloaded.IsEnvBacked() {
		t.Fatalf("expected reloaded store to prefer persisted config file")
	}
	accounts := reloaded.Accounts()
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts after reload, got %d", len(accounts))
	}
}

func TestEnvBackedStoreWritebackDoesNotBootstrapOnInvalidEnvJSON(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("DS2API_CONFIG_JSON", "{invalid-json")
	t.Setenv("DS2API_CONFIG_PATH", path)
	t.Setenv("DS2API_ENV_WRITEBACK", "1")

	cfg, fromEnv, loadErr := loadConfig()
	if loadErr == nil {
		t.Fatalf("expected loadConfig error for invalid env json")
	}
	if !fromEnv {
		t.Fatalf("expected fromEnv=true when parsing env config fails")
	}
	if len(cfg.Keys) != 0 || len(cfg.Accounts) != 0 {
		t.Fatalf("expected empty config on parse failure, got keys=%d accounts=%d", len(cfg.Keys), len(cfg.Accounts))
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no bootstrapped config file, stat err=%v", statErr)
	}
}

func TestEnvBackedStoreWritebackDoesNotBootstrapOnInvalidSemanticConfig(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"seed@example.com","password":"p"}],
		"runtime":{"account_max_inflight":300}
	}`)
	t.Setenv("DS2API_CONFIG_PATH", path)
	t.Setenv("DS2API_ENV_WRITEBACK", "1")

	cfg, fromEnv, loadErr := loadConfig()
	if loadErr == nil {
		t.Fatalf("expected loadConfig error for invalid runtime config")
	}
	if !fromEnv {
		t.Fatalf("expected fromEnv=true when env config is the source")
	}
	if !strings.Contains(loadErr.Error(), "runtime.account_max_inflight") {
		t.Fatalf("expected runtime validation error, got %v", loadErr)
	}
	if len(cfg.Keys) != 1 || len(cfg.Accounts) != 1 {
		t.Fatalf("expected env config to be parsed before validation, got keys=%d accounts=%d", len(cfg.Keys), len(cfg.Accounts))
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected invalid config not to be bootstrapped, stat err=%v", statErr)
	}
}

func TestLoadStoreWithErrorRejectsInvalidRuntimeConfig(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"u@example.com","password":"p"}],
		"runtime":{"account_max_inflight":300}
	}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	if _, err := LoadStoreWithError(); err == nil {
		t.Fatal("expected LoadStoreWithError to reject invalid runtime config")
	} else if !strings.Contains(err.Error(), "runtime.account_max_inflight") {
		t.Fatalf("expected runtime validation error, got %v", err)
	}
}

func TestEnvBackedStoreWritebackFallsBackToPersistedFileOnInvalidEnvJSON(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	if _, err := tmp.WriteString(`{"keys":["file-key"],"accounts":[{"email":"persisted@example.com","password":"p"}]}`); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmp.Close()

	t.Setenv("DS2API_CONFIG_JSON", "{invalid-json")
	t.Setenv("DS2API_CONFIG_PATH", path)
	t.Setenv("DS2API_ENV_WRITEBACK", "1")

	cfg, fromEnv, loadErr := loadConfig()
	if loadErr != nil {
		t.Fatalf("expected fallback to persisted file, got error: %v", loadErr)
	}
	if fromEnv {
		t.Fatalf("expected fallback to file-backed mode")
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "file-key" {
		t.Fatalf("unexpected keys after fallback: %#v", cfg.Keys)
	}
	if len(cfg.Accounts) != 1 || cfg.Accounts[0].Email != "persisted@example.com" {
		t.Fatalf("unexpected accounts after fallback: %#v", cfg.Accounts)
	}
}

func TestRuntimeTokenRefreshIntervalHoursDefaultsToSix(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"u@example.com","password":"p"}]
	}`)

	store := LoadStore()
	if got := store.RuntimeTokenRefreshIntervalHours(); got != 6 {
		t.Fatalf("expected default refresh interval 6, got %d", got)
	}
}

func TestRuntimeTokenRefreshIntervalHoursUsesConfigValue(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"u@example.com","password":"p"}],
		"runtime":{"token_refresh_interval_hours":9}
	}`)

	store := LoadStore()
	if got := store.RuntimeTokenRefreshIntervalHours(); got != 9 {
		t.Fatalf("expected configured refresh interval 9, got %d", got)
	}
}

func TestStoreUpdateAccountTokenKeepsIdentifierResolvable(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"accounts":[{"email":"user@example.com","password":"p"}]
	}`)

	store := LoadStore()
	before := store.Accounts()
	if len(before) != 1 {
		t.Fatalf("expected 1 account, got %d", len(before))
	}
	oldID := before[0].Identifier()
	if err := store.UpdateAccountToken(oldID, "new-token"); err != nil {
		t.Fatalf("update token failed: %v", err)
	}

	if got, ok := store.FindAccount(oldID); !ok || got.Token != "new-token" {
		t.Fatalf("expected find by stable account identifier")
	}
}

func TestLoadStoreRejectsInvalidFieldType(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":"not-array","accounts":[]}`)
	store := LoadStore()
	if len(store.Keys()) != 0 || len(store.Accounts()) != 0 {
		t.Fatalf("expected empty store when config type is invalid")
	}
}

func TestParseConfigStringSupportsQuotedBase64Prefix(t *testing.T) {
	rawJSON := `{"keys":["k1"],"accounts":[{"email":"u@example.com","password":"p"}]}`
	b64 := base64.StdEncoding.EncodeToString([]byte(rawJSON))
	cfg, err := parseConfigString(`"base64:` + b64 + `"`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "k1" {
		t.Fatalf("unexpected keys: %#v", cfg.Keys)
	}
}

func TestParseConfigStringSupportsRawURLBase64(t *testing.T) {
	rawJSON := `{"keys":["k-url"],"accounts":[]}`
	b64 := base64.RawURLEncoding.EncodeToString([]byte(rawJSON))
	cfg, err := parseConfigString(b64)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "k-url" {
		t.Fatalf("unexpected keys: %#v", cfg.Keys)
	}
}

func TestLoadConfigOnVercelWithoutConfigFileFallsBackToMemory(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("DS2API_CONFIG_PATH", "testdata/does-not-exist.json")

	cfg, fromEnv, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fromEnv {
		t.Fatalf("expected fromEnv=true for vercel fallback")
	}
	if len(cfg.Keys) != 0 || len(cfg.Accounts) != 0 {
		t.Fatalf("expected empty bootstrap config, got keys=%d accounts=%d", len(cfg.Keys), len(cfg.Accounts))
	}
}

func TestAccountTestStatusIsRuntimeOnlyAndNotPersisted(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(`{
		"accounts":[{"email":"u@example.com","password":"p","test_status":"ok"}]
	}`); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("DS2API_CONFIG_PATH", tmp.Name())

	store := LoadStore()
	if got, ok := store.AccountTestStatus("u@example.com"); ok || got != "" {
		t.Fatalf("expected no runtime status loaded from config, got %q", got)
	}
	if err := store.UpdateAccountTestStatus("u@example.com", "ok"); err != nil {
		t.Fatalf("update test status: %v", err)
	}
	if got, ok := store.AccountTestStatus("u@example.com"); !ok || got != "ok" {
		t.Fatalf("expected runtime status to be available, got %q (ok=%v)", got, ok)
	}

	content, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(content), "test_status") {
		t.Fatalf("expected test_status to stay out of persisted config, got: %s", content)
	}
}
