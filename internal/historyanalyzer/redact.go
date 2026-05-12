package historyanalyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"

	"ds2api/internal/util"
)

const defaultMaxExcerptRunes = 240

type Redactor struct {
	MaxExcerptRunes int
}

var (
	reInlineBearer = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~\-+/=]+`)
	reOpenAIToken  = regexp.MustCompile(`\bsk-[A-Za-z0-9][A-Za-z0-9_-]{10,}\b`)
	reAnthropicKey = regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{10,}\b`)
	reGitHubToken  = regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{10,}\b`)
	reGitHubPAT    = regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{10,}\b`)
	reEmail        = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
)

func DefaultRedactor() Redactor {
	return Redactor{MaxExcerptRunes: defaultMaxExcerptRunes}
}

func (r Redactor) Redact(text string) string {
	text = util.RedactSensitiveFields(text)
	text = reInlineBearer.ReplaceAllString(text, "Bearer <redacted>")
	text = reAnthropicKey.ReplaceAllString(text, "<redacted>")
	text = reOpenAIToken.ReplaceAllString(text, "<redacted>")
	text = reGitHubToken.ReplaceAllString(text, "<redacted>")
	text = reGitHubPAT.ReplaceAllString(text, "<redacted>")
	text = reEmail.ReplaceAllString(text, "<redacted-email>")
	return text
}

func (r Redactor) Excerpt(text string) string {
	text = strings.TrimSpace(r.Redact(text))
	limit := r.MaxExcerptRunes
	if limit <= 0 {
		limit = defaultMaxExcerptRunes
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "...<truncated>"
}

func (r Redactor) Evidence(source, field, text, note string) Evidence {
	excerpt := r.Excerpt(text)
	return Evidence{
		Source:  strings.TrimSpace(source),
		Field:   strings.TrimSpace(field),
		Excerpt: excerpt,
		Hash:    HashText(excerpt),
		Note:    strings.TrimSpace(note),
	}
}

func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
