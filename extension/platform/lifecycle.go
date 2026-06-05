// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// When selects the temporal slot for command-level Observer hooks. The
// framework wraps every command's RunE so both stages always fire, even
// when RunE itself returns an error (After is failure-safe).
type When int

const (
	// Before fires immediately before the command's business logic.
	Before When = iota

	// After fires after the command's business logic (or its denyStub
	// in the denied path). Always fires, even when RunE returned an
	// error; Invocation.Err is populated in that case.
	After
)

// LifecycleEvent selects the temporal slot for Lifecycle hooks. These are
// process-level events that fire once per binary execution, not per
// command. Only Startup and Shutdown are defined: additional bootstrap
// phases can be added later as a non-breaking addition if a concrete
// consumer surfaces.
type LifecycleEvent int

const (
	// Startup fires after plugin install has committed; Plugin.On
	// handlers for Startup are guaranteed to be registered before this
	// event is emitted (so they can receive it).
	Startup LifecycleEvent = iota

	// Shutdown fires once before the process exits. Handler total
	// execution is bounded by a hard 2s timeout to prevent a
	// misbehaving handler from holding up exit.
	Shutdown
)

// LifecycleContext is passed to LifecycleHandler. Err is the error from
// the preceding command (when Event == Shutdown after a failed RunE);
// otherwise nil.
type LifecycleContext struct {
	Event LifecycleEvent
	Err   error
}
