// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "github.com/larksuite/cli/internal/core"

// Risk is the three-tier risk taxonomy declared on every command.
//
// A defined type (not an alias of string) so plugin authors get
// compile-time + IDE candidate help when passing the constants below.
// Crossing the string boundary (yaml, cobra annotation) goes through
// ParseRisk so typos surface as `risk_invalid` rather than silently
// flowing through.
//
// This stays a plugin-SDK type of its own — the exported signature does not
// change — but the taxonomy itself is no longer defined twice: the constants
// below are derived from internal/core, and Core/FromCore convert between the
// two. A consistency test pins the two value sets together.
type Risk string

const (
	RiskRead          = Risk(core.RiskRead)
	RiskWrite         = Risk(core.RiskWrite)
	RiskHighRiskWrite = Risk(core.RiskHighRiskWrite)
)

// Core converts to the internal taxonomy. The two types carry identical
// values, so the conversion is total in both directions.
func (r Risk) Core() core.Risk { return core.Risk(r) }

// FromCore converts an internal Risk into the SDK-facing type.
func FromCore(r core.Risk) Risk { return Risk(r) }

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
	r, err := core.ParseRisk(s)
	if err != nil {
		return "", err
	}
	return Risk(r), nil
}

// IsValid reports whether r is one of the three recognised values.
func (r Risk) IsValid() bool { return core.Risk(r).IsValid() }

// Rank returns the comparable rank of r. ok=false when r is not in the
// closed taxonomy. The pruning engine compares ranks for the MaxRisk axis.
func (r Risk) Rank() (rank int, ok bool) { return core.Risk(r).Rank() }

// String returns the underlying string. Useful for yaml/json output
// and cobra annotation injection.
func (r Risk) String() string { return string(r) }
