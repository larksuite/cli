// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "fmt"

// Risk is the three-tier risk taxonomy declared on every command.
//
// A defined type (not an alias of string) so plugin authors get
// compile-time + IDE candidate help when passing the constants below.
// Crossing the string boundary (yaml, cobra annotation) goes through
// ParseRisk so typos surface as `risk_invalid` rather than silently
// flowing through.
type Risk string

const (
	RiskRead          Risk = "read"
	RiskWrite         Risk = "write"
	RiskHighRiskWrite Risk = "high-risk-write"
)

// riskOrder maps the Risk taxonomy to a comparable rank. The pruning
// engine compares ranks for the MaxRisk axis.
var riskOrder = map[Risk]int{
	RiskRead:          0,
	RiskWrite:         1,
	RiskHighRiskWrite: 2,
}

// ParseRisk converts a raw string (yaml, cobra annotation) into a Risk.
//
//   - s == ""        → ("", nil)            "not specified"
//   - s 在闭合枚举   → (Risk(s), nil)       OK
//   - s 不在枚举内   → ("", error)          invalid
//
// The (absent vs invalid) split mirrors the cmdpolicy engine's
// risk_not_annotated vs risk_invalid reason codes — callers can treat
// the "" + nil case as "not specified" without losing the distinction
// from a typo.
//
// Matching is strict: "Read" / "READ" / " read " are all rejected.
// annotation is developer code, not user input — strict matching is
// the typo-catch mechanism, not a normalisation opportunity.
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

// IsValid reports whether r is one of the three recognised values.
func (r Risk) IsValid() bool {
	_, ok := riskOrder[r]
	return ok
}

// Rank returns the comparable rank of r. ok=false when r is not in the
// closed taxonomy.
func (r Risk) Rank() (rank int, ok bool) {
	rank, ok = riskOrder[r]
	return rank, ok
}

// String returns the underlying string. Useful for yaml/json output
// and cobra annotation injection.
func (r Risk) String() string { return string(r) }
