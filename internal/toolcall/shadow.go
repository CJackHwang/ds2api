package toolcall

import (
	"ds2api/internal/config"
	"log/slog"
	"reflect"
)

// ParseConfidence classifies how confident the parser is in a shadow-diff result.
type ParseConfidence int

const (
	// ConfidenceHigh: xml_direct path, no ambiguity, whitelist hit → safe to enforce.
	ConfidenceHigh ParseConfidence = iota
	// ConfidenceMedium: parsed successfully but one signal is weak
	// (e.g. no whitelist hit, or CDATA recovery was needed).
	ConfidenceMedium
	// ConfidenceLow: ambiguous input, normalize failed, or no tool syntax seen.
	ConfidenceLow
)

func (c ParseConfidence) String() string {
	switch c {
	case ConfidenceHigh:
		return "high"
	case ConfidenceMedium:
		return "medium"
	default:
		return "low"
	}
}

// ShadowDiffRecord holds the result of a shadow diff comparison between
// the existing parse result and the candidate produced by buildParseCandidate.
type ShadowDiffRecord struct {
	Ran          bool
	HasDiff      bool
	OldCallCount int
	NewCallCount int
	OldSawSyntax bool
	NewSawSyntax bool
	// Confidence signals from the candidate (new) parser run.
	NewParsePath    string // parsePathXxx constant
	NewAmbiguous    bool   // both DSML and canonical wrapper syntax coexist
	NewWhitelistHit bool   // ≥1 call name in availableNames
}

// RunShadowDiff runs buildParseCandidate on the same source text that produced
// existing (existing.SourceText) and compares the results. It is a no-op when
// mode != "shadow". Diffs are written to structured logs under the key
// [parser_shadow_diff]; they are never exposed to callers.
func RunShadowDiff(mode string, existing ToolCallParseResult) ShadowDiffRecord {
	if mode != "shadow" {
		return ShadowDiffRecord{}
	}

	cand := buildParseCandidate(existing.SourceText, existing.AvailableNames)

	oldCalls := existing.Calls
	newCalls := cand.calls

	callsMatch := reflect.DeepEqual(oldCalls, newCalls)
	syntaxMatch := existing.SawToolCallSyntax == cand.sawToolCallSyntax

	hasDiff := !callsMatch || !syntaxMatch

	rec := ShadowDiffRecord{
		Ran:             true,
		HasDiff:         hasDiff,
		OldCallCount:    len(oldCalls),
		NewCallCount:    len(newCalls),
		OldSawSyntax:    existing.SawToolCallSyntax,
		NewSawSyntax:    cand.sawToolCallSyntax,
		NewParsePath:    cand.parsePath,
		NewAmbiguous:    cand.ambiguous,
		NewWhitelistHit: cand.nameWhitelistHit,
	}

	if hasDiff {
		logger := config.Logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Info("[parser_shadow_diff]",
			"has_diff", true,
			"old_call_count", rec.OldCallCount,
			"new_call_count", rec.NewCallCount,
			"old_saw_syntax", rec.OldSawSyntax,
			"new_saw_syntax", rec.NewSawSyntax,
			"new_parse_path", rec.NewParsePath,
			"new_ambiguous", rec.NewAmbiguous,
			"new_whitelist_hit", rec.NewWhitelistHit,
			"confidence", ClassifyConfidence(rec).String(),
		)
	}

	return rec
}

// ClassifyConfidence maps the confidence signals in a ShadowDiffRecord to a
// ParseConfidence level. Safe to call on the zero-value record (Ran == false),
// which returns ConfidenceLow.
//
// Rules (evaluated in order; first match wins):
//
//  1. Ran == false (zero-value record) → Low.
//  2. No tool-call syntax seen → Low (no call, nothing to be confident about).
//  3. Ambiguous (both DSML and canonical wrappers) → Low.
//  4. ParsePath is empty, stripped_empty, normalize_failed, or xml_parse_failed → Low.
//  5. xml_direct + whitelist hit → High.
//  6. xml_direct without whitelist hit → Medium (parsed but names unknown).
//  7. xml_cdata_recover → Medium (needed recovery heuristic).
//  8. Fallthrough → Medium.
func ClassifyConfidence(r ShadowDiffRecord) ParseConfidence {
	if !r.Ran {
		return ConfidenceLow
	}
	if !r.NewSawSyntax {
		return ConfidenceLow
	}
	if r.NewAmbiguous {
		return ConfidenceLow
	}
	switch r.NewParsePath {
	case parsePathEmpty, parsePathStrippedEmpty, parsePathNormalizeFailed, parsePathXMLFailed:
		return ConfidenceLow
	case parsePathXMLDirect:
		if r.NewWhitelistHit {
			return ConfidenceHigh
		}
		return ConfidenceMedium
	case parsePathXMLCDATARecover:
		return ConfidenceMedium
	}
	return ConfidenceMedium
}
