package util

import "regexp"

var (
	reBearerToken = regexp.MustCompile(`(?i)("authorization"\s*:\s*"Bearer\s+)[^"]+("?)`)
	reAPIKey      = regexp.MustCompile(`(?i)("(?:api[_-]key|x-api-key|apikey)"\s*:\s*")[^"]+("?)`)
)

// RedactBearerToken replaces Bearer token values in a JSON string with "<redacted>".
// Only the token portion of "Authorization": "Bearer <token>" is replaced.
func RedactBearerToken(s string) string {
	return reBearerToken.ReplaceAllString(s, `${1}<redacted>${2}`)
}

// RedactAPIKey replaces api_key / x-api-key / apikey field values in a JSON
// string with "<redacted>".
func RedactAPIKey(s string) string {
	return reAPIKey.ReplaceAllString(s, `${1}<redacted>${2}`)
}

// RedactSensitiveFields applies both RedactBearerToken and RedactAPIKey to s.
// Intended for use on request body JSON before storing in devcapture.
func RedactSensitiveFields(s string) string {
	s = RedactBearerToken(s)
	s = RedactAPIKey(s)
	return s
}
