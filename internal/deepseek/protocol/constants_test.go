package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSharedConstantsLoaded(t *testing.T) {
	cfg := sharedConstants{}
	if err := json.Unmarshal(sharedConstantsJSON, &cfg); err != nil {
		t.Fatalf("failed to parse shared constants: %v", err)
	}
	client := normalizeClientConstants(cfg.Client)
	if ClientVersion != client.Version {
		t.Fatalf("unexpected client version=%q", ClientVersion)
	}
	// User-Agent now contains randomized version, check format instead of exact match
	if !strings.HasPrefix(BaseHeaders["User-Agent"], client.Name+"/") {
		t.Fatalf("unexpected user agent prefix=%q", BaseHeaders["User-Agent"])
	}
	if !strings.Contains(BaseHeaders["User-Agent"], " Android/") {
		t.Fatalf("unexpected user agent missing Android=%q", BaseHeaders["User-Agent"])
	}
	if BaseHeaders["x-client-platform"] != "android" {
		t.Fatalf("unexpected base header x-client-platform=%q", BaseHeaders["x-client-platform"])
	}
	// x-client-version is now randomized, just check it exists and has valid format
	if BaseHeaders["x-client-version"] == "" {
		t.Fatal("expected base header x-client-version to be set")
	}
	if !strings.HasPrefix(BaseHeaders["x-client-version"], "2.0.") {
		t.Fatalf("unexpected base header x-client-version=%q", BaseHeaders["x-client-version"])
	}
	if BaseHeaders["Content-Type"] != "application/json" {
		t.Fatalf("unexpected base header Content-Type=%q", BaseHeaders["Content-Type"])
	}
	if len(SkipContainsPatterns) == 0 {
		t.Fatal("expected skip contains patterns to be loaded")
	}
	if _, ok := SkipExactPathSet["response/search_status"]; !ok {
		t.Fatal("expected response/search_status in exact skip path set")
	}
}

func TestClientHeadersDerivedFromSharedVersion(t *testing.T) {
	client := normalizeClientConstants(clientConstants{
		Name:            "DeepSeek",
		Platform:        "android",
		Version:         "9.8.7",
		AndroidAPILevel: "35",
		Locale:          "zh_CN",
	})
	headers := buildBaseHeaders(client, map[string]string{
		"User-Agent":       "stale",
		"x-client-version": "stale",
	})
	// User-Agent now contains randomized version
	if !strings.HasPrefix(headers["User-Agent"], "DeepSeek/") {
		t.Fatalf("unexpected derived user agent prefix=%q", headers["User-Agent"])
	}
	if !strings.Contains(headers["User-Agent"], " Android/") {
		t.Fatalf("unexpected derived user agent missing Android=%q", headers["User-Agent"])
	}
	// x-client-version is randomized, check format
	if !strings.HasPrefix(headers["x-client-version"], "9.8.") {
		t.Fatalf("unexpected derived client version=%q", headers["x-client-version"])
	}
}
