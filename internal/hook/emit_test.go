// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

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
