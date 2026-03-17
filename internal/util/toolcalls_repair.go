package util

import (
	"regexp"
	"strings"
)

var unquotedKeyPattern = regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)

// fallback regex for shallow nested objects.
var missingArrayBracketsPattern = regexp.MustCompile(`(:\s*)(\{(?:[^{}]|\{[^{}]*\})*\}(?:\s*,\s*\{(?:[^{}]|\{[^{}]*\})*\})+)`)

func repairInvalidJSONBackslashes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var out strings.Builder
	out.Grow(len(s) + 10)
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' {
			if i+1 < len(runes) {
				next := runes[i+1]
				switch next {
				case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
					out.WriteRune('\\')
					out.WriteRune(next)
					i++
					continue
				case 'u':
					if i+5 < len(runes) {
						isHex := true
						for j := 1; j <= 4; j++ {
							r := runes[i+1+j]
							if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
								isHex = false
								break
							}
						}
						if isHex {
							out.WriteRune('\\')
							out.WriteRune('u')
							for j := 1; j <= 4; j++ {
								out.WriteRune(runes[i+1+j])
							}
							i += 5
							continue
						}
					}
				}
			}
			out.WriteString("\\\\")
		} else {
			out.WriteRune(runes[i])
		}
	}
	return out.String()
}

func RepairLooseJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = unquotedKeyPattern.ReplaceAllString(s, `$1"$2":`)
	repaired := repairMissingArrayBracketsScanner(s)
	if repaired == s {
		repaired = missingArrayBracketsPattern.ReplaceAllString(s, `$1[$2]`)
	}
	return repaired
}

func repairMissingArrayBracketsScanner(s string) string {
	const maxScanLen = 200000
	if len(s) == 0 || len(s) > maxScanLen {
		return s
	}

	var out strings.Builder
	out.Grow(len(s) + 8)
	cursor := 0
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c != ':' {
			continue
		}
		end, ws, ok := detectObjectListRange(s, i+1)
		if !ok {
			continue
		}
		out.WriteString(s[cursor : i+1])
		out.WriteString(s[i+1 : i+1+ws])
		out.WriteByte('[')
		out.WriteString(s[i+1+ws : end])
		out.WriteByte(']')
		cursor = end
		i = end - 1
	}
	if cursor == 0 {
		return s
	}
	out.WriteString(s[cursor:])
	return out.String()
}

func detectObjectListRange(s string, start int) (end int, leadingWS int, ok bool) {
	i := skipWhitespaceASCII(s, start)
	if i >= len(s) || s[i] != '{' {
		return 0, 0, false
	}
	firstEnd, ok := scanJSONObjectEnd(s, i)
	if !ok {
		return 0, 0, false
	}
	pos := firstEnd
	count := 1
	for {
		comma := skipWhitespaceASCII(s, pos)
		if comma >= len(s) || s[comma] != ',' {
			break
		}
		nextObj := skipWhitespaceASCII(s, comma+1)
		if nextObj >= len(s) || s[nextObj] != '{' {
			break
		}
		nextEnd, ok := scanJSONObjectEnd(s, nextObj)
		if !ok {
			break
		}
		count++
		pos = nextEnd
	}
	if count < 2 {
		return 0, 0, false
	}
	return pos, i - start, true
}

func scanJSONObjectEnd(s string, start int) (int, bool) {
	if start >= len(s) || s[start] != '{' {
		return 0, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func skipWhitespaceASCII(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func repairPathLikeEscapesInJSON(raw string) (string, bool) {
	pathValuePattern := regexp.MustCompile(`(?i)("(?:path|file|cwd|dir|directory|filepath|filename)"\s*:\s*")((?:[^"\\]|\\.)*)(")`)
	changed := false
	repaired := pathValuePattern.ReplaceAllStringFunc(raw, func(seg string) string {
		parts := pathValuePattern.FindStringSubmatch(seg)
		if len(parts) != 4 {
			return seg
		}
		fixed, localChanged := doubleSingleBackslashes(parts[2])
		if localChanged {
			changed = true
		}
		return parts[1] + fixed + parts[3]
	})
	return repaired, changed
}

func doubleSingleBackslashes(s string) (string, bool) {
	if !strings.Contains(s, "\\") {
		return s, false
	}
	var out strings.Builder
	out.Grow(len(s) + 8)
	changed := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			changed = true
			out.WriteString("\\n")
			continue
		case '\r':
			changed = true
			out.WriteString("\\r")
			continue
		case '\t':
			changed = true
			out.WriteString("\\t")
			continue
		}
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '\\' {
			out.WriteString("\\\\")
			i++
			continue
		}
		changed = true
		out.WriteString("\\\\")
		if i+1 < len(s) {
			out.WriteByte(s[i+1])
			i++
		}
	}
	return out.String(), changed
}

func hasPathFieldControlChars(v map[string]any) bool {
	for k, raw := range v {
		if isPathLikeKey(k) {
			if s, ok := raw.(string); ok && strings.ContainsAny(s, "\n\r\t") {
				return true
			}
		}
		switch x := raw.(type) {
		case map[string]any:
			if hasPathFieldControlChars(x) {
				return true
			}
		case []any:
			for _, item := range x {
				if m, ok := item.(map[string]any); ok && hasPathFieldControlChars(m) {
					return true
				}
			}
		}
	}
	return false
}

func isPathLikeKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "path", "file", "cwd", "dir", "directory", "filepath", "filename":
		return true
	default:
		return false
	}
}

func escapeLiteralControlsInJSONStrings(s string) string {
	if s == "" {
		return s
	}
	var out strings.Builder
	out.Grow(len(s) + 8)
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				out.WriteByte(c)
				continue
			}
			if c == '\\' {
				escaped = true
				out.WriteByte(c)
				continue
			}
			switch c {
			case '\n':
				out.WriteString("\\n")
				continue
			case '\r':
				out.WriteString("\\r")
				continue
			case '\t':
				out.WriteString("\\t")
				continue
			case '"':
				inString = false
			}
			out.WriteByte(c)
			continue
		}
		if c == '"' {
			inString = true
		}
		out.WriteByte(c)
	}
	return out.String()
}

func normalizePathLikeFields(m map[string]any) {
	for key, value := range m {
		switch x := value.(type) {
		case string:
			if isPathLikeKey(key) {
				x = strings.ReplaceAll(x, "\n", `\\n`)
				x = strings.ReplaceAll(x, "\r", `\\r`)
				x = strings.ReplaceAll(x, "\t", `\\t`)
				m[key] = x
			}
		case map[string]any:
			normalizePathLikeFields(x)
		case []any:
			for i, item := range x {
				if child, ok := item.(map[string]any); ok {
					normalizePathLikeFields(child)
					x[i] = child
				}
			}
		}
	}
}
