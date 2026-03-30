package sse

import "strings"

var leakedContentFilterSuffixPrefixes = []string{
	"你好，这个问题我暂时无法回答",
	"您好，这个问题我暂时无法回答",
	"，这个问题我暂时无法回答",
	",这个问题我暂时无法回答",
}

func filterLeakedContentFilterParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return parts
	}
	out := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		cleaned := stripLeakedContentFilterSuffix(p.Text)
		if strings.TrimSpace(cleaned) == "" {
			continue
		}
		p.Text = cleaned
		out = append(out, p)
	}
	return out
}

func stripLeakedContentFilterSuffix(text string) string {
	if text == "" {
		return text
	}
	upper := strings.ToUpper(text)
	idx := strings.Index(upper, "CONTENT_FILTER")
	if idx < 0 {
		return text
	}
	suffix := strings.TrimSpace(text[idx+len("CONTENT_FILTER"):])
	if !looksLikeLeakedContentFilterSuffix(suffix) {
		return text
	}
	return strings.TrimRight(text[:idx], " \t\r\n")
}

func looksLikeLeakedContentFilterSuffix(suffix string) bool {
	if suffix == "" {
		return false
	}
	for _, p := range leakedContentFilterSuffixPrefixes {
		if strings.HasPrefix(suffix, p) {
			return true
		}
	}
	return false
}
