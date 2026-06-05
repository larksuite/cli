// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// Plugin is the single contract a third-party / embedding integrator
// implements to extend lark-cli. Four methods, every one mandatory.
//
// Name must match the grammar ^[a-z0-9][a-z0-9-]*$. The "." character
// is forbidden so plugin-name + hookName namespacing never produces
// ambiguous joins.
//
// Capabilities must be implemented even when every field is zero. The
// requirement is deliberate: it keeps FailurePolicy / Restricts in the
// author's eyeline.
//
// Install runs once during the Bootstrap pipeline. The plugin uses the
// supplied Registrar to register hooks and (optionally) a Rule. Errors
// returned from Install honour the plugin's Capabilities.FailurePolicy
// (fail-open warns + skips this plugin; fail-closed aborts the CLI).
type Plugin interface {
	Name() string
	Version() string
	Capabilities() Capabilities
	Install(r Registrar) error
}
