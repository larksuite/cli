// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/output"
)

// extensionTypedError models a plugin-defined typed error. The SDK cannot
// safely clone fields it does not own, so LifecycleContext documents that this
// shape is shared and must be treated as read-only.
type extensionTypedError struct {
	*errs.Problem
	PrivateState []string
}

// A Startup handler returning a regular error must surface as a typed
// *LifecycleError with Panic=false so the cmd-layer guard can pick
// reason_code=lifecycle_failed.
func TestEmit_StartupHandlerError_TypedError(t *testing.T) {
	reg := NewRegistry()
	want := errors.New("backend down")
	reg.AddLifecycle(LifecycleEntry{
		Event: platform.Startup,
		Name:  "p.boot",
		Fn:    func(context.Context, *platform.LifecycleContext) error { return want },
	})

	got := Emit(context.Background(), reg, platform.Startup, nil)
	if got == nil {
		t.Fatal("expected error from Emit, got nil")
	}
	var le *LifecycleError
	if !errors.As(got, &le) {
		t.Fatalf("expected *LifecycleError, got %T %v", got, got)
	}
	if le.Panic {
		t.Errorf("Panic = true, want false (returned error)")
	}
	if le.HookName != "p.boot" {
		t.Errorf("HookName = %q, want p.boot", le.HookName)
	}
	if !errors.Is(got, want) {
		t.Errorf("unwrap should reach original error")
	}
}

// A Startup handler that panics must be recovered and surface as a
// typed *LifecycleError with Panic=true so the cmd-layer guard can
// pick reason_code=lifecycle_panic.
func TestEmit_StartupHandlerPanic_TypedError(t *testing.T) {
	reg := NewRegistry()
	reg.AddLifecycle(LifecycleEntry{
		Event: platform.Startup,
		Name:  "p.boot",
		Fn:    func(context.Context, *platform.LifecycleContext) error { panic("boom") },
	})

	got := Emit(context.Background(), reg, platform.Startup, nil)
	if got == nil {
		t.Fatal("expected error from Emit, got nil")
	}
	var le *LifecycleError
	if !errors.As(got, &le) {
		t.Fatalf("expected *LifecycleError, got %T %v", got, got)
	}
	if !le.Panic {
		t.Errorf("Panic = false, want true (recovered panic)")
	}
	if le.HookName != "p.boot" {
		t.Errorf("HookName = %q, want p.boot", le.HookName)
	}
}

// A Startup handler that succeeds returns nil; subsequent handlers run.
func TestEmit_StartupAllHandlersRun(t *testing.T) {
	reg := NewRegistry()
	var calls []string
	reg.AddLifecycle(LifecycleEntry{
		Event: platform.Startup, Name: "a",
		Fn: func(context.Context, *platform.LifecycleContext) error {
			calls = append(calls, "a")
			return nil
		},
	})
	reg.AddLifecycle(LifecycleEntry{
		Event: platform.Startup, Name: "b",
		Fn: func(context.Context, *platform.LifecycleContext) error {
			calls = append(calls, "b")
			return nil
		},
	})
	if err := Emit(context.Background(), reg, platform.Startup, nil); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if len(calls) != 2 || calls[0] != "a" || calls[1] != "b" {
		t.Errorf("handlers fired in unexpected order: %v", calls)
	}
}

// Shutdown handler errors are logged, not propagated; Emit returns nil.
func TestEmit_ShutdownErrorsSwallowed(t *testing.T) {
	reg := NewRegistry()
	reg.AddLifecycle(LifecycleEntry{
		Event: platform.Shutdown, Name: "flush",
		Fn: func(context.Context, *platform.LifecycleContext) error {
			return errors.New("flush failed")
		},
	})
	if err := Emit(context.Background(), reg, platform.Shutdown, nil); err != nil {
		t.Errorf("Shutdown errors must NOT propagate, got: %v", err)
	}
}

func TestCopyLifecycleErr(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := copyLifecycleErr(nil); got != nil {
			t.Fatalf("copyLifecycleErr(nil) = %v, want nil", got)
		}
	})

	t.Run("owned typed error", func(t *testing.T) {
		cause := errors.New("invalid input")
		original := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad value").
			WithParam("--value").
			WithCause(cause)
		got, ok := copyLifecycleErr(original).(*errs.ValidationError)
		if !ok {
			t.Fatalf("copy = %T, want *errs.ValidationError", copyLifecycleErr(original))
		}
		if got == original {
			t.Fatal("typed error was shared instead of cloned")
		}
		if got.Param != original.Param || !errors.Is(got, cause) {
			t.Errorf("clone lost fields or cause: %+v", got)
		}
		got.Message = "changed"
		if original.Message == got.Message {
			t.Fatal("mutating the clone changed the producer's error")
		}
	})

	t.Run("bare signal", func(t *testing.T) {
		original := output.ErrBare(7)
		got, ok := copyLifecycleErr(original).(*output.BareError)
		if !ok || got == original || got.Code != original.Code {
			t.Fatalf("copy = %#v, want distinct BareError with code %d", got, original.Code)
		}
		got.Code = 0
		if original.Code != 7 {
			t.Fatal("mutating the BareError clone changed the producer's signal")
		}
	})

	t.Run("partial failure signal", func(t *testing.T) {
		original := output.PartialFailure(8)
		got, ok := copyLifecycleErr(original).(*output.PartialFailureError)
		if !ok || got == original || got.Code != original.Code {
			t.Fatalf("copy = %#v, want distinct PartialFailureError with code %d", got, original.Code)
		}
		got.Code = 0
		if original.Code != 8 {
			t.Fatal("mutating the PartialFailureError clone changed the producer's signal")
		}
	})

	t.Run("extension typed error is read-only pass-through", func(t *testing.T) {
		original := &extensionTypedError{
			Problem: &errs.Problem{
				Category: errs.CategoryNetwork,
				Subtype:  errs.SubtypeNetworkTimeout,
				Message:  "extension timeout",
			},
			PrivateState: []string{"opaque"},
		}
		if got := copyLifecycleErr(original); got != error(original) {
			t.Fatalf("copy = %T %v, want exact extension error pass-through", got, got)
		}
	})

	// A wrapper is part of what the command returned. Copying the typed error
	// found inside it would hand the handler a shorter message and break the
	// errors.Is the wrapper exists to support, so a wrapped chain is passed
	// through whole.
	t.Run("wrapped typed error keeps its wrapper", func(t *testing.T) {
		sentinel := errors.New("plugin backend unavailable")
		typed := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad value")
		original := fmt.Errorf("plugin %q: %w: %w", "backend", sentinel, typed)

		got := copyLifecycleErr(original)
		if got != original {
			t.Fatalf("copy = %T %q, want the wrapped chain shared as-is", got, got)
		}
		if !errors.Is(got, sentinel) {
			t.Error("copy no longer reaches the wrapper's sentinel")
		}
		if problem, ok := errs.ProblemOf(got); !ok ||
			problem.Subtype != errs.SubtypeInvalidArgument {
			t.Errorf("copy = %v, want the wrapped classification still readable", got)
		}
	})

	t.Run("wrapped exit signal keeps its wrapper", func(t *testing.T) {
		bare := output.ErrBare(7)
		original := fmt.Errorf("plugin short-circuit: %w", bare)

		got := copyLifecycleErr(original)
		if got != original {
			t.Fatalf("copy = %T %q, want the wrapped chain shared as-is", got, got)
		}
		if output.ExitCodeOf(got) != 7 {
			t.Errorf("exit code = %d, want 7 still readable through the wrapper",
				output.ExitCodeOf(got))
		}
	})

	t.Run("typed nil pointer", func(t *testing.T) {
		var bare *output.BareError
		if got := copyLifecycleErr(error(bare)); got != error(bare) {
			t.Fatalf("copy = %#v, want the nil-pointer value passed through untouched", got)
		}
	})
}
