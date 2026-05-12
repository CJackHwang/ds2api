// Package historyanalyzer contains the offline diagnostics model for turning
// saved request history into rule-based findings.
//
// The package is intentionally request-path neutral. It defines records,
// reports, rules, redaction helpers, ingestion helpers, and deterministic
// HA_* rules. CLI and Admin/WebUI integration are layered in later M4.1 phases.
package historyanalyzer
