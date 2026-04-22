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
	persistCfg.ClearAccountTokens()
	b, err := json.Marshal(persistCfg)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, r.configKey, b, 0).Err()
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
