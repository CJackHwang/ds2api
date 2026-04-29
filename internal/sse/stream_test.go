package sse

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStartParsedLinePumpParsesAndStops(t *testing.T) {
	body := strings.NewReader("data: {\"p\":\"response/content\",\"v\":\"hi\"}\n\ndata: [DONE]\n")
	results, done := StartParsedLinePump(context.Background(), body, false, "text")

	collected := make([]LineResult, 0, 2)
	for r := range results {
		collected = append(collected, r)
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected scanner error: %v", err)
	}
	if len(collected) < 2 {
		t.Fatalf("expected at least 2 parsed results, got %d", len(collected))
	}
	if !collected[0].Parsed || len(collected[0].Parts) == 0 {
		t.Fatalf("expected first line to contain parsed content")
	}
	last := collected[len(collected)-1]
	if !last.Parsed || !last.Stop {
		t.Fatalf("expected last line to stop stream, got parsed=%v stop=%v", last.Parsed, last.Stop)
	}
}

func buildAccumulateSSEBody(chunks []string, minChars int) *strings.Reader {
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(fmt.Sprintf("data: {\"p\":\"response/content\",\"v\":\"%s\"}\n", c))
	}
	sb.WriteString("data: [DONE]\n")
	return strings.NewReader(sb.String())
}

func TestAccumulateFlushOnMinChars(t *testing.T) {
	cfg := AccumulateConfig{
		Enabled:       true,
		MinChars:      10,
		MaxWait:       500 * time.Millisecond,
		FlushOnFinish: true,
	}
	body := buildAccumulateSSEBody([]string{"hello", " world", "foo", "bar"}, 10)
	results, done := startParsedLinePumpWithConfig(context.Background(), body, false, "text", cfg)

	collected := make([]LineResult, 0)
	for r := range results {
		collected = append(collected, r)
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stopCount := 0
	flushCount := 0
	var totalText strings.Builder
	for _, r := range collected {
		if r.Stop {
			stopCount++
			continue
		}
		if r.Parsed && len(r.Parts) > 0 {
			flushCount++
			for _, p := range r.Parts {
				totalText.WriteString(p.Text)
			}
		}
	}

	if totalText.String() != "hello worldfoobar" {
		t.Errorf("expected total text 'hello worldfoobar', got %q", totalText.String())
	}
	if flushCount == 0 {
		t.Error("expected at least one flush")
	}
	if stopCount != 1 {
		t.Errorf("expected 1 stop result, got %d", stopCount)
	}
}

func TestAccumulateDisabled(t *testing.T) {
	cfg := AccumulateConfig{
		Enabled:       false,
		MinChars:      150,
		MaxWait:       80 * time.Millisecond,
		FlushOnFinish: true,
	}
	body := buildAccumulateSSEBody([]string{"a", "b", "c"}, 150)
	results, done := startParsedLinePumpWithConfig(context.Background(), body, false, "text", cfg)

	collected := make([]LineResult, 0)
	for r := range results {
		collected = append(collected, r)
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nonStopCount := 0
	for _, r := range collected {
		if !r.Stop && r.Parsed {
			nonStopCount++
			if len(r.Parts) != 1 || len(r.Parts[0].Text) != 1 {
				t.Errorf("expected single char per result when accumulation disabled, got parts=%v text=%q", len(r.Parts), r.Parts[0].Text)
			}
		}
	}
	if nonStopCount != 3 {
		t.Errorf("expected 3 non-stop results when disabled, got %d", nonStopCount)
	}
}

func TestFlushOnFinish(t *testing.T) {
	cfg := AccumulateConfig{
		Enabled:       true,
		MinChars:      1000,
		MaxWait:       10 * time.Second,
		FlushOnFinish: true,
	}
	body := buildAccumulateSSEBody([]string{"hello", " world"}, 1000)
	results, done := startParsedLinePumpWithConfig(context.Background(), body, false, "text", cfg)

	collected := make([]LineResult, 0)
	for r := range results {
		collected = append(collected, r)
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var totalFlushedText string
	for _, r := range collected {
		if !r.Stop && r.Parsed {
			for _, p := range r.Parts {
				totalFlushedText += p.Text
			}
		}
	}
	if totalFlushedText != "hello world" {
		t.Errorf("expected 'hello world', got %q", totalFlushedText)
	}
}

func TestContextCancellation(t *testing.T) {
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	cfg := AccumulateConfig{
		Enabled:       true,
		MinChars:      10000,
		MaxWait:       10 * time.Second,
		FlushOnFinish: true,
	}

	results, done := startParsedLinePumpWithConfig(ctx, pr, false, "text", cfg)

	go func() {
		_, _ = io.WriteString(pw, "data: {\"p\":\"response/content\",\"v\":\"hello\"}\n")
		_ = pw.Close()
	}()

	r := <-results
	if !r.Parsed {
		t.Fatalf("expected parsed result, got %#v", r)
	}

	cancel()

	for range results {
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit after context cancellation")
	}
}
