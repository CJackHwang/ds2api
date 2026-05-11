package util

import (
	"testing"
)

func TestRedactAPIKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "api_key field",
			in:   `{"api_key":"sk-secret","model":"gpt-4"}`,
			want: `{"api_key":"<redacted>","model":"gpt-4"}`,
		},
		{
			name: "x-api-key field",
			in:   `{"x-api-key":"my-key","path":"/v1/chat"}`,
			want: `{"x-api-key":"<redacted>","path":"/v1/chat"}`,
		},
		{
			name: "apikey field",
			in:   `{"apikey":"abc123"}`,
			want: `{"apikey":"<redacted>"}`,
		},
		{
			name: "no api key field unchanged",
			in:   `{"model":"gpt-4","messages":[]}`,
			want: `{"model":"gpt-4","messages":[]}`,
		},
		{
			name: "empty api_key value redacted",
			in:   `{"api_key":"","model":"gpt-4"}`,
			want: `{"api_key":"<redacted>","model":"gpt-4"}`,
		},
		{
			name: "malformed json no closing quote left unchanged",
			in:   `{"api_key":"no-closing-quote`,
			want: `{"api_key":"<redacted>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactAPIKey(tc.in)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRedactSensitiveFieldsNoOp(t *testing.T) {
	in := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
	got := RedactSensitiveFields(in)
	if got != in {
		t.Errorf("expected no-op: got %q", got)
	}
}

func TestRedactSensitiveFieldsCombined(t *testing.T) {
	in := `{"api_key":"sk-xyz","x-api-key":"other-key","model":"gpt-4"}`
	got := RedactSensitiveFields(in)
	want := `{"api_key":"<redacted>","x-api-key":"<redacted>","model":"gpt-4"}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestRedactSensitiveFieldsBearerIsRedacted documents the M1 PR
// `feat/m1-governance-log-redaction-expand` behavior change: Authorization
// Bearer tokens MUST be redacted from any JSON-shaped payload that flows
// through devcapture / admin debug APIs. The previous "Bearer not redacted"
// expectation has been intentionally inverted.
func TestRedactSensitiveFieldsBearerIsRedacted(t *testing.T) {
	in := `{"Authorization":"Bearer tok123","model":"gpt-4"}`
	got := RedactSensitiveFields(in)
	want := `{"Authorization":"Bearer <redacted>","model":"gpt-4"}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestRedactToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bearer scheme in authorization-style field",
			in:   `{"authorization":"Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig"}`,
			want: `{"authorization":"Bearer <redacted>"}`,
		},
		{
			name: "bearer scheme case insensitive",
			in:   `{"Authorization":"bearer tok-abc"}`,
			want: `{"Authorization":"bearer <redacted>"}`,
		},
		{
			name: "token field",
			in:   `{"token":"abc123","model":"gpt-4"}`,
			want: `{"token":"<redacted>","model":"gpt-4"}`,
		},
		{
			name: "access_token field",
			in:   `{"access_token":"ya29.xyz","expires_in":3600}`,
			want: `{"access_token":"<redacted>","expires_in":3600}`,
		},
		{
			name: "refresh_token field",
			in:   `{"refresh_token":"1//rt-zzz"}`,
			want: `{"refresh_token":"<redacted>"}`,
		},
		{
			name: "id_token / id-token hyphenated variant",
			in:   `{"id-token":"jwt.value.sig"}`,
			want: `{"id-token":"<redacted>"}`,
		},
		{
			name: "bearer token with tilde (RFC 6750 token68 charset)",
			in:   `{"Authorization":"Bearer abc~def"}`,
			want: `{"Authorization":"Bearer <redacted>"}`,
		},
		{
			name: "non-bearer scheme left alone (only Bearer is redacted)",
			in:   `{"Authorization":"Basic dXNlcjpwYXNz"}`,
			want: `{"Authorization":"Basic dXNlcjpwYXNz"}`,
		},
		{
			name: "free-form word 'token' in user content not affected",
			in:   `{"content":"please refresh the token in settings"}`,
			want: `{"content":"please refresh the token in settings"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactToken(tc.in)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRedactEmail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "email field",
			in:   `{"email":"user@example.com","name":"alice"}`,
			want: `{"email":"<redacted>","name":"alice"}`,
		},
		{
			name: "e_mail underscore variant",
			in:   `{"e_mail":"u@example.com"}`,
			want: `{"e_mail":"<redacted>"}`,
		},
		{
			name: "email_address compound variant",
			in:   `{"email_address":"u@example.com"}`,
			want: `{"email_address":"<redacted>"}`,
		},
		{
			name: "free-form email-like substring in content unchanged",
			in:   `{"content":"contact me at user@example.com please"}`,
			want: `{"content":"contact me at user@example.com please"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactEmail(tc.in)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRedactMobile(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "mobile field",
			in:   `{"mobile":"13800138000"}`,
			want: `{"mobile":"<redacted>"}`,
		},
		{
			name: "phone field",
			in:   `{"phone":"+1-415-555-0100","name":"bob"}`,
			want: `{"phone":"<redacted>","name":"bob"}`,
		},
		{
			name: "phone_number compound variant",
			in:   `{"phone_number":"+8613800138000"}`,
			want: `{"phone_number":"<redacted>"}`,
		},
		{
			name: "tel field",
			in:   `{"tel":"021-12345678"}`,
			want: `{"tel":"<redacted>"}`,
		},
		{
			name: "free-form number in content unchanged",
			in:   `{"content":"my number is 13800138000 thanks"}`,
			want: `{"content":"my number is 13800138000 thanks"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactMobile(tc.in)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRedactSensitiveFieldsCombinedAllFamilies(t *testing.T) {
	in := `{"api_key":"sk-x","Authorization":"Bearer tk","email":"u@e.com","mobile":"13800138000","model":"gpt-4"}`
	got := RedactSensitiveFields(in)
	want := `{"api_key":"<redacted>","Authorization":"Bearer <redacted>","email":"<redacted>","mobile":"<redacted>","model":"gpt-4"}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
