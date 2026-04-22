package config

import (
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestLoadStoreBootstrapsLegacyConfigIntoRedis(t *testing.T) {
	redisServer := miniredis.RunT(t)
	t.Setenv("DS2API_REDIS_URL", "redis://"+redisServer.Addr())
	t.Setenv("DS2API_REDIS_KEY", "test:config")
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"user@example.com","password":"p","token":"runtime-token"}]
	}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	store := LoadStore()
	if got := store.StorageBackend(); got != "redis" {
		t.Fatalf("StorageBackend()=%q want=redis", got)
	}
	if !store.IsPersistentStorage() {
		t.Fatal("expected redis-backed store to report persistent storage")
	}
	if got := store.RedisConfigKey(); got != "test:config" {
		t.Fatalf("RedisConfigKey()=%q want=test:config", got)
	}

	raw, err := redisServer.Get("test:config")
	if err != nil {
		t.Fatalf("redis config missing: %v", err)
	}
	var persisted Config
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		t.Fatalf("unmarshal redis config: %v", err)
	}
	if len(persisted.Accounts) != 1 || persisted.Accounts[0].Token != "" {
		t.Fatalf("expected persisted redis config to clear runtime tokens, got %+v", persisted.Accounts)
	}
}

func TestStoreRecordCallPersistsMetricsToRedis(t *testing.T) {
	redisServer := miniredis.RunT(t)
	t.Setenv("DS2API_REDIS_URL", "redis://"+redisServer.Addr())
	t.Setenv("DS2API_REDIS_KEY", "test:config")
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	store := LoadStore()
	store.RecordCall(true)
	store.RecordCall(false)

	metrics := store.CallMetrics()
	if metrics.TotalCalls != 2 {
		t.Fatalf("TotalCalls=%d want=2", metrics.TotalCalls)
	}
	if metrics.SuccessCalls != 1 {
		t.Fatalf("SuccessCalls=%d want=1", metrics.SuccessCalls)
	}
	if metrics.FailedCalls != 1 {
		t.Fatalf("FailedCalls=%d want=1", metrics.FailedCalls)
	}
	if metrics.LastUpdatedAt == 0 {
		t.Fatal("expected LastUpdatedAt to be populated")
	}
}

func TestAcquireAutoDeleteAllLeaseUsesRedisLease(t *testing.T) {
	redisServer := miniredis.RunT(t)
	t.Setenv("DS2API_REDIS_URL", "redis://"+redisServer.Addr())
	t.Setenv("DS2API_REDIS_KEY", "test:config")
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")
	t.Setenv("DS2API_VERCEL_SESSION_DELETE_LEASE_TTL_SECONDS", "30")

	store := LoadStore()
	if !store.AcquireAutoDeleteAllLease("acct-1") {
		t.Fatal("expected first lease acquisition to succeed")
	}
	if store.AcquireAutoDeleteAllLease("acct-1") {
		t.Fatal("expected second lease acquisition to be rejected while ttl is active")
	}
	if !store.AcquireAutoDeleteAllLease("acct-2") {
		t.Fatal("expected different account lease to succeed")
	}
}
