// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// These integration tests exercise the Hook framework's plumbing
// (Plugin -> InstallAll -> Registry -> wireHooks -> RunE wrapper)
// against a SYNTHETIC command tree, not the real lark-cli shortcut
// tree. The synthetic tree keeps the test hermetic -- invoking real
// shortcuts requires a fully-populated Factory (HTTP, credentials,
// etc.) which is out of scope for a hook plumbing test.
//
// The e2e tests that go through Build() are kept thin (see
// TestBuildInternal_appliesPolicyToRealTree in policy_test.go); they
// assert plumbing existence (Hidden flag, etc.) without invoking
// shortcuts.

type fakeIntegrationPlugin struct {
	name            string
	caps            platform.Capabilities
	rule            *platform.Rule
	beforeCount     int64
	afterCount      int64
	wrapCount       int64
	wrapDeniesWrite bool // when true, Wrap returns AbortError for risk=write
	shutdownCalled  int64
}

func (p *fakeIntegrationPlugin) Name() string                        { return p.name }
func (p *fakeIntegrationPlugin) Version() string                     { return "0.0.1" }
func (p *fakeIntegrationPlugin) Capabilities() platform.Capabilities { return p.caps }

func (p *fakeIntegrationPlugin) Install(r platform.Registrar) error {
	if p.caps.Restricts && p.rule != nil {
		r.Restrict(p.rule)
	}
	r.Observe(platform.Before, "audit-pre", platform.All(),
		func(context.Context, platform.Invocation) {
			atomic.AddInt64(&p.beforeCount, 1)
		})
	r.Observe(platform.After, "audit-post", platform.All(),
		func(context.Context, platform.Invocation) {
			atomic.AddInt64(&p.afterCount, 1)
		})
	r.Wrap("policy", platform.ByWrite(),
		func(next platform.Handler) platform.Handler {
			return func(ctx context.Context, inv platform.Invocation) error {
				atomic.AddInt64(&p.wrapCount, 1)
				if p.wrapDeniesWrite {
					return &platform.AbortError{
						HookName: "policy",
						Reason:   "writes blocked by integration test plugin",
					}
				}
				return next(ctx, inv)
			}
		})
	r.On(platform.Shutdown, "flush",
		func(context.Context, *platform.LifecycleContext) error {
			atomic.AddInt64(&p.shutdownCalled, 1)
			return nil
		})
	return nil
}

// syntheticTree builds a small command tree we own end-to-end. The leaf
// has risk=write so the Wrap's ByWrite() selector matches.
func syntheticTree() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "lark-cli"}
	group := &cobra.Command{Use: "docs"}
	root.AddCommand(group)
	leaf := &cobra.Command{
		Use:  "+write",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmdutil.SetRisk(leaf, "write")
	group.AddCommand(leaf)
	return root, leaf
}

// End-to-end through the public install pipeline: register a plugin,
// run internalplatform.InstallAll (the same function buildInternal calls),
// wire hooks onto a synthetic tree, invoke the leaf, and confirm
// observers fired.
func TestPluginPipeline_observersWired(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	plugin := &fakeIntegrationPlugin{
		name: "audit-plugin",
		caps: platform.Capabilities{FailurePolicy: platform.FailOpen},
	}
	platform.Register(plugin)

	result, err := internalplatform.InstallAll(platform.RegisteredPlugins(), nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}

	root, leaf := syntheticTree()
	if err := wireHooks(context.Background(), root, result.Registry); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	_ = leaf.RunE(leaf, nil)

	if got := atomic.LoadInt64(&plugin.beforeCount); got != 1 {
		t.Errorf("Before observer fired %d times, want 1", got)
	}
	if got := atomic.LoadInt64(&plugin.afterCount); got != 1 {
		t.Errorf("After observer fired %d times, want 1", got)
	}
	if got := atomic.LoadInt64(&plugin.wrapCount); got != 1 {
		t.Errorf("Wrap fired %d times (ByWrite matches risk=write), want 1", got)
	}
}

// A Wrapper returning AbortError on a write command must surface as
// type="hook" in the envelope so the caller can parse the structured
// rejection.
func TestPluginPipeline_wrapAbortReachesEnvelope(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	plugin := &fakeIntegrationPlugin{
		name:            "policy-plugin",
		caps:            platform.Capabilities{FailurePolicy: platform.FailOpen},
		wrapDeniesWrite: true,
	}
	platform.Register(plugin)

	result, err := internalplatform.InstallAll(platform.RegisteredPlugins(), nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}

	root, leaf := syntheticTree()
	if err := wireHooks(context.Background(), root, result.Registry); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	err = leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "hook" {
		t.Errorf("envelope type = %q, want hook", exitErr.Detail.Type)
	}
	detail := exitErr.Detail.Detail.(map[string]any)
	if detail["reason_code"] != "aborted" {
		t.Errorf("detail.reason_code = %v, want aborted", detail["reason_code"])
	}
	if detail["hook_name"] != "policy-plugin.policy" {
		t.Errorf("detail.hook_name = %v, want policy-plugin.policy", detail["hook_name"])
	}

	// errors.As must still reach the original AbortError so consumers
	// can inspect the typed cause.
	var ab *platform.AbortError
	if !errors.As(err, &ab) {
		t.Errorf("error chain should expose *platform.AbortError")
	}
}

// Plugin.Restrict() contribution must reach the pruning resolver and
// take precedence over a yaml file (single-rule, plugin wins). This
// goes through the REAL Build() pipeline so the wiring between
// installPluginsAndHooks -> applyUserPolicyPruning -> cmdpolicy.Resolve
// is covered.
func TestPluginPipeline_restrictBeatsYaml(t *testing.T) {
	cfgDir := tmpHome(t)
	// yaml says allow everything; plugin says deny everything. Plugin
	// should win and a command should be denied.
	if err := os.WriteFile(filepath.Join(cfgDir, "policy.yml"),
		[]byte("name: yaml-allow\nallow: [\"**\"]\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	plugin := &fakeIntegrationPlugin{
		name: "restricter",
		caps: platform.Capabilities{
			Restricts:     true,
			FailurePolicy: platform.FailClosed,
		},
		rule: &platform.Rule{Name: "deny-all", Deny: []string{"**"}},
	}
	platform.Register(plugin)

	root := Build(context.Background(), buildInvocationForTest(t))

	// At least one runnable command must end up Hidden because of the
	// plugin Restrict (yaml had been allow-all and would have left
	// everything visible).
	var foundHidden bool
	walk(root, func(c *cobra.Command) {
		if c.HasParent() && c.Runnable() && c.Hidden {
			foundHidden = true
		}
	})
	if !foundHidden {
		t.Fatalf("plugin Restrict should have denied at least one command despite yaml allow-all")
	}
}

// Denial-guard end-to-end: register a plugin with a Wrap that would
// SILENTLY suppress denial (return nil without calling next). After
// installing pruning (which marks a command as denied) and wiring
// hooks, calling the denied command must STILL produce the denial
// error -- the Wrap must never run on the denied path.
func TestPluginPipeline_denialGuardIntegrated(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	wrapCalled := false
	plugin := &fakeIntegrationPlugin{
		name:            "policy-plugin",
		caps:            platform.Capabilities{FailurePolicy: platform.FailOpen},
		wrapDeniesWrite: false, // wrap would normally allow
	}
	// Override Wrap with a malicious behavior: return nil (silence the
	// denial). We do this by wrapping the install: register a
	// second Wrap that suppresses errors.
	platform.Register(plugin)

	// Add another plugin with a malicious wrap.
	malicious := &mockMaliciousPlugin{
		name:        "malicious",
		invokedFlag: &wrapCalled,
	}
	platform.Register(malicious)

	result, err := internalplatform.InstallAll(platform.RegisteredPlugins(), nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}

	root, leaf := syntheticTree()
	// Simulate cmdpolicy.Apply marking leaf as denied.
	leaf.Hidden = true
	leaf.DisableFlagParsing = true
	if leaf.Annotations == nil {
		leaf.Annotations = map[string]string{}
	}
	leaf.Annotations["lark:policy_denied_layer"] = "policy"
	leaf.Annotations["lark:policy_denied_source"] = "plugin:other"
	denyStubCalled := false
	leaf.RunE = func(*cobra.Command, []string) error {
		denyStubCalled = true
		return errors.New("CommandPruned (denyStub)")
	}

	if err := wireHooks(context.Background(), root, result.Registry); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	err = leaf.RunE(leaf, nil)
	if wrapCalled {
		t.Errorf("denial guard violated: malicious Wrap ran on a denied command")
	}
	if !denyStubCalled {
		t.Errorf("denyStub should run on the denial path even when a Wrap is registered")
	}
	if err == nil {
		t.Errorf("denial error must propagate, got nil")
	}
}

// mockMaliciousPlugin registers a Wrap that returns nil unconditionally
// -- exactly the kind of plugin the denial guard defends against.
type mockMaliciousPlugin struct {
	name        string
	invokedFlag *bool
}

func (p *mockMaliciousPlugin) Name() string    { return p.name }
func (p *mockMaliciousPlugin) Version() string { return "0.0.1" }
func (p *mockMaliciousPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailOpen}
}
func (p *mockMaliciousPlugin) Install(r platform.Registrar) error {
	r.Wrap("hijack", platform.All(),
		func(_ platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error {
				if p.invokedFlag != nil {
					*p.invokedFlag = true
				}
				return nil // silence everything
			}
		})
	return nil
}

// Verifies buildInternal returns a non-nil *hook.Registry when a plugin
// is registered and Emit(Shutdown) on that registry fires the plugin's
// On(Shutdown) handler. This is the contract Execute relies on to fire
// Shutdown after rootCmd.Execute returns.
func TestBuildInternal_returnsRegistryForShutdownEmit(t *testing.T) {
	tmpHome(t)

	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	plugin := &fakeIntegrationPlugin{
		name: "shutdown-test",
		caps: platform.Capabilities{FailurePolicy: platform.FailOpen},
	}
	platform.Register(plugin)

	_, _, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg == nil {
		t.Fatalf("buildInternal returned nil registry; plugin's Shutdown handler is unreachable")
	}

	if err := hook.Emit(context.Background(), reg, platform.Shutdown, nil); err != nil {
		t.Fatalf("Emit(Shutdown): %v", err)
	}
	if got := atomic.LoadInt64(&plugin.shutdownCalled); got != 1 {
		t.Errorf("On(Shutdown) handler fired %d times, want 1", got)
	}
}

// When plugin install fails (FailClosed), buildInternal returns nil
// registry. Execute must nil-check before calling Emit so we don't fault
// on the FailClosed bypass-guard path.
func TestBuildInternal_failClosedYieldsNilRegistry(t *testing.T) {
	tmpHome(t)

	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	// A plugin that fails install and is FailClosed -> InstallAll
	// returns an error, buildInternal installs the guard and returns
	// early with nil registry.
	plugin := &failingPlugin{
		name: "fail-closed",
		caps: platform.Capabilities{FailurePolicy: platform.FailClosed},
		err:  errors.New("install failure simulated"),
	}
	platform.Register(plugin)

	_, _, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("buildInternal returned non-nil registry on FailClosed install error")
	}
}

type failingPlugin struct {
	name string
	caps platform.Capabilities
	err  error
}

func (p *failingPlugin) Name() string                        { return p.name }
func (p *failingPlugin) Version() string                     { return "0.0.1" }
func (p *failingPlugin) Capabilities() platform.Capabilities { return p.caps }
func (p *failingPlugin) Install(platform.Registrar) error    { return p.err }

// === Plugin Restrict conflict guard ===
//
// Two plugins both calling r.Restrict must surface as a structured
// plugin_conflict envelope (reason_code multiple_restrict_plugins) at
// dispatch time, NOT as a silent stderr warning. Otherwise a
// safety-sensitive operator could miss that their policy never took
// effect.
func TestPluginConflictGuard_MultipleRestrictAbortsCLI(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	rule := &platform.Rule{Name: "any", Allow: []string{"**"}}
	platform.Register(&fakeIntegrationPlugin{
		name: "plugin-a",
		caps: platform.Capabilities{Restricts: true, FailurePolicy: platform.FailClosed},
		rule: rule,
	})
	platform.Register(&fakeIntegrationPlugin{
		name: "plugin-b",
		caps: platform.Capabilities{Restricts: true, FailurePolicy: platform.FailClosed},
		rule: rule,
	})

	_, root, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("conflict guard path should yield nil registry")
	}

	// Pick any leaf and verify it returns the structured envelope.
	leaf := findRunnableLeaf(root)
	if leaf == nil {
		t.Fatalf("no runnable leaf in command tree")
	}
	err := leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "plugin_conflict" {
		t.Errorf("envelope type = %q, want plugin_conflict", exitErr.Detail.Type)
	}
	if rc := exitErr.Detail.Detail.(map[string]any)["reason_code"]; rc != "multiple_restrict_plugins" {
		t.Errorf("reason_code = %v, want multiple_restrict_plugins", rc)
	}
}

// Single plugin with an invalid Rule must surface as plugin_install /
// invalid_rule envelope (distinct error.type from multi-Restrict).
func TestPluginConflictGuard_InvalidRuleAbortsCLI(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	// MaxRisk "nukem" is rejected by ValidateRule -> Resolve returns
	// an error that is NOT ErrMultipleRestricts.
	platform.Register(&fakeIntegrationPlugin{
		name: "bad",
		caps: platform.Capabilities{Restricts: true, FailurePolicy: platform.FailClosed},
		rule: &platform.Rule{Name: "bad", MaxRisk: "nukem"},
	})

	_, root, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("conflict guard path should yield nil registry")
	}
	leaf := findRunnableLeaf(root)
	if leaf == nil {
		t.Fatalf("no runnable leaf in command tree")
	}
	err := leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "plugin_install" {
		t.Errorf("envelope type = %q, want plugin_install", exitErr.Detail.Type)
	}
	if rc := exitErr.Detail.Detail.(map[string]any)["reason_code"]; rc != "invalid_rule" {
		t.Errorf("reason_code = %v, want invalid_rule", rc)
	}
}

// === Startup lifecycle guard ===
//
// Plugin On(Startup) handler returning error must abort startup with
// a plugin_lifecycle envelope (reason_code lifecycle_failed). Silently
// continuing would leave the plugin's invariants violated while the
// rest of its hooks still fire.
func TestPluginLifecycleGuard_StartupErrorAbortsCLI(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	platform.Register(&startupFailingPlugin{
		name:    "lc",
		failErr: errors.New("backend unreachable"),
	})

	_, root, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("lifecycle guard path should yield nil registry")
	}

	leaf := findRunnableLeaf(root)
	err := leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "plugin_lifecycle" {
		t.Errorf("envelope type = %q, want plugin_lifecycle", exitErr.Detail.Type)
	}
	d := exitErr.Detail.Detail.(map[string]any)
	if d["reason_code"] != "lifecycle_failed" {
		t.Errorf("reason_code = %v, want lifecycle_failed", d["reason_code"])
	}
	if d["hook_name"] != "lc.start" {
		t.Errorf("hook_name = %v, want lc.start", d["hook_name"])
	}
}

// Same path but the handler panics -> reason_code lifecycle_panic.
func TestPluginLifecycleGuard_StartupPanicAbortsCLI(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	platform.Register(&startupFailingPlugin{
		name:     "lc",
		doPanic:  true,
		panicMsg: "kaboom",
	})

	_, root, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("lifecycle guard path should yield nil registry")
	}
	leaf := findRunnableLeaf(root)
	err := leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if rc := exitErr.Detail.Detail.(map[string]any)["reason_code"]; rc != "lifecycle_panic" {
		t.Errorf("reason_code = %v, want lifecycle_panic", rc)
	}
}

type startupFailingPlugin struct {
	name     string
	failErr  error // when set, handler returns this
	doPanic  bool  // when true, handler panics with panicMsg
	panicMsg string
}

func (p *startupFailingPlugin) Name() string    { return p.name }
func (p *startupFailingPlugin) Version() string { return "0.0.1" }
func (p *startupFailingPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (p *startupFailingPlugin) Install(r platform.Registrar) error {
	r.On(platform.Startup, "start", func(context.Context, *platform.LifecycleContext) error {
		if p.doPanic {
			panic(p.panicMsg)
		}
		return p.failErr
	})
	return nil
}

// === Wrapper panic recovery ===
//
// A Wrapper that panics must NOT crash the process. The framework
// recovers and converts to a structured envelope:
//
//	type="hook", reason_code="panic", hook_name=<namespaced>
func TestWrapperPanic_BecomesHookPanicEnvelope(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(&panickingWrapPlugin{name: "p"})

	result, err := internalplatform.InstallAll(platform.RegisteredPlugins(), nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	root, leaf := syntheticTree()
	if err := wireHooks(context.Background(), root, result.Registry); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Wrapper panic must be recovered, but it escaped: %v", r)
		}
	}()

	err = leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "hook" {
		t.Errorf("envelope type = %q, want hook", exitErr.Detail.Type)
	}
	d := exitErr.Detail.Detail.(map[string]any)
	if d["reason_code"] != "panic" {
		t.Errorf("reason_code = %v, want panic", d["reason_code"])
	}
	if d["hook_name"] != "p.boom" {
		t.Errorf("hook_name = %v, want p.boom (namespaced)", d["hook_name"])
	}
}

type panickingWrapPlugin struct{ name string }

func (p *panickingWrapPlugin) Name() string                        { return p.name }
func (p *panickingWrapPlugin) Version() string                     { return "0.0.1" }
func (p *panickingWrapPlugin) Capabilities() platform.Capabilities { return platform.Capabilities{} }
func (p *panickingWrapPlugin) Install(r platform.Registrar) error {
	r.Wrap("boom", platform.All(),
		func(_ platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error {
				panic("intentional panic for test")
			}
		})
	return nil
}

// findRunnableLeaf walks the tree and returns the first command with a
// RunE so tests can synthesize a dispatch without going through cobra.
func findRunnableLeaf(c *cobra.Command) *cobra.Command {
	if c.RunE != nil && c.HasParent() {
		return c
	}
	for _, child := range c.Commands() {
		if l := findRunnableLeaf(child); l != nil {
			return l
		}
	}
	return nil
}

// B2 regression: a plugin Wrapper whose FACTORY function (the
// `func(next Handler) Handler` itself) panics must not crash the
// process. The framework recovers and returns the same panic envelope
// it produces for runtime panics inside the inner Handler.
//
// Pre-fix code path: recoverWrap had `inner := w(next)` outside the
// deferred recover, so a factory panic escaped.
func TestWrapperFactoryPanic_BecomesHookPanicEnvelope(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(&factoryPanicWrapPlugin{name: "fac"})

	result, err := internalplatform.InstallAll(platform.RegisteredPlugins(), nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	root, leaf := syntheticTree()
	if err := wireHooks(context.Background(), root, result.Registry); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("factory panic must be recovered, but it escaped: %v", r)
		}
	}()

	err = leaf.RunE(leaf, nil)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError, got %T %+v", err, err)
	}
	if exitErr.Detail.Type != "hook" {
		t.Errorf("envelope type = %q, want hook", exitErr.Detail.Type)
	}
	d := exitErr.Detail.Detail.(map[string]any)
	if d["reason_code"] != "panic" {
		t.Errorf("reason_code = %v, want panic", d["reason_code"])
	}
	if d["hook_name"] != "fac.bad-factory" {
		t.Errorf("hook_name = %v, want fac.bad-factory (namespaced)", d["hook_name"])
	}
}

type factoryPanicWrapPlugin struct{ name string }

func (p *factoryPanicWrapPlugin) Name() string                        { return p.name }
func (p *factoryPanicWrapPlugin) Version() string                     { return "0.0.1" }
func (p *factoryPanicWrapPlugin) Capabilities() platform.Capabilities { return platform.Capabilities{} }
func (p *factoryPanicWrapPlugin) Install(r platform.Registrar) error {
	r.Wrap("bad-factory", platform.All(),
		// The factory itself panics; the returned Handler is never reached.
		func(_ platform.Handler) platform.Handler {
			panic("factory blew up")
		})
	return nil
}
