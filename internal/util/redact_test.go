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

func TestRedactSensitiveFieldsBearerNotRedacted(t *testing.T) {
	in := `{"Authorization":"Bearer tok123","model":"gpt-4"}`
	got := RedactSensitiveFields(in)
	if got != in {
		t.Errorf("Authorization Bearer should not be redacted from JSON body: got %q", got)
	}
}
