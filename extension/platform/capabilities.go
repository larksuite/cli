// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// FailurePolicy controls what the framework does when a plugin's install
// stage fails (Capabilities() panics, Install returns error, etc.).
type FailurePolicy int

const (
	// FailOpen (default) — log a warning and skip THIS plugin; the rest
	// of the CLI keeps running. Appropriate for pure-observer plugins
	// where missing audit data is preferable to a broken CLI.
	FailOpen FailurePolicy = iota

	// FailClosed — abort the entire CLI startup. Required for any
	// plugin that contributes Restrict() (a missing policy plugin =
	// missing security boundary) or that owns any safety-sensitive
	// concern. Enforced by the framework: Capabilities.Restricts=true
	// must pair with FailurePolicy=FailClosed.
	FailClosed
)

// Capabilities declares the plugin's self-description. Plugin.Capabilities
// MUST be implemented even when every field would be its zero value --
// the requirement keeps FailurePolicy / Restricts visible to the author
// at the moment they write the plugin, preventing the "I just want to
// add an audit observer" mistake of accidentally shipping a policy
// plugin with the default FailOpen.
type Capabilities struct {
	// RequiredCLIVersion is a semver constraint (e.g. ">=1.1.0").
	// Plugins that need a specific framework feature should declare
	// the minimum version they tested against; the host fails the
	// install when the running CLI is older. Empty string means "no
	// version requirement".
	RequiredCLIVersion string

	// Restricts declares whether Install will call r.Restrict(). The
	// framework enforces consistency: declaring Restricts=true and
	// then NOT calling r.Restrict (or vice versa) aborts the install
	// with the `restricts_mismatch` reason_code. This pre-flight
	// declaration also lets `config policy show` introspect "which
	// plugins are policy plugins" without running them.
	Restricts bool

	// FailurePolicy decides what happens on install failure. See the
	// constants above; the framework requires FailClosed whenever
	// Restricts=true.
	FailurePolicy FailurePolicy
}
