// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// Rule is the declarative policy rule data structure. yaml files and
// Plugin.Restrict() both produce the same Rule.
//
// At any moment there is at most one effective Rule -- the resolver decides
// which source wins (Plugin > yaml > none). This package only defines the
// shape; selection lives in internal/cmdpolicy.
//
// The four filter fields are joined by AND. See the engine's Evaluate for
// the full semantics. JSON tags are used by `config policy show`; yaml
// parsing lives in internal/cmdpolicy/yaml so the public API does not
// depend on a yaml library.
type Rule struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Allow is a list of doublestar globs (slash-separated paths). An empty
	// slice means "no path restriction"; a non-empty slice means "command
	// path must match at least one glob".
	Allow []string `json:"allow,omitempty"`

	// Deny is a list of doublestar globs. A path that matches any Deny glob
	// is rejected regardless of Allow.
	Deny []string `json:"deny,omitempty"`

	// MaxRisk is the highest allowed risk level (inclusive). Empty string
	// means "no risk restriction". Comparison uses the closed taxonomy
	// read < write < high-risk-write.
	MaxRisk Risk `json:"max_risk,omitempty"`

	// Identities is the allowed identity whitelist. A command passes when
	// the intersection with the command's own supported identities is
	// non-empty. Empty slice means "no identity restriction".
	Identities []Identity `json:"identities,omitempty"`

	// AllowUnannotated controls how commands missing a risk_level
	// annotation are handled when this Rule is active.
	//
	// Default (false, fail-closed): unannotated commands are rejected
	// with reason_code=risk_not_annotated. This is the safe default
	// — a typo'd or forgotten annotation cannot slip past an
	// "agent read-only" rule.
	//
	// Set to true to opt out during gradual adoption: lark-cli main
	// has hundreds of service commands that may not yet carry
	// risk_level annotations, and a brand-new policy plugin would
	// otherwise lock the binary to nothing.
	//
	// This flag does NOT affect risk_invalid (typos): a command that
	// claims a risk but mis-spells it is always denied, regardless of
	// AllowUnannotated. Typo is a code bug, not a migration phase.
	//
	// No yaml tag: yaml decoding lives in internal/cmdpolicy/yaml so
	// platform stays free of a yaml library dependency.
	AllowUnannotated bool `json:"allow_unannotated,omitempty"`
}
