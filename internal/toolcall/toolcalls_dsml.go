package toolcall

import (
	"regexp"
	"strings"
)

var dsmlToolTagPattern = regexp.MustCompile(`(?is)<\s*/?\s*(?:\|?\s*dsml\s*\|)\s*(tool_calls|invoke|parameter)\b`)
var dsmlToolTagNormalizePattern = regexp.MustCompile(`(?is)<\s*(/?)\s*(?:\|?\s*dsml\s*\|)\s*(tool_calls|invoke|parameter)\b`)

func normalizeDSMLToolCallSyntax(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	if !dsmlToolTagPattern.MatchString(text) {
		return text
	}
	return dsmlToolTagNormalizePattern.ReplaceAllString(text, "<$1$2")
}
