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
	wantUserAgent := client.Name + "/" + client.Version + " Android/" + client.AndroidAPILevel
	if BaseHeaders["User-Agent"] != wantUserAgent {
		t.Fatalf("unexpected user agent=%q", BaseHeaders["User-Agent"])
	}
	if BaseHeaders["x-client-platform"] != "android" {
		t.Fatalf("unexpected base header x-client-platform=%q", BaseHeaders["x-client-platform"])
	}
	if BaseHeaders["x-client-version"] != ClientVersion {
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
	if headers["User-Agent"] != "DeepSeek/9.8.7 Android/35" {
		t.Fatalf("unexpected derived user agent=%q", headers["User-Agent"])
	}
	if headers["x-client-version"] != "9.8.7" {
		t.Fatalf("unexpected derived client version=%q", headers["x-client-version"])
	}
}
