// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// Registrar is the imperative API a plugin uses inside its Install
// method to wire up hooks and rules. The framework provides a staging
// implementation that buffers calls and commits them atomically when
// Install returns nil; failure rolls everything back.
//
// hookName must match the grammar ^[a-z0-9][a-z0-9-]*$ (no dots). The
// framework prepends the plugin's Name() with a dot so the global hook
// identifier is "{plugin}.{hook}". A plugin cannot register two hooks
// with the same name in the same Install call.
//
// Restrict may be called multiple times per plugin; each call adds one
// scoped Rule (OR-combined by the engine). Two or more DISTINCT plugins
// contributing Restrict() is a configuration error (the resolver aborts
// startup).
type Registrar interface {
	// Observe registers a side-effect-only command hook at the given
	// When stage. The selector decides which commands it fires on.
	Observe(when When, hookName string, sel Selector, fn Observer)

	// Wrap registers a middleware-style command hook. The Wrap chain
	// composes left-to-right in registration order; the outermost
	// Wrapper runs first.
	Wrap(hookName string, sel Selector, w Wrapper)

	// On registers a lifecycle handler for the given event.
	On(event LifecycleEvent, hookName string, fn LifecycleHandler)

	// Restrict contributes a pruning Rule. May be called more than once
	// to declare several scoped grants (OR-combined by the engine).
	// Plugin rules take precedence over the yaml source; two distinct
	// plugins both calling Restrict abort startup.
	Restrict(r *Rule)
}

// EmbeddedSkillsRegistrar is the optional extension a host registrar
// implements to accept embedded-skill customization. It is deliberately NOT
// part of Registrar: every exported symbol in this package is a stability
// contract, and widening Registrar would break existing third-party
// implementations (fakes, decorators, custom hosts). A Builder-built plugin
// type-asserts for this interface at Install time and fails closed when the
// host lacks it -- a declared customization is never silently dropped.
//
// Skill content has a single owner: a second customizing plugin, a FailOpen
// declaration, or a SkillsOverlay that cannot compose aborts startup
// unconditionally. EmbeddedSkills is a distribution build-integrity boundary:
// silently dropping it could republish host defaults the distribution removed.
// Removing a skill drops it from skills list/read and from structured
// framework-owned pointers, but does not disable any command; use Restrict to
// actually block a command.
type EmbeddedSkillsRegistrar interface {
	// EmbeddedSkills contributes a SkillsOverlay customizing the CLI's
	// embedded skill content (see SkillsOverlay).
	EmbeddedSkills(spec *SkillsOverlay)
}
