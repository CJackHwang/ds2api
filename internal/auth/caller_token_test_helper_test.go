package auth

import "net/http"

// extractCallerToken is a test-only wrapper around extractCallerTokenCore that
// preserves the historical default of accepting Gemini-style `?key=` / `?api_key=`
// query parameters as a fallback credential.
//
// Production code always goes through (*Resolver).extractCallerToken, which
// consults config.auth.allow_gemini_query_key (and DS2API_ALLOW_GEMINI_QUERY_KEY)
// to decide whether the query-key fallback is honoured.
func extractCallerToken(req *http.Request) string {
	return extractCallerTokenCore(req, true)
}
