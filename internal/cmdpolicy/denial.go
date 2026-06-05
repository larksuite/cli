// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import "sort"

// Layer values match CommandDeniedError.Layer and the detail.layer
// field of the JSON envelope (under error.type = "command_denied").
const (
	LayerStrictMode = "strict_mode"
	// LayerPolicy is the user-layer enforcement label. The string value
	// is "policy" — the package name "cmdpolicy" matches it. This
	// replaces the older "pruning" label.
	LayerPolicy = "policy"
)

// Denial is the merged record for a single rejected command path. It
// is distinct from the user-layer-only Decision type: Denial only
// exists when the command is rejected (the Allowed bool would be
// wasted here, hence not reusing Decision).
type Denial struct {
	Layer        string // "strict_mode" | "policy"
	PolicySource string // "plugin:secaudit" | "yaml:mywork" | "strict-mode" | ""
	RuleName     string // matched Rule.Name (if any)
	ReasonCode   string // closed enum, see docs/extension/reason-codes.md
	Reason       string // human-readable
}

// ChildDenial is what AggregateChildren consumes — it pairs a Denial
// with the child command's path so the aggregate can carry that
// breakdown for envelope.detail.children_denied.
type ChildDenial struct {
	Path   string
	Denial Denial
}

// AggregateChildren produces the parent-group Denial when every child
// of a command group is itself denied. The rules:
//
//   - all children share Layer "strict_mode" → parent Layer =
//     strict_mode, parent ReasonCode = single child's ReasonCode (if
//     consistent) or "mixed_children_strict_mode" otherwise.
//   - all children share Layer "policy" → parent Layer = policy,
//     ReasonCode behaves analogously.
//   - mixed layers across children → parent Layer = "policy",
//     ReasonCode = "all_children_denied", PolicySource = "mixed".
//
// Calling with an empty slice returns a zero Denial — callers should
// treat this as "no aggregation needed".
func AggregateChildren(children []ChildDenial) Denial {
	if len(children) == 0 {
		return Denial{}
	}

	layers := map[string]struct{}{}
	reasonCodes := map[string]struct{}{}
	sources := map[string]struct{}{}
	ruleNames := map[string]struct{}{}
	for _, c := range children {
		layers[c.Denial.Layer] = struct{}{}
		reasonCodes[c.Denial.ReasonCode] = struct{}{}
		if c.Denial.PolicySource != "" {
			sources[c.Denial.PolicySource] = struct{}{}
		}
		if c.Denial.RuleName != "" {
			ruleNames[c.Denial.RuleName] = struct{}{}
		}
	}

	// Mixed: layers differ across children. Parent goes to Layer=policy
	// (the more "user-recoverable" of the two — swapping policy can
	// flip children, swapping credential cannot).
	if len(layers) > 1 {
		return Denial{
			Layer:        LayerPolicy,
			PolicySource: "mixed",
			ReasonCode:   "all_children_denied",
			Reason:       "all child commands are denied (mixed reasons)",
		}
	}

	var layer string
	for l := range layers {
		layer = l
	}

	d := Denial{Layer: layer}

	switch len(reasonCodes) {
	case 1:
		for rc := range reasonCodes {
			d.ReasonCode = rc
		}
	default:
		switch layer {
		case LayerStrictMode:
			d.ReasonCode = "mixed_children_strict_mode"
		default:
			d.ReasonCode = "mixed_children_policy"
		}
	}

	if len(sources) == 1 {
		for s := range sources {
			d.PolicySource = s
		}
	}
	if layer == LayerStrictMode {
		d.PolicySource = "strict-mode"
	}

	if len(ruleNames) == 1 {
		for n := range ruleNames {
			d.RuleName = n
		}
	}

	d.Reason = "all child commands are denied"
	return d
}

// SortChildren orders children by Path. The aggregate output of
// AggregateChildren is deterministic regardless of slice order, but
// tests and the envelope's children_denied list want a stable order.
func SortChildren(children []ChildDenial) {
	sort.Slice(children, func(i, j int) bool {
		return children[i].Path < children[j].Path
	})
}
