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

// LifecycleContext is passed to LifecycleHandler. When Event == Shutdown, Err
// is the failure the invocation ended with — from the command itself, or from
// the framework rejecting the command line before any command ran; otherwise
// nil.
//
// Err is the error the command returned, with its wrapping intact, so
// errors.Is and errors.As reach whatever the producer put in the chain. It
// carries the same Category and Subtype the CLI wrote to its stderr envelope.
//
// What the user receives is settled before this event fires, so writing to Err
// cannot change it. Where the SDK can copy the error it hands each handler its
// own value, so one handler cannot change what the next one observes; where it
// cannot — a wrapped chain, or a type defined outside the SDK — the value is
// shared. Treat Err as read-only and that distinction stops mattering.
//
// Read it with errs.ProblemOf and check the boolean — two exit-code-only
// signals carry no Problem and write no envelope, because their result is
// already on stdout: a partial failure, and a bare predicate exit.
//
// Some failures end the process before this event can be emitted, so a handler
// must not be relied on as an exhaustive audit trail. Bootstrap rejections, a
// plugin whose own installation or Startup handler failed, and shell-completion
// invocations all exit without a Shutdown event. So does a failure to render
// help or usage text from cobra's own help command, which ends the process on
// the spot rather than returning.
type LifecycleContext struct {
	Event LifecycleEvent
	Err   error
}
