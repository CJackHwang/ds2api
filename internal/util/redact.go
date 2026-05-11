// Package util — redact.go: field-level redaction helpers for structured
// payloads (typically JSON request/response bodies stored by devcapture or
// surfaced through admin debug APIs).
//
// Each Redact* helper targets a specific class of sensitive information:
//   - RedactAPIKey:  api_key / x-api-key / apikey JSON fields.
//   - RedactToken:   Authorization "Bearer …" plus token-family JSON fields
//     (token / access_token / refresh_token / id_token /
//     auth_token / bearer_token / session_token).
//   - RedactEmail:   email / e_mail / mail / email_address JSON fields.
//   - RedactMobile:  mobile / phone / tel / phone_number JSON fields.
//
// The helpers all operate on JSON-shaped strings using value-position regex
// (look-behind of `"<field>":"`) so they are safe to run on message bodies
// that contain free-form user text — content elsewhere in the string is left
// untouched. RedactSensitiveFields composes all of them and is the single
// entry point callers should prefer.
package util

import "regexp"

// fieldValueRedactor returns a compiled regex that matches the value portion
// of one of the supplied JSON field names. The replacement template is
// `${1}<redacted>`, where group 1 is the field name + colon + opening quote.
func fieldValueRedactor(fieldAlternation string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)("(?:` + fieldAlternation + `)"\s*:\s*")[^"]*`)
}

var (
	reAPIKey = fieldValueRedactor(`api[_-]key|x-api-key|apikey`)

	// Token-family JSON field values. Covers OAuth-style tokens commonly
	// found in request/response bodies. Does NOT match the HTTP
	// "Authorization" header field value as a whole — that is handled by
	// reBearer below so that the field key (e.g. "authorization") is
	// preserved verbatim.
	reTokenField = fieldValueRedactor(
		`token|access[_-]token|refresh[_-]token|id[_-]token|auth[_-]token|bearer[_-]token|session[_-]token`,
	)

	// Bearer scheme inside JSON string values. Matches `"Bearer <token>"`
	// anywhere a JSON string can carry an Authorization header (including
	// devcapture-recorded request headers). Group 1 keeps the literal
	// "Bearer " (case-preserved) so the scheme stays visible.
	reBearer = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~\-+/=]+`)

	// Email-family JSON field values.
	reEmailField = fieldValueRedactor(`email|e[_-]mail|mail|email[_-]address`)

	// Mobile / phone JSON field values.
	reMobileField = fieldValueRedactor(`mobile|phone|tel|phone[_-]number|mobile[_-]number|telephone`)
)

// RedactAPIKey replaces api_key / x-api-key / apikey field values in a JSON
// string with "<redacted>".
func RedactAPIKey(s string) string {
	return reAPIKey.ReplaceAllString(s, `${1}<redacted>`)
}

// RedactToken replaces token-family JSON field values and Authorization
// "Bearer …" scheme tokens with "<redacted>".
func RedactToken(s string) string {
	s = reTokenField.ReplaceAllString(s, `${1}<redacted>`)
	s = reBearer.ReplaceAllString(s, `${1}<redacted>`)
	return s
}

// RedactEmail replaces email-family JSON field values with "<redacted>".
func RedactEmail(s string) string {
	return reEmailField.ReplaceAllString(s, `${1}<redacted>`)
}

// RedactMobile replaces mobile / phone JSON field values with "<redacted>".
func RedactMobile(s string) string {
	return reMobileField.ReplaceAllString(s, `${1}<redacted>`)
}

// RedactSensitiveFields composes every redaction helper. Callers persisting
// JSON-shaped payloads (devcapture, admin debug APIs, log lines) should
// prefer this single entry point so adding new redaction rules in one place
// covers all storage paths.
func RedactSensitiveFields(s string) string {
	s = RedactAPIKey(s)
	s = RedactToken(s)
	s = RedactEmail(s)
	s = RedactMobile(s)
	return s
}
