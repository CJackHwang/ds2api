package sse

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
)

const (
	parsedLineBufferSize = 128
	scannerBufferSize    = 64 * 1024
	maxScannerLineSize   = 2 * 1024 * 1024
)

// AccumulateConfig defines buffering configuration for SSE output
type AccumulateConfig struct {
	Enabled       bool          // Enable token accumulation
	MinChars      int           // Minimum characters before flush (default: 150)
	MaxWait       time.Duration // Maximum wait time before flush (default: 80ms)
	FlushOnFinish bool          // Force flush when stream finishes
}

// DefaultAccumulateConfig returns sensible defaults
func DefaultAccumulateConfig() AccumulateConfig {
	return AccumulateConfig{
		Enabled:       true,
		MinChars:      150,
		MaxWait:       80 * time.Millisecond,
		FlushOnFinish: true,
	}
}

// StartParsedLinePump scans an upstream DeepSeek SSE body and emits normalized
// line parse results. It centralizes scanner setup + current fragment type
// tracking for all streaming adapters.
// NEW: Added accumulation support to reduce SSE chunk count from 500+ to ~10-20
func StartParsedLinePump(ctx context.Context, body io.Reader, thinkingEnabled bool, initialType string) (<-chan LineResult, <-chan error) {
	return startParsedLinePumpWithConfig(ctx, body, thinkingEnabled, initialType, DefaultAccumulateConfig())
}

// startParsedLinePumpWithConfig is the internal extended version with accumulation control
func startParsedLinePumpWithConfig(ctx context.Context, body io.Reader, thinkingEnabled bool, initialType string, cfg AccumulateConfig) (<-chan LineResult, <-chan error) {
	out := make(chan LineResult, parsedLineBufferSize)
	done := make(chan error, 1)

	go func() {
		defer close(out)
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, scannerBufferSize), maxScannerLineSize)
		currentType := initialType

		var pumpErr error

		// Accumulation buffer
		var textBuffer strings.Builder
		var thinkingBuffer strings.Builder
		var toolDetectionThinkingBuffer strings.Builder
		var lastFlush time.Time
		var textPendingType string
		var thinkingPendingType string

		flushBuffer := func(force bool) {
			if !cfg.Enabled {
				return
			}

			now := time.Now()
			elapsed := now.Sub(lastFlush)

			textLen := textBuffer.Len()
			thinkingLen := thinkingBuffer.Len()

			// Determine if we should flush
			shouldFlush := force ||
				textLen >= cfg.MinChars ||
				(thinkingLen > 0 && textLen >= 50) ||
				(!force && elapsed >= cfg.MaxWait && (textLen > 0 || thinkingLen > 0))

			if !shouldFlush {
				return
			}

			// Create accumulated result
			var parts []ContentPart
			if thinkingLen > 0 {
				parts = append(parts, ContentPart{Text: thinkingBuffer.String(), Type: thinkingPendingType})
			}
			if textLen > 0 {
				parts = append(parts, ContentPart{Text: textBuffer.String(), Type: textPendingType})
			}

			// Send accumulated result
			if len(parts) > 0 || toolDetectionThinkingBuffer.Len() > 0 {
				var detectionParts []ContentPart
				if toolDetectionThinkingBuffer.Len() > 0 {
					detectionParts = append(detectionParts, ContentPart{Text: toolDetectionThinkingBuffer.String(), Type: "thinking"})
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

			// Reset buffers
			textBuffer.Reset()
			thinkingBuffer.Reset()
			toolDetectionThinkingBuffer.Reset()
			lastFlush = now
		}

		for scanner.Scan() {
			result := ParseDeepSeekContentLine(scanner.Bytes(), thinkingEnabled, currentType)
			currentType = result.NextType

			// Handle stop signal - flush remaining buffer first
			if result.Stop {
				if cfg.Enabled && cfg.FlushOnFinish {
					// Accumulate tool detection thinking parts
					for _, p := range result.ToolDetectionThinkingParts {
						toolDetectionThinkingBuffer.WriteString(p.Text)
					}
					// Include any remaining content in final flush
					if textBuffer.Len() > 0 || len(result.Parts) > 0 || toolDetectionThinkingBuffer.Len() > 0 {
						// Append remaining parts to buffer before flushing
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
				// Send stop signal without Parts (already flushed above)
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
				continue
			}

			if !result.Parsed {
				continue
			}

			// Accumulate content instead of sending immediately
			if cfg.Enabled {
				for _, p := range result.ToolDetectionThinkingParts {
					toolDetectionThinkingBuffer.WriteString(p.Text)
				}
				for _, p := range result.Parts {
					if p.Type == "thinking" {
						// Thinking content after text: force-flush text first to preserve stream order
						if textBuffer.Len() > 0 {
							flushBuffer(true)
						}
						thinkingBuffer.WriteString(p.Text)
						thinkingPendingType = "thinking"
					} else {
						// Text content: accumulate
						textBuffer.WriteString(p.Text)
						textPendingType = p.Type
					}
				}

				// Check if we should flush based on content size
				if textBuffer.Len() >= cfg.MinChars {
					flushBuffer(false)
				}
			} else {
				// Original behavior - send immediately
				select {
				case out <- result:
				case <-ctx.Done():
					pumpErr = ctx.Err()
					return
				}
			}
		}

		// Final flush on EOF
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
