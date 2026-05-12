package config

import (
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger = newLogger(nil)

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

	if cfg != nil && cfg.FileEnabled && cfg.File != "" {
		maxSize := cfg.MaxSizeMB
		if maxSize <= 0 {
			maxSize = 100
		}
		maxBackups := cfg.MaxBackups
		if maxBackups <= 0 {
			maxBackups = 3
		}
		fileHandler := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			Compress:   true,
		}
		handlers = append(handlers, slog.NewTextHandler(fileHandler, &slog.HandlerOptions{Level: level}))
	}

	return slog.New(slog.NewMultiHandler(handlers...))
}

func RefreshLogger(cfg LogConfig) {
	Logger = newLogger(&cfg)
}
