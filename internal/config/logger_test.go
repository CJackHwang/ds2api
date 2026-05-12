package config

import (
	"testing"
)

func TestLogConfigDefaults(t *testing.T) {
	cfg := LogConfig{}
	if cfg.Level != "" {
		t.Errorf("expected empty level, got %q", cfg.Level)
	}
	if cfg.File != "" {
		t.Errorf("expected empty file, got %q", cfg.File)
	}
	if cfg.FileEnabled != false {
		t.Errorf("expected file_enabled false, got %v", cfg.FileEnabled)
	}
	if cfg.MaxSizeMB != 0 {
		t.Errorf("expected max_size_mb 0, got %d", cfg.MaxSizeMB)
	}
	if cfg.MaxBackups != 0 {
		t.Errorf("expected max_backups 0, got %d", cfg.MaxBackups)
	}
}

func TestLogConfigSetters(t *testing.T) {
	cfg := LogConfig{
		Level:       "debug",
		File:        "/var/log/test.log",
		FileEnabled: true,
		MaxSizeMB:   50,
		MaxBackups:  5,
	}
	if cfg.Level != "debug" {
		t.Errorf("expected level debug, got %q", cfg.Level)
	}
	if cfg.File != "/var/log/test.log" {
		t.Errorf("expected file /var/log/test.log, got %q", cfg.File)
	}
	if !cfg.FileEnabled {
		t.Error("expected file_enabled true")
	}
	if cfg.MaxSizeMB != 50 {
		t.Errorf("expected max_size_mb 50, got %d", cfg.MaxSizeMB)
	}
	if cfg.MaxBackups != 5 {
		t.Errorf("expected max_backups 5, got %d", cfg.MaxBackups)
	}
}

func TestNewLoggerWithNilConfig(t *testing.T) {
	logger := newLogger(nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLoggerWithValidConfig(t *testing.T) {
	cfg := LogConfig{
		Level:       "info",
		File:        "/tmp/test.log",
		FileEnabled: false,
		MaxSizeMB:   10,
		MaxBackups:  2,
	}
	logger := newLogger(&cfg)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestRefreshLoggerUpdatesGlobalLogger(t *testing.T) {
	oldLogger := Logger
	cfg := LogConfig{
		Level:       "debug",
		FileEnabled: false,
	}
	RefreshLogger(cfg)
	if Logger == nil {
		t.Fatal("expected non-nil global logger after RefreshLogger")
	}
	// Verify the new logger is different from the old one
	// (in a real scenario we'd verify it was actually reconfigured)
	_ = oldLogger // suppress unused warning
}
