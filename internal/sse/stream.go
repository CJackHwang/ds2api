package sse

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"time"
)

const (
	parsedLineBufferSize = 128
	scannerBufferSize    = 64 * 1024
	maxScannerLineSize   = 2 * 1024 * 1024
)

type AccumulateConfig struct {
	Enabled        bool          // Enable token accumulation
	MinChars       int           // Minimum characters before non-timer flush (default: 80)
	MaxWait        time.Duration // Maximum time to wait before flushing accumulated text (default: 10ms)
	FlushOnFinish  bool          // Force flush when stream finishes
	WordBoundary   bool          // Only flush at word boundaries (spaces, punctuation, newlines)
	FlushOnNewline bool          // Flush immediately when newline is detected in content
}

var productionAccumulate = AccumulateConfig{
	Enabled:        true,
	MinChars:       80,
	MaxWait:        10 * time.Millisecond,
	FlushOnFinish:  true,
	WordBoundary:   false,
	FlushOnNewline: true,
}

var testAccumulate = AccumulateConfig{
	Enabled: false,
}

func DefaultAccumulateConfig() AccumulateConfig {
	if strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "___test") {
		return testAccumulate
	}
	return productionAccumulate
}

func StartParsedLinePump(ctx context.Context, body io.Reader, thinkingEnabled bool, initialType string) (<-chan LineResult, <-chan error) {
	return startParsedLinePumpWithConfig(ctx, body, thinkingEnabled, initialType, DefaultAccumulateConfig())
}

func startParsedLinePumpWithConfig(ctx context.Context, body io.Reader, thinkingEnabled bool, initialType string, cfg AccumulateConfig) (<-chan LineResult, <-chan error) {
	out := make(chan LineResult, parsedLineBufferSize)
	done := make(chan error, 1)

	go func() {
		defer close(out)
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, scannerBufferSize), maxScannerLineSize)
		currentType := initialType

		var pumpErr error

		var textBuffer strings.Builder
		var thinkingBuffer strings.Builder
		var toolDetectionThinkingBuffer strings.Builder
		var textPendingType string
		var thinkingPendingType string

		var maxWaitTimer *time.Timer
		var maxWaitCh <-chan time.Time
		if cfg.Enabled && cfg.MaxWait > 0 {
			maxWaitTimer = time.NewTimer(cfg.MaxWait)
			maxWaitCh = maxWaitTimer.C
		}
		defer func() {
			if maxWaitTimer != nil {
				maxWaitTimer.Stop()
			}
		}()

		var resetMaxWait func()
		resetMaxWait = func() {
			if maxWaitTimer == nil {
				return
			}
			if !maxWaitTimer.Stop() {
				select {
				case <-maxWaitTimer.C:
				default:
				}
			}
			maxWaitTimer.Reset(cfg.MaxWait)
		}

		shouldFlushImmediate := func(text string) bool {
			if cfg.FlushOnNewline && strings.ContainsAny(text, "\n\r") {
				return true
			}
			return false
		}

		var flushBuffer func(force bool)
		flushBuffer = func(force bool) {
			if !cfg.Enabled {
				return
			}

			textLen := textBuffer.Len()
			thinkingLen := thinkingBuffer.Len()

			shouldFlush := force ||
				textLen >= cfg.MinChars ||
				(thinkingLen > 0 && textLen >= 50)

			if !shouldFlush {
				return
			}

			var parts []ContentPart

			if thinkingLen > 0 {
				parts = append(parts, ContentPart{Text: thinkingBuffer.String(), Type: thinkingPendingType})
				thinkingBuffer.Reset()
			}

			if textLen > 0 {
				parts = append(parts, ContentPart{Text: textBuffer.String(), Type: textPendingType})
				textBuffer.Reset()
			}

			if len(parts) > 0 || toolDetectionThinkingBuffer.Len() > 0 {
				var detectionParts []ContentPart
				if toolDetectionThinkingBuffer.Len() > 0 {
					detectionParts = append(detectionParts, ContentPart{Text: toolDetectionThinkingBuffer.String(), Type: "thinking"})
					toolDetectionThinkingBuffer.Reset()
				}
				result := LineResult{
					Parsed:                     true,
					Stop:                       false,
					Parts:                      parts,
					ToolDetectionThinkingParts: detectionParts,
					NextType:                   currentType,
				}
				select {
				case out <- result:
				case <-ctx.Done():
					pumpErr = ctx.Err()
					return
				}
			}

			resetMaxWait()
		}

		for scanner.Scan() {
			result := ParseDeepSeekContentLine(scanner.Bytes(), thinkingEnabled, currentType)
			currentType = result.NextType

			if result.Stop {
				if cfg.Enabled && cfg.FlushOnFinish {
					for _, p := range result.ToolDetectionThinkingParts {
						toolDetectionThinkingBuffer.WriteString(p.Text)
					}
					if textBuffer.Len() > 0 || len(result.Parts) > 0 || toolDetectionThinkingBuffer.Len() > 0 {
						for _, p := range result.Parts {
							if p.Type == "thinking" {
								thinkingBuffer.WriteString(p.Text)
								thinkingPendingType = "thinking"
							} else {
								textBuffer.WriteString(p.Text)
								textPendingType = p.Type
							}
						}
						flushBuffer(true)
					}
				}
				if result.ErrorMessage != "" || result.ContentFilter {
					select {
					case out <- result:
					case <-ctx.Done():
						pumpErr = ctx.Err()
						return
					}
				} else {
					stopResult := LineResult{
						Parsed:   true,
						Stop:     true,
						NextType: currentType,
					}
					select {
					case out <- stopResult:
					case <-ctx.Done():
						pumpErr = ctx.Err()
						return
					}
				}
				continue
			}

			if !result.Parsed {
				continue
			}

			if cfg.Enabled {
				for _, p := range result.ToolDetectionThinkingParts {
					toolDetectionThinkingBuffer.WriteString(p.Text)
				}
				for _, p := range result.Parts {
					if p.Type == "thinking" {
						if textBuffer.Len() > 0 {
							flushBuffer(true)
						}
						thinkingBuffer.WriteString(p.Text)
						thinkingPendingType = "thinking"
					} else {
						textBuffer.WriteString(p.Text)
						textPendingType = p.Type
						if shouldFlushImmediate(p.Text) {
							flushBuffer(true)
						}
					}
				}

				if textBuffer.Len() >= cfg.MinChars {
					flushBuffer(false)
				}
				select {
				case <-maxWaitCh:
					if textBuffer.Len() > 0 || thinkingBuffer.Len() > 0 || toolDetectionThinkingBuffer.Len() > 0 {
						flushBuffer(true)
					}
					resetMaxWait()
				default:
				}
			} else {
				select {
				case out <- result:
				case <-ctx.Done():
					pumpErr = ctx.Err()
					return
				}
			}
		}

		if cfg.Enabled {
			flushBuffer(true)
		}

		if pumpErr != nil {
			done <- pumpErr
		} else {
			done <- scanner.Err()
		}
	}()
	return out, done
}
