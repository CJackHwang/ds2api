package config

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisConfigKey          = "ds2api:config"
	redisOpTimeout                 = 3 * time.Second
	defaultVercelDeleteLeaseTTL    = 15 * time.Second
	redisMetricsTotalCallsField    = "total_calls"
	redisMetricsSuccessCallsField  = "success_calls"
	redisMetricsFailedCallsField   = "failed_calls"
	redisMetricsLastUpdatedAtField = "last_updated_at"
)

type storageBackend string

const (
	storageBackendFile   storageBackend = "file"
	storageBackendEnv    storageBackend = "env"
	storageBackendRedis  storageBackend = "redis"
	storageBackendMemory storageBackend = "memory"
)

type CallMetrics struct {
	TotalCalls    int64 `json:"total_calls"`
	SuccessCalls  int64 `json:"success_calls"`
	FailedCalls   int64 `json:"failed_calls"`
	LastUpdatedAt int64 `json:"last_updated_at,omitempty"`
}

type redisRuntime struct {
	client                *redis.Client
	configKey             string
	accountsIndexKey      string
	accountKeyPrefix      string
	metricsKey            string
	autoDeleteLeasePrefix string
}

func newRedisRuntimeFromEnv() *redisRuntime {
	rawURL := strings.TrimSpace(firstNonEmptyEnv(
		"DS2API_REDIS_URL",
		"KV_URL",
		"REDIS_URL",
		"UPSTASH_REDIS_URL",
	))
	return newRedisRuntime(rawURL, redisConfigKeyFromEnv())
}

func newRedisRuntime(rawURL, configKey string) *redisRuntime {
	if rawURL == "" {
		return nil
	}
	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		Logger.Warn("[redis] invalid url", "error", err)
		return nil
	}
	client := redis.NewClient(opts)
	configKey = strings.TrimSpace(configKey)
	if configKey == "" {
		configKey = redisConfigKeyFromEnv()
	}
	return &redisRuntime{
		client:                client,
		configKey:             configKey,
		accountsIndexKey:      configKey + ":accounts:index",
		accountKeyPrefix:      configKey + ":accounts:",
		metricsKey:            configKey + ":metrics",
		autoDeleteLeasePrefix: configKey + ":auto_delete_lease:",
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func redisConfigKeyFromEnv() string {
	if key := strings.TrimSpace(os.Getenv("DS2API_REDIS_KEY")); key != "" {
		return key
	}
	prefix := strings.TrimSpace(os.Getenv("DS2API_REDIS_KEY_PREFIX"))
	if prefix != "" {
		prefix = strings.TrimSuffix(prefix, ":")
		return prefix + ":config"
	}
	return defaultRedisConfigKey
}

func (r *redisRuntime) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), redisOpTimeout)
}

func (r *redisRuntime) loadConfig(ctx context.Context) (Config, bool, error) {
	if r == nil || r.client == nil {
		return Config{}, false, nil
	}
	raw, err := r.client.Get(ctx, r.configKey).Result()
	if errors.Is(err, redis.Nil) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	cfg, err := parseConfigString(raw)
	if err != nil {
		return Config{}, true, err
	}
	cfg.ClearAccountTokens()
	cfg.DropInvalidAccounts()
	return cfg, true, nil
}

func (r *redisRuntime) saveConfig(ctx context.Context, cfg Config) error {
	if r == nil || r.client == nil {
		return nil
	}
	persistCfg := cfg.Clone()
	persistCfg.Accounts = nil
	persistCfg.ClearAccountTokens()
	b, err := json.Marshal(persistCfg)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, r.configKey, b, 0).Err()
}

func (r *redisRuntime) loadAccounts(ctx context.Context) ([]Account, bool, error) {
	if r == nil || r.client == nil {
		return nil, false, nil
	}
	identifiers, err := r.readAccountsIndex(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(identifiers) == 0 {
		return nil, false, nil
	}

	keys := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		keys = append(keys, r.accountStorageKey(identifier))
	}
	values, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, true, err
	}

	accounts := make([]Account, 0, len(values))
	for _, value := range values {
		raw, ok := value.(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		var acc Account
		if err := json.Unmarshal([]byte(raw), &acc); err != nil {
			return nil, true, err
		}
		acc.Token = ""
		accounts = append(accounts, acc)
	}
	return normalizeStoreAccounts(accounts), true, nil
}

func (r *redisRuntime) saveAccounts(ctx context.Context, accounts []Account) error {
	if r == nil || r.client == nil {
		return nil
	}
	existingIdentifiers, err := r.readAccountsIndex(ctx)
	if err != nil {
		return err
	}

	accounts = normalizeStoreAccounts(accounts)
	nextIdentifiers := make([]string, 0, len(accounts))
	nextSet := make(map[string]struct{}, len(accounts))

	pipe := r.client.TxPipeline()
	for _, acc := range accounts {
		identifier := acc.Identifier()
		if identifier == "" {
			continue
		}
		nextIdentifiers = append(nextIdentifiers, identifier)
		nextSet[identifier] = struct{}{}

		persistAcc := acc
		persistAcc.Token = ""
		b, err := json.Marshal(persistAcc)
		if err != nil {
			return err
		}
		pipe.Set(ctx, r.accountStorageKey(identifier), b, 0)
	}
	for _, identifier := range existingIdentifiers {
		if _, ok := nextSet[identifier]; ok {
			continue
		}
		pipe.Del(ctx, r.accountStorageKey(identifier))
	}
	if len(nextIdentifiers) == 0 {
		pipe.Del(ctx, r.accountsIndexKey)
	} else {
		b, err := json.Marshal(nextIdentifiers)
		if err != nil {
			return err
		}
		pipe.Set(ctx, r.accountsIndexKey, b, 0)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *redisRuntime) saveState(ctx context.Context, cfg Config, accounts []Account) error {
	if r == nil || r.client == nil {
		return nil
	}
	if err := r.saveConfig(ctx, cfg); err != nil {
		return err
	}
	return r.saveAccounts(ctx, accounts)
}

func (r *redisRuntime) readAccountsIndex(ctx context.Context) ([]string, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}
	raw, err := r.client.Get(ctx, r.accountsIndexKey).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var identifiers []string
	if err := json.Unmarshal([]byte(raw), &identifiers); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(identifiers))
	seen := make(map[string]struct{}, len(identifiers))
	for _, identifier := range identifiers {
		identifier = strings.TrimSpace(identifier)
		if identifier == "" {
			continue
		}
		if _, ok := seen[identifier]; ok {
			continue
		}
		seen[identifier] = struct{}{}
		out = append(out, identifier)
	}
	return out, nil
}

func (r *redisRuntime) accountStorageKey(identifier string) string {
	return r.accountKeyPrefix + stableRedisKeyFragment(identifier)
}

func (r *redisRuntime) loadCallMetrics(ctx context.Context) (CallMetrics, error) {
	if r == nil || r.client == nil {
		return CallMetrics{}, nil
	}
	values, err := r.client.HMGet(ctx, r.metricsKey,
		redisMetricsTotalCallsField,
		redisMetricsSuccessCallsField,
		redisMetricsFailedCallsField,
		redisMetricsLastUpdatedAtField,
	).Result()
	if err != nil {
		return CallMetrics{}, err
	}
	if len(values) != 4 {
		return CallMetrics{}, nil
	}
	return CallMetrics{
		TotalCalls:    int64FromRedis(values[0]),
		SuccessCalls:  int64FromRedis(values[1]),
		FailedCalls:   int64FromRedis(values[2]),
		LastUpdatedAt: int64FromRedis(values[3]),
	}, nil
}

func (r *redisRuntime) recordCall(ctx context.Context, success bool) error {
	if r == nil || r.client == nil {
		return nil
	}
	now := time.Now().Unix()
	pipe := r.client.TxPipeline()
	pipe.HIncrBy(ctx, r.metricsKey, redisMetricsTotalCallsField, 1)
	if success {
		pipe.HIncrBy(ctx, r.metricsKey, redisMetricsSuccessCallsField, 1)
	} else {
		pipe.HIncrBy(ctx, r.metricsKey, redisMetricsFailedCallsField, 1)
	}
	pipe.HSet(ctx, r.metricsKey, redisMetricsLastUpdatedAtField, now)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRuntime) acquireAutoDeleteLease(ctx context.Context, accountID string, ttl time.Duration) (bool, error) {
	if r == nil || r.client == nil {
		return true, nil
	}
	key := r.autoDeleteLeasePrefix + stableRedisKeyFragment(accountID)
	return r.client.SetNX(ctx, key, time.Now().Unix(), ttl).Result()
}

func int64FromRedis(v any) int64 {
	switch n := v.(type) {
	case nil:
		return 0
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func stableRedisKeyFragment(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(strings.ToLower(value))))
	return hex.EncodeToString(sum[:8])
}
