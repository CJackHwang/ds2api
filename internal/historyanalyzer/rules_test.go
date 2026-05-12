package historyanalyzer

import "testing"

func TestKnownRuleSpecsReturnsCopy(t *testing.T) {
	specs := KnownRuleSpecs()
	if len(specs) != 10 {
		t.Fatalf("known rule specs = %d, want 10", len(specs))
	}
	specs[0].ID = "mutated"

	fresh := KnownRuleSpecs()
	if fresh[0].ID == "mutated" {
		t.Fatal("KnownRuleSpecs returned mutable backing store")
	}
}

func TestRuleSpecByID(t *testing.T) {
	spec, ok := RuleSpecByID(RuleAccountRetryExhausted)
	if !ok {
		t.Fatal("expected account retry exhausted spec")
	}
	if spec.Category != CategoryAccountRuntime {
		t.Fatalf("category = %q, want %q", spec.Category, CategoryAccountRuntime)
	}
	if spec.DefaultSeverity != SeverityHigh {
		t.Fatalf("severity = %q, want %q", spec.DefaultSeverity, SeverityHigh)
	}
}
