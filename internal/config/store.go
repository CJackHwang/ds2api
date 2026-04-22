package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu       sync.RWMutex
	cfg      Config
	path     string
	fromEnv  bool
	backend  storageBackend
	redis    *redisRuntime
	keyMap   map[string]struct{} // O(1) API key lookup index
	accMap   map[string]int      // O(1) account lookup: identifier -> slice index
	accTest  map[string]string   // runtime-only account test status cache
	metrics  CallMetrics
	leaseMap map[string]time.Time
}

func LoadStore() *Store {
	store, err := loadStore()
	if err != nil {
		Logger.Warn("[config] load failed", "error", err)
	}
	if len(store.cfg.Keys) == 0 && len(store.cfg.Accounts) == 0 {
		Logger.Warn("[config] empty config loaded")
	}
	store.rebuildIndexes()
	return store
}

func LoadStoreWithError() (*Store, error) {
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	store.rebuildIndexes()
	return store, nil
}

func loadStore() (*Store, error) {
	redisState := newRedisRuntimeFromEnv()
	cfg, fromEnv, backend, err := loadConfigWithBackend(redisState)
	if validateErr := ValidateConfig(cfg); validateErr != nil {
		err = errors.Join(err, validateErr)
	}
	return &Store{
		cfg:      cfg,
		path:     ConfigPath(),
		fromEnv:  fromEnv,
		backend:  backend,
		redis:    redisState,
		leaseMap: map[string]time.Time{},
	}, err
}

func loadConfig() (Config, bool, error) {
	cfg, fromEnv, _, err := loadConfigWithBackend(nil)
	return cfg, fromEnv, err
}

func loadConfigWithBackend(redisState *redisRuntime) (Config, bool, storageBackend, error) {
	if redisState != nil {
		ctx, cancel := redisState.context()
		defer cancel()
		cfg, found, err := redisState.loadConfig(ctx)
		if err != nil {
			Logger.Warn("[config] redis load failed, falling back", "error", err)
		}
		if err == nil && found {
			return cfg, false, storageBackendRedis, nil
		}

		cfg, fromEnv, backend, fallbackErr := loadConfigFromLegacySources()
		if fallbackErr == nil && backend != storageBackendMemory {
			ctx, cancel = redisState.context()
			defer cancel()
			if saveErr := redisState.saveConfig(ctx, cfg); saveErr == nil {
				return cfg, false, storageBackendRedis, nil
			} else {
				Logger.Warn("[config] redis bootstrap failed", "error", saveErr)
			}
		}
		return cfg, fromEnv, backend, fallbackErr
	}
	return loadConfigFromLegacySources()
}

func loadConfigFromLegacySources() (Config, bool, storageBackend, error) {
	rawCfg := strings.TrimSpace(os.Getenv("DS2API_CONFIG_JSON"))
	if rawCfg != "" {
		cfg, err := parseConfigString(rawCfg)
		if err != nil {
			if !IsVercel() && envWritebackEnabled() {
				if fileCfg, fileErr := loadConfigFromFile(ConfigPath()); fileErr == nil {
					return fileCfg, false, storageBackendFile, nil
				}
			}
			return cfg, true, storageBackendEnv, err
		}
		cfg.ClearAccountTokens()
		cfg.DropInvalidAccounts()
		if IsVercel() || !envWritebackEnabled() {
			return cfg, true, storageBackendEnv, err
		}
		content, fileErr := os.ReadFile(ConfigPath())
		if fileErr == nil {
			var fileCfg Config
			if unmarshalErr := json.Unmarshal(content, &fileCfg); unmarshalErr == nil {
				fileCfg.DropInvalidAccounts()
				return fileCfg, false, storageBackendFile, err
			}
		}
		if errors.Is(fileErr, os.ErrNotExist) {
			if validateErr := ValidateConfig(cfg); validateErr != nil {
				return cfg, true, storageBackendEnv, validateErr
			}
			writeErr := writeConfigFile(ConfigPath(), cfg.Clone())
			if writeErr == nil {
				return cfg, false, storageBackendFile, err
			}
			Logger.Warn("[config] env writeback bootstrap failed", "error", writeErr)
		}
		return cfg, true, storageBackendEnv, err
	}

	cfg, err := loadConfigFromFile(ConfigPath())
	if err != nil {
		if IsVercel() {
			// Vercel one-click deploy may start without a writable/present config file.
			// Keep an in-memory config so users can bootstrap via WebUI.
			return Config{}, true, storageBackendMemory, nil
		}
		return Config{}, false, storageBackendFile, err
	}
	if IsVercel() {
		// Vercel filesystem is ephemeral/read-only for runtime writes; treat the
		// loaded file as a bootstrap snapshot and keep runtime changes in memory.
		return cfg, true, storageBackendMemory, nil
	}
	return cfg, false, storageBackendFile, nil
}

func loadConfigFromFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}
	cfg.DropInvalidAccounts()
	if strings.Contains(string(content), `"test_status"`) && !IsVercel() {
		if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.WriteFile(path, b, 0o644)
		}
	}
	return cfg, nil
}

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

func (s *Store) HasAPIKey(k string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.keyMap[k]
	return ok
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Keys)
}

func (s *Store) Accounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Accounts)
}

func (s *Store) FindAccount(identifier string) (Account, bool) {
	identifier = strings.TrimSpace(identifier)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.findAccountIndexLocked(identifier); ok {
		return s.cfg.Accounts[idx], true
	}
	return Account{}, false
}

func (s *Store) UpdateAccountTestStatus(identifier, status string) error {
	identifier = strings.TrimSpace(identifier)
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.findAccountIndexLocked(identifier)
	if !ok {
		return errors.New("account not found")
	}
	s.setAccountTestStatusLocked(s.cfg.Accounts[idx], status, identifier)
	return nil
}

func (s *Store) AccountTestStatus(identifier string) (string, bool) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.accTest[identifier]
	return status, ok
}

func (s *Store) UpdateAccountToken(identifier, token string) error {
	identifier = strings.TrimSpace(identifier)
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.findAccountIndexLocked(identifier)
	if !ok {
		return errors.New("account not found")
	}
	oldID := s.cfg.Accounts[idx].Identifier()
	s.cfg.Accounts[idx].Token = token
	newID := s.cfg.Accounts[idx].Identifier()
	if identifier != "" {
		s.accMap[identifier] = idx
	}
	if oldID != "" {
		s.accMap[oldID] = idx
	}
	if newID != "" {
		s.accMap[newID] = idx
	}
	return s.saveLocked()
}

func (s *Store) Replace(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg.Clone()
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Update(mutator func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Clone()
	if err := mutator(&cfg); err != nil {
		return err
	}
	s.cfg = cfg
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.redis != nil {
		if err := s.saveRedisLocked(); err != nil {
			return err
		}
		s.fromEnv = false
		s.backend = storageBackendRedis
		return nil
	}
	if s.fromEnv && (IsVercel() || !envWritebackEnabled()) {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	persistCfg := s.cfg.Clone()
	persistCfg.ClearAccountTokens()
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigBytes(s.path, b); err != nil {
		return err
	}
	s.fromEnv = false
	s.backend = storageBackendFile
	return nil
}

func (s *Store) saveRedisLocked() error {
	ctx, cancel := s.redis.context()
	defer cancel()
	return s.redis.saveConfig(ctx, s.cfg)
}

func (s *Store) IsEnvBacked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fromEnv
}

func (s *Store) StorageBackend() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return string(s.backend)
}

func (s *Store) IsPersistentStorage() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backend == storageBackendFile || s.backend == storageBackendRedis
}

func (s *Store) IsRedisConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.redis != nil
}

func (s *Store) RedisConfigKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.redis == nil {
		return ""
	}
	return s.redis.configKey
}

func (s *Store) SetVercelSync(hash string, ts int64) error {
	return s.Update(func(c *Config) error {
		c.VercelSyncHash = hash
		c.VercelSyncTime = ts
		return nil
	})
}

func (s *Store) ExportJSONAndBase64() (string, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exportCfg := s.cfg.Clone()
	exportCfg.ClearAccountTokens()
	b, err := json.Marshal(exportCfg)
	if err != nil {
		return "", "", err
	}
	return string(b), base64.StdEncoding.EncodeToString(b), nil
}

func (s *Store) RecordCall(success bool) {
	if s.recordCallToRedis(success) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics.TotalCalls++
	if success {
		s.metrics.SuccessCalls++
	} else {
		s.metrics.FailedCalls++
	}
	s.metrics.LastUpdatedAt = time.Now().Unix()
}

func (s *Store) recordCallToRedis(success bool) bool {
	s.mu.RLock()
	redisState := s.redis
	s.mu.RUnlock()
	if redisState == nil {
		return false
	}
	ctx, cancel := redisState.context()
	defer cancel()
	if err := redisState.recordCall(ctx, success); err != nil {
		Logger.Warn("[redis] record call failed", "error", err)
		return false
	}
	return true
}

func (s *Store) CallMetrics() CallMetrics {
	s.mu.RLock()
	redisState := s.redis
	metrics := s.metrics
	s.mu.RUnlock()
	if redisState == nil {
		return metrics
	}
	ctx, cancel := redisState.context()
	defer cancel()
	loaded, err := redisState.loadCallMetrics(ctx)
	if err != nil {
		Logger.Warn("[redis] load call metrics failed", "error", err)
		return metrics
	}
	return loaded
}

func (s *Store) AcquireAutoDeleteAllLease(accountID string) bool {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return true
	}
	ttl := autoDeleteAllLeaseTTL()
	if ttl <= 0 {
		return true
	}

	s.mu.RLock()
	redisState := s.redis
	s.mu.RUnlock()
	if redisState != nil {
		ctx, cancel := redisState.context()
		defer cancel()
		ok, err := redisState.acquireAutoDeleteLease(ctx, accountID, ttl)
		if err != nil {
			Logger.Warn("[redis] acquire auto-delete lease failed", "account", accountID, "error", err)
			return true
		}
		return ok
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leaseMap == nil {
		s.leaseMap = map[string]time.Time{}
	}
	if until, ok := s.leaseMap[accountID]; ok && now.Before(until) {
		return false
	}
	s.leaseMap[accountID] = now.Add(ttl)
	for key, until := range s.leaseMap {
		if now.After(until) {
			delete(s.leaseMap, key)
		}
	}
	return true
}

func autoDeleteAllLeaseTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DS2API_VERCEL_SESSION_DELETE_LEASE_TTL_SECONDS"))
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	if IsVercel() {
		return defaultVercelDeleteLeaseTTL
	}
	return 0
}

func (s *Store) SaveConfigToRedis(ctx context.Context, rawJSON, redisURL, key string) error {
	s.mu.RLock()
	redisState := s.redis
	s.mu.RUnlock()
	return saveConfigJSONToRedis(ctx, rawJSON, redisURL, key, redisState)
}

func saveConfigJSONToRedis(ctx context.Context, rawJSON, redisURL, key string, fallback *redisRuntime) error {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return errors.New("config json is empty")
	}
	var cfg Config
	if err := json.Unmarshal([]byte(rawJSON), &cfg); err != nil {
		return err
	}
	cfg.DropInvalidAccounts()
	cfg.ClearAccountTokens()

	redisState := fallback
	if strings.TrimSpace(redisURL) != "" {
		redisState = newRedisRuntime(redisURL, key)
	} else if key = strings.TrimSpace(key); key != "" {
		redisState = newRedisRuntime(firstNonEmptyEnv(
			"DS2API_REDIS_URL",
			"KV_URL",
			"REDIS_URL",
			"UPSTASH_REDIS_URL",
		), key)
	}
	if redisState == nil {
		return errors.New("redis is not configured")
	}
	return redisState.saveConfig(ctx, cfg)
}
