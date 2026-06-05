// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// CommandView is the read-only view of a cobra.Command exposed to plugins
// and the policy engine. *cobra.Command is deliberately NOT reachable
// through this interface -- a plugin should never mutate the command tree.
//
// View semantics:
//
//   - The view is a live proxy over the underlying *cobra.Command and its
//     annotation chain. Strict-mode replaces nodes via RemoveCommand+
//     AddCommand; the replacement stub explicitly carries the original
//     command's annotations and help text forward so audit / compliance
//     observers still see Risk / Identities / Domain after a denial.
//     User-layer policy mutates in place, so its denyStubs preserve the
//     original metadata by construction.
//
//   - Path() is the canonical slash form ("docs/+fetch"), matching the
//     doublestar glob semantics used by Rule.Allow / Rule.Deny.
//
//   - Risk() returns ok=false when the command is unannotated. The policy
//     engine treats an unannotated command as implicit deny whenever any
//     Rule without AllowUnannotated=true is registered, so risk-based
//     Selectors never see unannotated commands during normal hook dispatch
//     under that configuration.
type CommandView interface {
	// Path is the canonical slash-separated path, rootless ("docs/+update").
	Path() string

	// Domain returns the business domain ("docs", "im", "") inherited from
	// the nearest ancestor with a cmdmeta.domain annotation. Empty string
	// when no ancestor declares one.
	Domain() string

	// Risk returns the static risk level. ok=false signals "no risk_level
	// annotation found in the parent chain" (unknown).
	Risk() (level Risk, ok bool)

	// Identities returns the supported identities. nil signals "no
	// supportedIdentities annotation in the parent chain".
	Identities() []Identity

	// Annotation exposes the raw cobra annotation map for plugins that
	// need a tag the framework does not surface.
	Annotation(key string) (string, bool)
}
