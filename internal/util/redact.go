package util

import "regexp"

var (
	reAPIKey = regexp.MustCompile(`(?i)("(?:api[_-]key|x-api-key|apikey)"\s*:\s*")[^"]*`)
)

// RedactAPIKey replaces api_key / x-api-key / apikey field values in a JSON
// string with "<redacted>".
func RedactAPIKey(s string) string {
	return reAPIKey.ReplaceAllString(s, `${1}<redacted>`)
}

// RedactSensitiveFields applies RedactAPIKey to s.
// Intended for use on JSON strings before storing in devcapture.
func RedactSensitiveFields(s string) string {
	return RedactAPIKey(s)
}
