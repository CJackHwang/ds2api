package toolcall

import (
	"ds2api/internal/config"
	"log/slog"
	"reflect"
)

// ShadowDiffRecord holds the result of a shadow diff comparison between
// the existing parse result and the candidate produced by buildParseCandidate.
type ShadowDiffRecord struct {
	Ran          bool
	HasDiff      bool
	OldCallCount int
	NewCallCount int
	OldSawSyntax bool
	NewSawSyntax bool
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
		Ran:          true,
		HasDiff:      hasDiff,
		OldCallCount: len(oldCalls),
		NewCallCount: len(newCalls),
		OldSawSyntax: existing.SawToolCallSyntax,
		NewSawSyntax: cand.sawToolCallSyntax,
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
			"new_parse_path", cand.parsePath,
			"new_ambiguous", cand.ambiguous,
			"new_whitelist_hit", cand.nameWhitelistHit,
		)
	}

	return rec
}
