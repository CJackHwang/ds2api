package sse

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestStartParsedLinePumpEmptyBody(t *testing.T) {
	body := strings.NewReader("")
	results, done := StartParsedLinePump(context.Background(), body, false, "text")

	collected := make([]LineResult, 0)
	for r := range results {
		collected = append(collected, r)
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collected) != 0 {
		t.Fatalf("expected no results for empty body, got %d", len(collected))
	}
}

func TestStartParsedLinePumpMultipleLines(t *testing.T) {
	body := strings.NewReader(
		"data: {\"p\":\"response/thinking_content\",\"v\":\"think\"}\n" +
			"data: {\"p\":\"response/content\",\"v\":\"text\"}\n" +
			"data: [DONE]\n",
	)
	cfg := AccumulateConfig{Enabled: false}
	results, done := startParsedLinePumpWithConfig(context.Background(), body, true, "thinking", cfg)

	collected := make([]LineResult, 0)
	for r := range results {
		collected = append(collected, r)
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collected) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(collected))
	}
	// First should be thinking
	if collected[0].Parts[0].Type != "thinking" {
		t.Fatalf("expected first part thinking, got %q", collected[0].Parts[0].Type)
	}
	// Last should be stop
	last := collected[len(collected)-1]
	if !last.Stop {
		t.Fatal("expected last result to be stop")
	}
}

func TestStartParsedLinePumpTypeTracking(t *testing.T) {
	body := strings.NewReader(
		"data: {\"p\":\"response/fragments\",\"o\":\"APPEND\",\"v\":[{\"type\":\"THINK\",\"content\":\"思\"}]}\n" +
			"data: {\"p\":\"response/fragments/-1/content\",\"v\":\"考\"}\n" +
			"data: {\"p\":\"response/fragments\",\"o\":\"APPEND\",\"v\":[{\"type\":\"RESPONSE\",\"content\":\"答\"}]}\n" +
			"data: {\"p\":\"response/fragments/-1/content\",\"v\":\"案\"}\n" +
			"data: [DONE]\n",
	)
	cfg := AccumulateConfig{Enabled: false}
	results, done := startParsedLinePumpWithConfig(context.Background(), body, true, "text", cfg)

	types := make([]string, 0)
	for r := range results {
		for _, p := range r.Parts {
			types = append(types, p.Type)
		}
	}
	<-done

	// Should have: thinking, thinking, text, text
	expected := []string{"thinking", "thinking", "text", "text"}
	if len(types) != len(expected) {
		t.Fatalf("expected types %v, got %v", expected, types)
	}
	for i, want := range expected {
		if types[i] != want {
			t.Fatalf("type[%d] mismatch: want %q got %q (all=%v)", i, want, types[i], types)
		}
	}
}

func TestStartParsedLinePumpContextCancellation(t *testing.T) {
	pr, pw := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	results, done := StartParsedLinePump(ctx, pr, false, "text")

	// Write one line and close pipe to unblock scanner
	go func() {
		_, _ = io.WriteString(pw, "data: {\"p\":\"response/content\",\"v\":\"hello\"}\n")
		_ = pw.Close()
	}()

	// Read first result
	r := <-results
	if !r.Parsed || len(r.Parts) == 0 {
		t.Fatalf("expected first parsed result, got %#v", r)
	}

	// Cancel context
	cancel()

	// Drain remaining results
	for range results {
	}

	err := <-done
	// Error may be context.Canceled, io.EOF, or nil
	if err != nil && err != context.Canceled && err != io.EOF {
		t.Fatalf("expected context.Canceled, io.EOF or nil, got %v", err)
	}
}

func TestStartParsedLinePumpOnlyDONE(t *testing.T) {
	body := strings.NewReader("data: [DONE]\n")
	results, done := StartParsedLinePump(context.Background(), body, false, "text")

	collected := make([]LineResult, 0)
	for r := range results {
		collected = append(collected, r)
	}
	<-done

	if len(collected) != 1 {
		t.Fatalf("expected 1 result, got %d", len(collected))
	}
	if !collected[0].Stop {
		t.Fatal("expected stop on [DONE]")
	}
}

func TestStartParsedLinePumpNonSSELines(t *testing.T) {
	body := strings.NewReader(
		"event: update\n" +
			": comment line\n" +
			"data: {\"p\":\"response/content\",\"v\":\"valid\"}\n" +
			"data: [DONE]\n",
	)
	results, done := StartParsedLinePump(context.Background(), body, false, "text")

	var validCount int
	for r := range results {
		if r.Parsed && len(r.Parts) > 0 {
			validCount++
		}
	}
	<-done

	if validCount != 1 {
		t.Fatalf("expected 1 valid result, got %d", validCount)
	}
}

func TestStartParsedLinePumpThinkingDisabled(t *testing.T) {
	body := strings.NewReader(
		"data: {\"p\":\"response/thinking_content\",\"v\":\"thought\"}\n" +
			"data: {\"p\":\"response/content\",\"v\":\"response\"}\n" +
			"data: [DONE]\n",
	)
	// With thinking disabled, thinking content should still be emitted but marked differently
	results, done := StartParsedLinePump(context.Background(), body, false, "text")

	var parts []ContentPart
	for r := range results {
		parts = append(parts, r.Parts...)
	}
	<-done

	if len(parts) < 1 {
		t.Fatalf("expected at least 1 part, got %d", len(parts))
	}
}
