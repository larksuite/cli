// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/extension/platform"
)

// shutdownDeadline is the hard upper bound on how long Shutdown
// handlers in total may run. Past this, the framework returns control
// to the caller regardless of unfinished handlers. 2s matches the
// design-doc constraint.
const shutdownDeadline = 2 * time.Second

// LifecycleError is the typed failure returned by Emit for non-Shutdown
// events when a LifecycleHandler returns an error or panics. Callers can
// errors.As to extract HookName, Event, and the Panic discriminator
// (panic vs returned error) so the envelope writer can produce
// distinct reason_code values:
//
//   - Panic == false -> reason_code = "lifecycle_failed"
//   - Panic == true  -> reason_code = "lifecycle_panic"
//
// Shutdown handler failures are logged inside emitShutdown and never
// returned through this type (Shutdown is non-recoverable; the contract
// is "best effort, never block exit").
type LifecycleError struct {
	Event    platform.LifecycleEvent
	HookName string
	Panic    bool
	Cause    error
}

func (e *LifecycleError) Error() string {
	kind := "failed"
	if e.Panic {
		kind = "panic"
	}
	return fmt.Sprintf("lifecycle hook %q %s: %v", e.HookName, kind, e.Cause)
}

func (e *LifecycleError) Unwrap() error { return e.Cause }

// Emit fires every LifecycleHandler registered for event in
// registration order. lastErr is propagated to handlers via
// LifecycleContext.Err (typical use: Shutdown handlers see the error
// the command exited with).
//
// Behaviour by event:
//
//   - Startup: any handler returning a non-nil error aborts the
//     bootstrap (caller decides whether to fail-closed). The first
//     such error is returned as *LifecycleError.
//
//   - Shutdown: handler errors are logged but do not affect the
//     returned error; the framework also caps the total time at
//     shutdownDeadline.
func Emit(ctx context.Context, reg *Registry, event platform.LifecycleEvent, lastErr error) error {
	if reg == nil {
		return nil
	}
	handlers := reg.LifecycleHandlers(event)
	if len(handlers) == 0 {
		return nil
	}
	lc := &platform.LifecycleContext{Event: event, Err: lastErr}

	if event == platform.Shutdown {
		return emitShutdown(ctx, handlers, lc)
	}
	for _, h := range handlers {
		if err := callLifecycleSafe(ctx, h, lc); err != nil {
			return err
		}
	}
	return nil
}

// emitShutdown enforces the 2-second total deadline. Handlers receive
// a derived context with the remaining budget; once the budget is
// exhausted, the remaining handlers are skipped (with a stderr
// warning) and Emit returns.
func emitShutdown(parent context.Context, handlers []LifecycleEntry, lc *platform.LifecycleContext) error {
	ctx, cancel := context.WithTimeout(parent, shutdownDeadline)
	defer cancel()
	deadline := time.Now().Add(shutdownDeadline)

	for _, h := range handlers {
		if time.Now().After(deadline) {
			fmt.Fprintf(stderr(), "warning: shutdown deadline exceeded; skipping hook %q\n", h.Name)
			continue
		}
		if err := callLifecycleSafe(ctx, h, lc); err != nil {
			// Shutdown errors are logged, not propagated -- exit is
			// non-recoverable anyway.
			fmt.Fprintf(stderr(), "warning: shutdown hook %q: %v\n", h.Name, err)
		}
	}
	return nil
}

// callLifecycleSafe invokes a LifecycleHandler with panic recovery.
// Returns *LifecycleError with Panic=true on recovered panic, Panic=false
// on a regular returned error. nil if the handler succeeded.
func callLifecycleSafe(ctx context.Context, h LifecycleEntry, lc *platform.LifecycleContext) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &LifecycleError{
				Event:    lc.Event,
				HookName: h.Name,
				Panic:    true,
				Cause:    fmt.Errorf("%v", r),
			}
		}
	}()
	if e := h.Fn(ctx, lc); e != nil {
		return &LifecycleError{
			Event:    lc.Event,
			HookName: h.Name,
			Panic:    false,
			Cause:    e,
		}
	}
	return nil
}
