package util

import (
	"testing"
)

func TestRedactBearerToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "lowercase authorization bearer",
			in:   `{"authorization":"Bearer sk-secret123","model":"gpt-4"}`,
			want: `{"authorization":"Bearer <redacted>","model":"gpt-4"}`,
		},
		{
			name: "mixed-case Authorization Bearer",
			in:   `{"Authorization":"Bearer my-token","content":"hello"}`,
			want: `{"Authorization":"Bearer <redacted>","content":"hello"}`,
		},
		{
			name: "no bearer token unchanged",
			in:   `{"model":"gpt-4","messages":[]}`,
			want: `{"model":"gpt-4","messages":[]}`,
		},
		{
			name: "empty string unchanged",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactBearerToken(tc.in)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

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
	in := `{"Authorization":"Bearer tok123","api_key":"sk-xyz","model":"gpt-4"}`
	got := RedactSensitiveFields(in)
	want := `{"Authorization":"Bearer <redacted>","api_key":"<redacted>","model":"gpt-4"}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
