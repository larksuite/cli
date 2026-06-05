// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
)

// fakeViewSource is a minimal CommandView for tests -- it ignores the
// cobra command and returns a fixed view.
type fakeViewSource struct{ view platform.CommandView }

func (f fakeViewSource) View(*cobra.Command) platform.CommandView { return f.view }

type fakeView struct {
	path string
	risk string
}

func (v fakeView) Path() string                     { return v.path }
func (v fakeView) Domain() string                   { return "" }
func (v fakeView) Risk() (platform.Risk, bool)      { return platform.Risk(v.risk), v.risk != "" }
func (v fakeView) Identities() []platform.Identity  { return nil }
func (v fakeView) Annotation(string) (string, bool) { return "", false }

func makeLeaf(use string) *cobra.Command {
	return &cobra.Command{Use: use, RunE: func(*cobra.Command, []string) error { return nil }}
}

// Observers fire on Before AND After even when RunE returns an error.
// This is the failure-path observability contract -- After must always
// run so audit hooks see completion regardless of outcome.
func TestInstall_observersBeforeAndAfterAlwaysRun(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	leaf := &cobra.Command{Use: "+x", RunE: func(*cobra.Command, []string) error {
		return errors.New("boom")
	}}
	root.AddCommand(leaf)

	reg := hook.NewRegistry()

	var seen []string
	reg.AddObserver(hook.ObserverEntry{
		Name: "before", When: platform.Before, Selector: platform.All(),
		Fn: func(_ context.Context, inv platform.Invocation) {
			seen = append(seen, fmt.Sprintf("before:err=%v", inv.Err()))
		},
	})
	reg.AddObserver(hook.ObserverEntry{
		Name: "after", When: platform.After, Selector: platform.All(),
		Fn: func(_ context.Context, inv platform.Invocation) {
			seen = append(seen, fmt.Sprintf("after:err=%v", inv.Err()))
		},
	})

	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+x"}})

	err := leaf.RunE(leaf, nil)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected RunE to return original error, got %v", err)
	}

	wantBefore := "before:err=<nil>" // before fires with Err still nil
	wantAfter := "after:err=boom"    // after sees the failed RunE error
	if len(seen) != 2 || seen[0] != wantBefore || seen[1] != wantAfter {
		t.Fatalf("observer ordering / Err propagation broken, got %v", seen)
	}
}

// Wrap chain composes outermost-first (registration order). A regression
// that inverts the composition would change which Wrapper short-circuits
// first for safety-sensitive layers.
func TestInstall_wrapperChainOrder(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	var order []string
	leaf := &cobra.Command{Use: "+x", RunE: func(*cobra.Command, []string) error {
		order = append(order, "RunE")
		return nil
	}}
	root.AddCommand(leaf)

	reg := hook.NewRegistry()
	reg.AddWrapper(hook.WrapperEntry{
		Name: "outer", Selector: platform.All(),
		Fn: func(next platform.Handler) platform.Handler {
			return func(ctx context.Context, inv platform.Invocation) error {
				order = append(order, "outer-before")
				err := next(ctx, inv)
				order = append(order, "outer-after")
				return err
			}
		},
	})
	reg.AddWrapper(hook.WrapperEntry{
		Name: "inner", Selector: platform.All(),
		Fn: func(next platform.Handler) platform.Handler {
			return func(ctx context.Context, inv platform.Invocation) error {
				order = append(order, "inner-before")
				err := next(ctx, inv)
				order = append(order, "inner-after")
				return err
			}
		},
	})

	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+x"}})
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	want := []string{"outer-before", "inner-before", "RunE", "inner-after", "outer-after"}
	if !equalStrings(order, want) {
		t.Fatalf("Wrapper order = %v, want %v", order, want)
	}
}

// Denial guard physical isolation: the most safety-critical invariant.
// A denied command must NEVER reach a Wrap chain. We register a Wrap
// that, given the chance, would silently allow the call (return nil,
// don't call next, no AbortError). The guard must skip Wrap entirely
// so the denyStub's error reaches the caller.
//
// Without this guarantee, any plugin Wrap matching All() could
// bypass user policy / strict-mode denials.
func TestInstall_denialGuard_physicalIsolation(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	denyStubCalled := false
	leaf := &cobra.Command{
		Use: "+forbidden",
		RunE: func(*cobra.Command, []string) error {
			denyStubCalled = true
			return errors.New("CommandPruned: this is the denyStub")
		},
		Annotations: map[string]string{
			"lark:policy_denied_layer":  "policy",
			"lark:policy_denied_source": "yaml",
		},
	}
	root.AddCommand(leaf)

	reg := hook.NewRegistry()

	maliciousWrapCalled := false
	reg.AddWrapper(hook.WrapperEntry{
		Name: "malicious", Selector: platform.All(),
		Fn: func(next platform.Handler) platform.Handler {
			return func(ctx context.Context, inv platform.Invocation) error {
				maliciousWrapCalled = true
				return nil // suppress the denial
			}
		},
	})

	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+forbidden"}})

	err := leaf.RunE(leaf, nil)
	if maliciousWrapCalled {
		t.Errorf("denial guard violated: Wrap was invoked on a denied command")
	}
	if !denyStubCalled {
		t.Errorf("denyStub (original RunE) should still run on the denial path")
	}
	if err == nil {
		t.Fatalf("denyStub error must propagate, got nil")
	}
}

// Observer panics must not break the main flow. The guard converts the
// panic to a stderr warning and continues; the command still runs.
func TestInstall_observerPanicIsolated(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	runECalled := false
	leaf := &cobra.Command{Use: "+x", RunE: func(*cobra.Command, []string) error {
		runECalled = true
		return nil
	}}
	root.AddCommand(leaf)

	reg := hook.NewRegistry()
	reg.AddObserver(hook.ObserverEntry{
		Name: "buggy", When: platform.Before, Selector: platform.All(),
		Fn: func(context.Context, platform.Invocation) {
			panic("plugin author wrote bad code")
		},
	})

	// Capture stderr to make sure the warning was emitted. Restore the
	// previous sink so a subsequent test isn't stuck writing into our
	// discarded buffer.
	t.Cleanup(hook.SetStderrForTesting(&bytes.Buffer{})) // discard

	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+x"}})
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("RunE should still succeed when an Observer panicked, got %v", err)
	}
	if !runECalled {
		t.Errorf("RunE must execute despite Observer panic")
	}
}

// A Wrapper returning AbortError surfaces as *output.ExitError with
// type="hook" so cmd/root.go's envelope writer can serialise it.
func TestInstall_abortErrorBecomesExitError(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	leaf := makeLeaf("+x")
	root.AddCommand(leaf)

	reg := hook.NewRegistry()
	reg.AddWrapper(hook.WrapperEntry{
		Name: "rejecter", Selector: platform.All(),
		Fn: func(_ platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error {
				return &platform.AbortError{
					HookName: "rejecter",
					Reason:   "policy says no",
				}
			}
		},
	})

	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+x"}})

	err := leaf.RunE(leaf, nil)
	if err == nil {
		t.Fatalf("Wrap aborted; expected error")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("AbortError must convert to *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "hook" {
		t.Errorf("envelope type = %q, want hook", exitErr.Detail.Type)
	}
	detail := exitErr.Detail.Detail.(map[string]any)
	if detail["reason_code"] != "aborted" || detail["hook_name"] != "rejecter" {
		t.Errorf("detail = %+v", detail)
	}
	// The original AbortError must still be reachable via errors.As.
	var ab *platform.AbortError
	if !errors.As(err, &ab) {
		t.Errorf("error chain should expose *platform.AbortError")
	}
}

// namespacedWrap must not mutate a shared *AbortError. A plugin author
// might construct a sentinel at package scope and return it from
// multiple Wrap invocations; mutating it would let attribution leak
// across concurrent command runs and would also race.
//
// Production path test: drive a real cobra.Command through Install
// so namespacedWrap inside install.go is exercised. The plugin returns
// the same sentinel pointer twice. Both observed envelopes must have
// the framework-namespaced HookName, but the sentinel's own HookName
// must remain whatever the plugin originally set.
func TestInstall_namespacedWrap_doesNotMutateSentinel(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	leafA := makeLeaf("+a")
	leafB := makeLeaf("+b")
	root.AddCommand(leafA)
	root.AddCommand(leafB)

	sentinel := &platform.AbortError{HookName: "sentinel-original", Reason: "no"}

	reg := hook.NewRegistry()
	// Two Wrappers, different namespaced names, return the SAME
	// sentinel.
	reg.AddWrapper(hook.WrapperEntry{
		Name:     "plugin-a.wrap",
		Selector: platform.ByCommandPath("+a"),
		Fn: func(platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error { return sentinel }
		},
	})
	reg.AddWrapper(hook.WrapperEntry{
		Name:     "plugin-b.wrap",
		Selector: platform.ByCommandPath("+b"),
		Fn: func(platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error { return sentinel }
		},
	})

	hook.Install(root, reg, fakeViewSourceByPath{})

	// Invoke both leaves.
	errA := leafA.RunE(leafA, nil)
	errB := leafB.RunE(leafB, nil)

	// Sentinel must remain untouched: the framework must copy before
	// rewriting HookName.
	if sentinel.HookName != "sentinel-original" {
		t.Errorf("sentinel AbortError was mutated: HookName = %q", sentinel.HookName)
	}

	// Each invocation's envelope must carry the correct namespace --
	// proving the framework DID set the right name on its own copy.
	checkHookName(t, errA, "plugin-a.wrap")
	checkHookName(t, errB, "plugin-b.wrap")
}

// fakeViewSourceByPath returns a CommandView whose Path matches the
// leaf's Use field (so ByCommandPath selectors discriminate).
type fakeViewSourceByPath struct{}

func (fakeViewSourceByPath) View(c *cobra.Command) platform.CommandView {
	return fakeView{path: c.Use}
}

func checkHookName(t *testing.T, err error, want string) {
	t.Helper()
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError, got %T", err)
	}
	detail := exitErr.Detail.Detail.(map[string]any)
	if detail["hook_name"] != want {
		t.Errorf("hook_name = %v, want %v", detail["hook_name"], want)
	}
}

// A Before observer mutating inv.Args() must not affect what the
// original RunE sees: pins the slice-level read-only contract.
func TestInstall_argsNotMutableByObserver(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}

	var seenByRunE []string
	leaf := &cobra.Command{
		Use: "+echo",
		RunE: func(_ *cobra.Command, args []string) error {
			seenByRunE = append([]string(nil), args...)
			return nil
		},
	}
	root.AddCommand(leaf)

	reg := hook.NewRegistry()
	reg.AddObserver(hook.ObserverEntry{
		Name: "tamper", When: platform.Before, Selector: platform.All(),
		Fn: func(_ context.Context, inv platform.Invocation) {
			got := inv.Args()
			if len(got) > 0 {
				got[0] = "HIJACKED"
			}
		},
	})
	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+echo"}})

	originalArgs := []string{"hello", "world"}
	if err := leaf.RunE(leaf, originalArgs); err != nil {
		t.Fatalf("RunE returned %v", err)
	}
	if !equalStrings(seenByRunE, originalArgs) {
		t.Fatalf("RunE saw mutated args: got %v, want %v", seenByRunE, originalArgs)
	}
	if originalArgs[0] != "hello" {
		t.Fatalf("caller's original args were mutated: %v", originalArgs)
	}
}

// Root command (no parent) must never be wrapped -- it dispatches help
// and other framework concerns. The root has no RunE so we instead
// verify the root's children are wrapped while the root itself remains
// untouched (RunE stays nil).
func TestInstall_rootStaysUntouched(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	leaf := makeLeaf("+x")
	root.AddCommand(leaf)
	reg := hook.NewRegistry()
	hook.Install(root, reg, fakeViewSource{view: fakeView{path: "+x"}})
	if root.RunE != nil {
		t.Fatalf("root.RunE should remain nil after Install")
	}
	if leaf.RunE == nil {
		t.Fatalf("child leaf.RunE must remain non-nil (wrapped)")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
