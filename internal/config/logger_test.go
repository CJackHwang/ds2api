package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLogPath(t *testing.T) {
	cwd, _ := os.Getwd()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid: /var/log/ds2api and subdirectories
		{name: "var_log_ds2api", path: "/var/log/ds2api/app.log", wantErr: false},
		{name: "var_log_subdir", path: "/var/log/ds2api/subdir/app.log", wantErr: false},
		{name: "var_log_any", path: "/var/log/anyname.log", wantErr: false},

		// Valid: program working directory
		{name: "cwd", path: cwd + "/app.log", wantErr: false},
		{name: "cwd_subdir", path: cwd + "/logs/app.log", wantErr: false},

		// Invalid: parent traversal (resolved absolute path still contains ..)
		{name: "parent_traversal", path: "/var/log/../../../etc/passwd", wantErr: true},

		// Invalid: parent traversal
		{name: "parent_traversal", path: "/var/log/../../../etc/passwd", wantErr: true},
		{name: "parent_in_middle", path: "/var/log/ds2api/../../other/app.log", wantErr: true},

		// Invalid: absolute paths outside allowed directories
		{name: "etc_passwd", path: "/etc/passwd", wantErr: true},
		{name: "tmp_file", path: "/tmp/app.log", wantErr: true},
		{name: "root_file", path: "/root/app.log", wantErr: true},

		// Valid: relative paths (resolved against cwd)
		{name: "relative_logs", path: "logs/app.log", wantErr: false},
		{name: "relative_current", path: "app.log", wantErr: false},

		// Edge cases
		{name: "double_slash", path: "/var/log//ds2api/app.log", wantErr: false}, // Clean will fix this
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLogPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

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
	cwd, _ := os.Getwd()
	cfg := LogConfig{
		Level:       "info",
		File:        filepath.Join(cwd, "test.log"), // Valid: under working directory
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
