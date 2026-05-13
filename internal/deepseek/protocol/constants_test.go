package protocol

import (
	"encoding/json"
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
	if BaseHeaders["x-client-platform"] != "web" {
		t.Fatalf("unexpected base header x-client-platform=%q", BaseHeaders["x-client-platform"])
	}
	if BaseHeaders["x-client-version"] != "2.0.0" {
		t.Fatalf("unexpected base header x-client-version=%q", BaseHeaders["x-client-version"])
	}
	if BaseHeaders["Content-Type"] != "application/json" {
		t.Fatalf("unexpected base header Content-Type=%q", BaseHeaders["Content-Type"])
	}
	if BaseHeaders["origin"] != "https://chat.deepseek.com" {
		t.Fatalf("unexpected base header origin=%q", BaseHeaders["origin"])
	}
	if BaseHeaders["x-app-version"] != "2.0.0" {
		t.Fatalf("unexpected base header x-app-version=%q", BaseHeaders["x-app-version"])
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
		Name:     "DeepSeek",
		Platform: "web",
		Version:  "2.0.0",
		Locale:   "zh_CN",
	})
	headers := buildBaseHeaders(client, map[string]string{
		"x-client-version": "stale",
	})
	if headers["x-client-version"] != "2.0.0" {
		t.Fatalf("unexpected derived client version=%q", headers["x-client-version"])
	}
	if headers["x-client-platform"] != "web" {
		t.Fatalf("unexpected derived client platform=%q", headers["x-client-platform"])
	}
}
