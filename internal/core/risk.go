// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "fmt"

// Risk is the three-tier risk taxonomy declared on every command.
//
// A defined type (not an alias of string) so no `string` value can reach a
// declaration without a conversion, and so editors offer the three constants.
// Note what the type does NOT do: an untyped literal still converts
// implicitly, so `Risk: "high-risk-wrtie"` compiles. Keeping typos out is
// therefore a three-layer job — the type here, the quality-gate rules that
// reject a hand-written literal and a manifest value outside the enum, and
// the runtime gate (cmdutil.EnforceRiskDeclaration) that refuses to run a
// command whose level it does not recognise.
//
// Crossing a string boundary — cobra annotations, generated service metadata,
// plugin manifests — goes through ParseRisk so a bad value surfaces as an
// error instead of flowing through as the lowest tier.
//
// This is the single source of truth for the taxonomy. internal/cmdutil
// re-exports these constants for command code, extension/platform mirrors
// them for the plugin SDK (its own defined type, converted here), and errs
// carries the wire-level strings; all three are pinned to these values by
// consistency tests.
type Risk string

// Risk levels — the three-tier convention used across the CLI. They live here,
// at the leaf, so the envelope renderer (internal/schema) and the command
// toolkit (internal/cmdutil) share one vocabulary without a renderer depending
// on command utilities. Framework confirmation gating acts only on
// RiskHighRiskWrite.
const (
	RiskRead          Risk = "read"
	RiskWrite         Risk = "write"
	RiskHighRiskWrite Risk = "high-risk-write"
)

// riskOrder maps the taxonomy to a comparable rank. Absence from this map is
// what makes a value invalid — the map is the closed enum.
var riskOrder = map[Risk]int{
	RiskRead:          0,
	RiskWrite:         1,
	RiskHighRiskWrite: 2,
}

// ParseRisk converts a raw string (cobra annotation, generated metadata,
// plugin manifest) into a Risk.
//
//   - s == ""       → ("", nil)       "not specified"
//   - s in the enum → (Risk(s), nil)  OK
//   - anything else → ("", error)     invalid
//
// The absent-vs-invalid split mirrors the cmdpolicy engine's
// risk_not_annotated vs risk_invalid reason codes: callers can treat the
// ("", nil) case as "not specified" without losing the distinction from a
// typo, which is a code bug and must never be silently downgraded.
//
// Matching is strict: "Read" / "READ" / " read " are all rejected. These
// values come from developer code and generated metadata, not from user
// input — strict matching is the typo-catch mechanism, not a normalisation
// opportunity.
func ParseRisk(s string) (Risk, error) {
	if s == "" {
		return "", nil
	}
	r := Risk(s)
	if _, ok := riskOrder[r]; !ok {
		return "", fmt.Errorf("invalid risk %q: must be read|write|high-risk-write", s)
	}
	return r, nil
}

// IsValid reports whether r is one of the three recognised values. The empty
// value is not valid — callers that accept "not specified" must check for it
// separately, so absent and invalid never collapse into one branch.
func (r Risk) IsValid() bool {
	_, ok := riskOrder[r]
	return ok
}

// Rank returns the comparable rank of r. ok=false when r is outside the
// closed taxonomy.
func (r Risk) Rank() (rank int, ok bool) {
	rank, ok = riskOrder[r]
	return rank, ok
}

// String returns the underlying string, for annotation injection and
// json/yaml output.
func (r Risk) String() string { return string(r) }
