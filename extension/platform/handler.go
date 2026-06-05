// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "context"

// Handler is the inner function shape every Wrapper composes. It IS the
// "command business logic" from the Wrapper's perspective -- calling
// next(ctx, inv) inside a Wrapper means "let the command proceed";
// returning early without calling next short-circuits.
type Handler func(ctx context.Context, inv Invocation) error

// Observer is a side-effect-only command hook. No return value, no
// next-chain control: an Observer can read Invocation but cannot prevent
// the command from running. Used for audit, metrics, and completion
// logs. After-stage Observers fire even when the command failed
// (Invocation.Err() is populated in that case).
type Observer func(ctx context.Context, inv Invocation)

// Wrapper is a middleware-style hook: it receives the rest of the
// handler chain and returns a wrapped version. The Wrapper decides
// whether to call next (allow), abstain (deny, return an AbortError),
// or transform the result. Multiple Wrappers compose left-to-right by
// registration order; the outermost runs first.
//
// ⚠️ IMPORTANT: The factory function `func(next Handler) Handler` is
// invoked ONCE PER COMMAND DISPATCH, not once at plugin install. This
// lets the framework recover from a panicking factory and convert it
// to a structured envelope, but it means any state captured by the
// outer closure is rebuilt on every command. Long-lived state (HTTP
// clients, caches, metrics counters) MUST live on the Plugin struct
// or in package-level variables, never in factory-local captures.
type Wrapper func(next Handler) Handler

// LifecycleHandler runs at one of the process-level LifecycleEvent
// slots. The handler may use ctx for cancellation; in the Shutdown
// case the framework supplies a context with a 2-second hard deadline.
type LifecycleHandler func(ctx context.Context, lc *LifecycleContext) error
