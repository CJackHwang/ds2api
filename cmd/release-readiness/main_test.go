package main

import (
	"testing"

	"ds2api/internal/readiness"
)

func TestParseRequiredGateResultRejectsSkip(t *testing.T) {
	_, err := parseRequiredGateResult("skip")
	if err == nil {
		t.Fatal("expected skip to be rejected for required gate")
	}
}

func TestParseGateResultAllowsLiveSkip(t *testing.T) {
	got, err := parseLiveGateResult("skip")
	if err != nil {
		t.Fatal(err)
	}
	if got != readiness.GateSkip {
		t.Fatalf("result = %q, want %q", got, readiness.GateSkip)
	}
}

func TestResolveBranchUsesExplicitValue(t *testing.T) {
	got, err := resolveBranch("release/test")
	if err != nil {
		t.Fatal(err)
	}
	if got != "release/test" {
		t.Fatalf("branch = %q, want release/test", got)
	}
}
