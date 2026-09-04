// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
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
	if event == platform.Shutdown {
		return emitShutdown(ctx, handlers, event, lastErr)
	}
	for _, h := range handlers {
		if err := callLifecycleSafe(ctx, h, newLifecycleContext(event, lastErr)); err != nil {
			return err
		}
	}
	return nil
}

// newLifecycleContext builds one handler's own context. Typed error fields are
// exported, so handing every handler the same value would let the first one
// observing a failure change what the rest of them see. Each handler therefore
// gets its own context, and its own copy of the error wherever the value can be
// copied.
func newLifecycleContext(event platform.LifecycleEvent, lastErr error) *platform.LifecycleContext {
	return &platform.LifecycleContext{Event: event, Err: copyLifecycleErr(lastErr)}
}

// copyLifecycleErr returns an independent value when err is itself one of the
// error shapes this module owns.
//
// Everything else is shared as-is, and a chain that merely wraps an owned shape
// counts as everything else: the value inside is not the error the command
// returned, so substituting it would drop the wrapper's message and any
// sentinel errors.Is could reach through it. A type defined outside these
// shapes cannot be copied at all without reflecting over fields we do not know.
// LifecycleContext documents both limits and asks handlers to treat Err as
// read-only.
func copyLifecycleErr(err error) error {
	if err == nil {
		return nil
	}
	if !ownsErrorValue(err) {
		return err
	}
	if clone, ok := recovery.CloneTyped(err); ok {
		return clone
	}
	// A typed nil pointer held in a non-nil interface has nothing to copy;
	// recovery.CloneTyped reports the same for the typed errors it owns.
	switch signal := err.(type) { //nolint:errorlint // deliberate: see ownsErrorValue
	case *output.BareError:
		if signal == nil {
			return err
		}
		clone := *signal
		return &clone
	case *output.PartialFailureError:
		if signal == nil {
			return err
		}
		clone := *signal
		return &clone
	}
	return err
}

// ownsErrorValue reports whether err is itself one of the owned shapes rather
// than a chain wrapping one. The copy helpers locate their target with
// errors.As and recovery.CloneTyped, both of which search the whole chain, so
// they are only safe to apply once err has been shown to be that target.
//
//nolint:errorlint // asserting on err is the point: what err is, not what it wraps.
func ownsErrorValue(err error) bool {
	switch err.(type) {
	case *output.BareError, *output.PartialFailureError, errs.TypedError:
		return true
	}
	return false
}

// emitShutdown enforces the 2-second total deadline. Handlers receive
// a derived context with the remaining budget; once the budget is
// exhausted, the remaining handlers are skipped (with a stderr
// warning) and Emit returns.
func emitShutdown(parent context.Context, handlers []LifecycleEntry, event platform.LifecycleEvent, lastErr error) error {
	ctx, cancel := context.WithTimeout(parent, shutdownDeadline)
	defer cancel()
	deadline := time.Now().Add(shutdownDeadline)

	for _, h := range handlers {
		if time.Now().After(deadline) {
			fmt.Fprintf(stderr(), "warning: shutdown deadline exceeded; skipping hook %q\n", h.Name)
			continue
		}
		if err := callLifecycleSafe(ctx, h, newLifecycleContext(event, lastErr)); err != nil {
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
