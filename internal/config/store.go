package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
)

type Store struct {
	mu      sync.RWMutex
	cfg     Config
	path    string
	fromEnv bool
	keyMap  map[string]struct{} // O(1) API key lookup index
	accMap  map[string]int      // O(1) account lookup: identifier -> slice index
	accTest map[string]string   // runtime-only account test status cache
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
	cfg, fromEnv, err := loadConfig()
	cfg.Normalize()
	if validateErr := ValidateConfig(cfg); validateErr != nil {
		err = errors.Join(err, validateErr)
	}
	return &Store{cfg: cfg, path: ConfigPath(), fromEnv: fromEnv}, err
}

func loadConfig() (Config, bool, error) {
	rawCfg := strings.TrimSpace(os.Getenv("DS2API_CONFIG_JSON"))
	if rawCfg != "" {
		cfg, err := parseConfigString(rawCfg)
		if err != nil {
			if !IsVercel() && envWritebackEnabled() {
				if fileCfg, fileErr := loadConfigFromFile(ConfigPath()); fileErr == nil {
					return fileCfg, false, nil
				}
			}
			return cfg, true, err
		}
		cfg.ClearAccountTokens()
		cfg.Normalize()
		cfg.DropInvalidAccounts()
		if IsVercel() || !envWritebackEnabled() {
			return cfg, true, err
		}
		content, fileErr := os.ReadFile(ConfigPath())
		if fileErr == nil {
			var fileCfg Config
			if unmarshalErr := json.Unmarshal(content, &fileCfg); unmarshalErr == nil {
				fileCfg.Normalize()
				fileCfg.DropInvalidAccounts()
				return fileCfg, false, err
			}
		}
		if errors.Is(fileErr, os.ErrNotExist) {
			if validateErr := ValidateConfig(cfg); validateErr != nil {
				return cfg, true, validateErr
			}
			if writeErr := writeConfigFile(ConfigPath(), cfg.Clone()); writeErr == nil {
				return cfg, false, err
			} else {
				Logger.Warn("[config] env writeback bootstrap failed", "error", writeErr)
			}
		}
		return cfg, true, err
	}

	cfg, err := loadConfigFromFile(ConfigPath())
	if err != nil {
		if IsVercel() {
			// Vercel one-click deploy may start without a writable/present config file.
			// Keep an in-memory config so users can bootstrap via WebUI then sync env.
			return Config{}, true, nil
		}
		return Config{}, false, err
	}
	if IsVercel() {
		// Vercel filesystem is ephemeral/read-only for runtime writes; avoid save errors.
		return cfg, true, nil
	}
	return cfg, false, nil
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
	cfg.Normalize()
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
	// Keep historical aliases usable for long-lived queues while also adding
	// the latest identifier after token refresh.
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
	cfg.Normalize()
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
	cfg.Normalize()
	s.cfg = cfg
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return nil
}

func (s *Store) saveLocked() error {
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
	return nil
}

func (s *Store) IsEnvBacked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fromEnv
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
