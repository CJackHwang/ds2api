package toolcall

// toolcalls_candidates.go holds internal candidate helper logic for the
// parser pipeline confidence model.

// parsePath constants describe which code path was taken during parsing.
// These values are internal to parseCandidate and only surfaced via logs.
const (
	parsePathEmpty           = "empty"             // input was empty / all whitespace
	parsePathStrippedEmpty   = "stripped_empty"    // content existed only inside fenced code blocks
	parsePathNormalizeFailed = "normalize_failed"  // DSML normalisation returned an error
	parsePathXMLFailed       = "xml_parse_failed"  // normalised text yielded no XML tool calls
	parsePathXMLDirect       = "xml_direct"        // XML parsed successfully on the first attempt
	parsePathXMLCDATARecover = "xml_cdata_recover" // XML parsed only after loose-CDATA sanitisation
)

// namesHitWhitelist returns true when at least one call name appears in
// availableNames. Returns false when either slice is empty.
func namesHitWhitelist(calls []ParsedToolCall, availableNames []string) bool {
	if len(availableNames) == 0 || len(calls) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(availableNames))
	for _, n := range availableNames {
		set[n] = struct{}{}
	}
	for _, c := range calls {
		if _, ok := set[c.Name]; ok {
			return true
		}
	}
	return false
}
