package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func envWritebackEnabled() bool {
	return false
}

func (s *Store) IsEnvWritebackEnabled() bool {
	return envWritebackEnabled()
}

func (s *Store) HasEnvConfigSource() bool {
	return false
}

func (s *Store) ConfigPath() string {
	return s.path
}

func writeConfigFile(path string, cfg Config) error {
	persistCfg := cfg.Clone()
	persistCfg.ClearAccountTokens()
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigBytes(path, b)
}

func writeConfigBytes(path string, b []byte) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return os.WriteFile(path, b, 0o644)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	return os.WriteFile(path, b, 0o644)
}
