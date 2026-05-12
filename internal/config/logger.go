package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger = newLogger(nil)

// validateLogPath checks if the log file path is within allowed directories.
// Allowed paths:
//   - /var/log/ds2api (or any /var/log/* subdirectory)
//   - The program's current working directory or its subdirectories
//   - Relative paths (resolved against current working directory)
func validateLogPath(path string) error {
	// Handle relative paths by resolving against current directory
	resolvedPath := path
	if !filepath.IsAbs(path) {
		var err error
		resolvedPath, err = filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("cannot resolve relative path: %w", err)
		}
	}

	cleaned := filepath.Clean(resolvedPath)

	// Block parent directory traversal
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("log path cannot contain parent directory reference")
	}

	// Allowed: /var/log/ds2api or /var/log/*
	if strings.HasPrefix(cleaned, "/var/log/") {
		return nil
	}

	// Allowed: program's working directory and subdirectories
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}
	cwd = filepath.Clean(cwd)
	if pathWithinDir(cleaned, cwd) {
		return nil
	}

	return fmt.Errorf("log path must be under /var/log/ or the program directory (%s)", cwd)
}

func pathWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func newLogger(cfg *LogConfig) *slog.Logger {
	level := new(slog.LevelVar)
	if cfg != nil && cfg.Level != "" {
		switch strings.ToUpper(strings.TrimSpace(cfg.Level)) {
		case "DEBUG":
			level.Set(slog.LevelDebug)
		case "WARN":
			level.Set(slog.LevelWarn)
		case "ERROR":
			level.Set(slog.LevelError)
		default:
			level.Set(slog.LevelInfo)
		}
	} else {
		switch strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
		case "DEBUG":
			level.Set(slog.LevelDebug)
		case "WARN":
			level.Set(slog.LevelWarn)
		case "ERROR":
			level.Set(slog.LevelError)
		default:
			level.Set(slog.LevelInfo)
		}
	}

	handlers := []slog.Handler{slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})}

	// Apply default log file path if enabled but no path specified
	filePath := ""
	if cfg != nil && cfg.FileEnabled {
		filePath = cfg.File
		if filePath == "" {
			filePath = "logs/ds2api.log" // default path
		}
	}

	if cfg != nil && cfg.FileEnabled {
		// Validate path security
		if err := validateLogPath(filePath); err != nil {
			// Cannot use Logger here due to initialization cycle; use slog directly
			slog.Warn("log file path rejected", "path", filePath, "error", err)
			return slog.New(slog.NewMultiHandler(handlers...))
		}

		maxSize := cfg.MaxSizeMB
		if maxSize <= 0 {
			maxSize = 100
		}
		if maxSize > 1024 {
			maxSize = 1024
		}
		maxBackups := cfg.MaxBackups
		if maxBackups <= 0 {
			maxBackups = 3
		}
		fileHandler := &lumberjack.Logger{
			Filename:   filePath,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			Compress:   true,
		}
		// Test write permissions; if file is not writable, skip file handler silently
		if _, err := fileHandler.Write([]byte("")); err == nil {
			handlers = append(handlers, slog.NewTextHandler(fileHandler, &slog.HandlerOptions{Level: level}))
		}
	}

	return slog.New(slog.NewMultiHandler(handlers...))
}

func RefreshLogger(cfg LogConfig) {
	newLoggerInstance := newLogger(&cfg)
	newLoggerInstance.Info("logger reconfigured", "level", cfg.Level, "file", cfg.File, "file_enabled", cfg.FileEnabled)
	Logger = newLoggerInstance
}
