package config

import (
	"os"
	"path/filepath"
	"strings"
)

func BaseDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func IsVercel() bool {
	return strings.TrimSpace(os.Getenv("VERCEL")) != "" || strings.TrimSpace(os.Getenv("NOW_REGION")) != ""
}

func ResolvePath(envKey, defaultRel string) string {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw != "" {
		if filepath.IsAbs(raw) {
			return raw
		}
		return filepath.Join(BaseDir(), raw)
	}
	return filepath.Join(BaseDir(), defaultRel)
}

func ConfigPath() string {
	if strings.TrimSpace(os.Getenv("DS2API_CONFIG_PATH")) == "" && BaseDir() == "/app" {
		return containerDefaultConfigPath()
	}
	return ResolvePath("DS2API_CONFIG_PATH", "config.json")
}

func containerDefaultConfigPath() string {
	// Container images run as non-root by default. Only use /data when mounted/provisioned.
	// Otherwise keep /app/config.json so admin-side save does not fail on MkdirAll("/data").
	if st, err := os.Stat("/data"); err == nil && st.IsDir() {
		return "/data/config.json"
	}
	return "/app/config.json"
}

func legacyContainerConfigPath() string {
	return "/app/config.json"
}

func shouldTryLegacyContainerConfigPath() bool {
	return strings.TrimSpace(os.Getenv("DS2API_CONFIG_PATH")) == "" && BaseDir() == "/app"
}

func RawStreamSampleRoot() string {
	return ResolvePath("DS2API_RAW_STREAM_SAMPLE_ROOT", "tests/raw_stream_samples")
}

func ChatHistoryPath() string {
	// On Vercel, /var/task is read-only at runtime. If no explicit path is set,
	// default to /tmp/chat_history.json (the only writable directory).
	if IsVercel() && strings.TrimSpace(os.Getenv("DS2API_CHAT_HISTORY_PATH")) == "" {
		return "/tmp/chat_history.json"
	}
	return ResolvePath("DS2API_CHAT_HISTORY_PATH", "data/chat_history.json")
}

func StaticAdminDir() string {
	return ResolvePath("DS2API_STATIC_ADMIN_DIR", "static/admin")
}
