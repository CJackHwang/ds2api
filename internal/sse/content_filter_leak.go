package sse

import "strings"

func filterLeakedContentFilterParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return parts
	}
	out := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		cleaned := stripLeakedContentFilterSuffix(p.Text)
		if shouldDropCleanedLeakedChunk(cleaned) {
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
	upperText := strings.ToUpper(text)
	idx := strings.Index(upperText, "CONTENT_FILTER")
	if idx < 0 {
		return text
	}
	// Keep "\n" so we don't collapse line structure when the upstream model
	// appends leaked CONTENT_FILTER markers after a line break.
	return strings.TrimRight(text[:idx], " \t\r")
}

func shouldDropCleanedLeakedChunk(cleaned string) bool {
	if cleaned == "" {
		return true
	}
	// Preserve newline-only chunks to avoid dropping legitimate line breaks
	// before a leaked CONTENT_FILTER suffix.
	if strings.Contains(cleaned, "\n") {
		return false
	}
	return strings.TrimSpace(cleaned) == ""
}
