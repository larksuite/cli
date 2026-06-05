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
