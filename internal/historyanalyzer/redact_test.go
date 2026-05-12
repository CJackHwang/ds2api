package historyanalyzer

import (
	"strings"
	"testing"
)

func TestRedactorRedactsSensitiveText(t *testing.T) {
	redactor := DefaultRedactor()
	input := `{"api_key":"abc123","authorization":"Bearer secret-token","email":"user@example.com"} sk-testsecret123456789 github_pat_abcdef1234567890`

	got := redactor.Redact(input)
	for _, forbidden := range []string{"abc123", "secret-token", "user@example.com", "sk-testsecret", "github_pat_abcdef"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted output still contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("redacted output missing replacement marker: %s", got)
	}
}

func TestEvidenceRedactsTruncatesAndHashes(t *testing.T) {
	redactor := Redactor{MaxExcerptRunes: 8}
	evidence := redactor.Evidence("chat", "final_prompt", "Bearer abcdefghijklmnop tail", "sample")

	if strings.Contains(evidence.Excerpt, "abcdefghijklmnop") {
		t.Fatalf("evidence excerpt leaked token: %s", evidence.Excerpt)
	}
	if !strings.Contains(evidence.Excerpt, "<truncated>") {
		t.Fatalf("evidence excerpt was not truncated: %s", evidence.Excerpt)
	}
	if evidence.Hash == "" {
		t.Fatal("evidence hash is empty")
	}
	if evidence.Note != "sample" {
		t.Fatalf("note = %q, want sample", evidence.Note)
	}
}
