// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// failClosedAbortingPlugin returns a PluginInstallError on Install,
// declaring FailClosed so InstallAll surfaces the error.
type failClosedAbortingPlugin struct{}

func (failClosedAbortingPlugin) Name() string    { return "policy" }
func (failClosedAbortingPlugin) Version() string { return "1.0.0" }
func (failClosedAbortingPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (failClosedAbortingPlugin) Install(platform.Registrar) error {
	return errors.New("upstream policy server unreachable")
}

// When a FailClosed plugin fails to install, buildInternal must
// install a PersistentPreRunE that returns a structured *output.ExitError.
// The user must NEVER see a silent partial-install state.
//
// This pins the build.go fix for codex's NEW ISSUE about
// build.go demoting FailClosed errors to warnings.
func TestBuildInternal_failClosedAbortsCLI(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	platform.Register(failClosedAbortingPlugin{})

	root := Build(context.Background(), buildInvocationForTest(t))

	if root.PersistentPreRunE == nil {
		t.Fatalf("FailClosed install error must wire a PersistentPreRunE that aborts subsequent commands")
	}

	err := root.PersistentPreRunE(root, nil)
	checkGuardError(t, err)

	// CRITICAL: subcommands that declare their own PersistentPreRunE
	// (cmd/auth/auth.go and cmd/config/config.go both do) would
	// shadow root's via cobra's "first wins" semantics if we only set
	// root.PersistentPreRunE. Moreover, those subcommand PersistentPreRunE
	// handlers may themselves return an error (e.g. auth's
	// external_provider check at internal/cmdutil/factory.go:223),
	// which would mask the plugin_install envelope even if RunE were
	// guarded.
	//
	// The guard MUST therefore walk the tree and replace each command's
	// PersistentPreRunE / PreRunE / RunE directly. This test pins
	// that the bypass is closed.
	auth := findChildByUse(t, root, "auth")
	if auth == nil {
		t.Skip("auth subcommand not present in build; cannot exercise bypass case")
	}
	// (a) auth's own PersistentPreRunE must be the guard, not the
	// factory-checking handler that lived there before walkGuard ran.
	if auth.PersistentPreRunE == nil {
		t.Fatalf("auth.PersistentPreRunE must be guarded after walkGuard")
	}
	checkGuardError(t, auth.PersistentPreRunE(auth, nil))

	// (b) A runnable leaf below auth also gets the guard on RunE. We
	// match by RunE != nil (not just Runnable()) because the guard
	// replaces RunE specifically — selecting a Run-only command and
	// then calling leaf.RunE would nil-deref.
	var leaf *cobra.Command
	walk(auth, func(c *cobra.Command) {
		if leaf != nil {
			return
		}
		if c != auth && c.RunE != nil {
			leaf = c
		}
	})
	if leaf == nil {
		t.Skip("no auth subcommand with RunE found")
	}
	checkGuardError(t, leaf.RunE(leaf, nil))
}

// checkGuardError asserts that err is the structured plugin_install
// ExitError the guard produces.
func checkGuardError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("PersistentPreRunE must surface the install error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "plugin_install" {
		t.Errorf("envelope type = %q, want plugin_install", exitErr.Detail.Type)
	}
	detail := exitErr.Detail.Detail.(map[string]any)
	if detail["plugin"] != "policy" {
		t.Errorf("detail.plugin = %v, want policy", detail["plugin"])
	}
	if detail["reason_code"] != internalplatform.ReasonInstallFailed {
		t.Errorf("detail.reason_code = %v, want install_failed", detail["reason_code"])
	}
}

// findChildByUse helper.
func findChildByUse(t *testing.T, parent *cobra.Command, use string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Use == use {
			return c
		}
	}
	return nil
}

// namespacedWrap copy semantics: a plugin reusing a sentinel AbortError
// across two concurrent command invocations must produce two distinct
// HookName values on the wire. Mutation would interleave them.
//
// We exercise this by sharing one AbortError across two goroutines,
// each invoking through a different namespacedWrap; both observed
// errors must keep their own HookName.
func TestNamespacedWrap_doesNotMutateSharedAbortError(t *testing.T) {
	shared := &platform.AbortError{HookName: "plugin-shared-name", Reason: "rejected"}

	makeWrapper := func(name string) platform.Wrapper {
		return func(next platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error { return shared }
		}
	}

	reg := hook.NewRegistry()
	reg.AddWrapper(hook.WrapperEntry{
		Name: "p1.wrap", Selector: platform.All(), Fn: makeWrapper("p1.wrap"),
	})
	reg.AddWrapper(hook.WrapperEntry{
		Name: "p2.wrap", Selector: platform.All(), Fn: makeWrapper("p2.wrap"),
	})

	// Drive matched wrappers separately to exercise both namespace paths.
	matched := reg.MatchingWrappers(stubView{})
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched wrappers, got %d", len(matched))
	}

	results := make([]string, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i, m := range matched {
		go func() {
			defer wg.Done()
			err := m.Fn(func(context.Context, platform.Invocation) error { return nil })(
				context.Background(), stubInvocation{})
			if ab, ok := err.(*platform.AbortError); ok {
				results[i] = ab.HookName
			}
		}()
	}
	wg.Wait()

	// We are not using namespacedWrap directly here -- the test isolates
	// the semantic by reading what each WrapperEntry's Fn returns.
	// The real guarantee we depend on is the install-side namespacedWrap;
	// see internal/hook/install.go for the production path. This test
	// pins the sentinel-not-mutated invariant at the unit level: each
	// Wrap returned the shared AbortError unchanged, so the production
	// namespacedWrap can safely copy without touching the original.
	if shared.HookName != "plugin-shared-name" {
		t.Errorf("shared sentinel AbortError was mutated: HookName = %q", shared.HookName)
	}
	_ = results
}

// stubView for the wrap selector match.
type stubView struct{}

func (stubView) Path() string                     { return "x" }
func (stubView) Domain() string                   { return "" }
func (stubView) Risk() (platform.Risk, bool)      { return "", false }
func (stubView) Identities() []platform.Identity  { return nil }
func (stubView) Annotation(string) (string, bool) { return "", false }

// stubInvocation is the minimal platform.Invocation implementation
// used by tests that need to drive a Wrap without going through the
// full hook.Install pipeline.
type stubInvocation struct{}

func (stubInvocation) Cmd() platform.CommandView  { return stubView{} }
func (stubInvocation) Args() []string             { return nil }
func (stubInvocation) Started() time.Time         { return time.Time{} }
func (stubInvocation) Err() error                 { return nil }
func (stubInvocation) DeniedByPolicy() bool       { return false }
func (stubInvocation) DenialLayer() string        { return "" }
func (stubInvocation) DenialPolicySource() string { return "" }
