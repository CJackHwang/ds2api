package sse

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRawStreamSamplesTokenReplay(t *testing.T) {
	root := filepath.Join("..", "..", "tests", "raw_stream_samples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read samples root: %v", err)
	}

	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ssePath := filepath.Join(root, entry.Name(), "upstream.stream.sse")
		if _, err := os.Stat(ssePath); err != nil {
			continue
		}
		found++
		t.Run(entry.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(ssePath)
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}
			parsedTokens, expectedTokens, err := replayAndCollectTokens(string(raw))
			if err != nil {
				t.Fatalf("scan raw stream: %v", err)
			}
			if expectedTokens <= 0 {
				t.Fatalf("expected positive token usage from raw stream, got %d", expectedTokens)
			}
			if parsedTokens != expectedTokens {
				t.Fatalf("token mismatch parsed=%d expected=%d", parsedTokens, expectedTokens)
			}
		})
	}

	if found == 0 {
		t.Fatalf("no upstream.stream.sse samples found under %s", root)
	}
}

func replayAndCollectTokens(raw string) (parsedTokens int, expectedTokens int, scanErr error) {
	currentType := "thinking"
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" || !strings.HasPrefix(payload, "{") {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if n := rawAccumulatedTokenUsage(chunk); n > 0 {
			expectedTokens = n
		}
		res := ParseDeepSeekContentLine([]byte(line), true, currentType)
		currentType = res.NextType
		if res.OutputTokens > 0 {
			parsedTokens = res.OutputTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return parsedTokens, expectedTokens, nil
}

func rawAccumulatedTokenUsage(v any) int {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if n := rawAccumulatedTokenUsage(item); n > 0 {
				return n
			}
		}
	case map[string]any:
		if n := rawToInt(x["accumulated_token_usage"]); n > 0 {
			return n
		}
		if p, _ := x["p"].(string); strings.Contains(strings.ToLower(strings.TrimSpace(p)), "accumulated_token_usage") {
			if n := rawToInt(x["v"]); n > 0 {
				return n
			}
		}
		for _, vv := range x {
			if n := rawAccumulatedTokenUsage(vv); n > 0 {
				return n
			}
		}
	}
	return 0
}

func rawToInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
	}
	return 0
}
