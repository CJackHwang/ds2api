// Package contextengine compiles a normalized turn sequence into a ContextPlan
// that describes which segments to include in the next completion request and
// which to trim, together with a token-budget report.
//
// Intended position in the processing pipeline:
//
//	promptcompat (normalise) → contextengine (plan) → completionruntime (send)
//
// M1 scope: segment mapping, orphan-tool-call detection, token budget
// accounting (no trimming yet — trimming is M3).
package contextengine
