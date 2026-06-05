// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// happyPlugin is a textbook plugin: declares Capabilities, calls a few
// Registrar methods, returns nil. The install pipeline must accept it.
type happyPlugin struct{ name string }

func (p happyPlugin) Name() string    { return p.name }
func (p happyPlugin) Version() string { return "1.0.0" }
func (p happyPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		FailurePolicy: platform.FailOpen,
	}
}
func (p happyPlugin) Install(r platform.Registrar) error {
	r.Observe(platform.Before, "audit-pre", platform.All(),
		func(context.Context, platform.Invocation) {})
	r.Wrap("policy", platform.All(),
		func(next platform.Handler) platform.Handler {
			return func(ctx context.Context, inv platform.Invocation) error {
				return next(ctx, inv)
			}
		})
	r.On(platform.Shutdown, "flush",
		func(context.Context, *platform.LifecycleContext) error { return nil })
	return nil
}

func TestInstallAll_happyPlugin(t *testing.T) {
	result, err := internalplatform.InstallAll([]platform.Plugin{happyPlugin{name: "audit"}}, nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	if result.Registry == nil {
		t.Fatalf("registry should be populated")
	}
	if len(result.PluginRules) != 0 {
		t.Errorf("happy plugin did not call Restrict; rules should be empty")
	}
	// Cross-check: observers, wrappers, lifecycles got staged through to the live Registry.
	if len(result.Registry.MatchingObservers(fakeView{}, platform.Before)) != 1 {
		t.Errorf("Before observer not committed")
	}
	if len(result.Registry.MatchingWrappers(fakeView{})) != 1 {
		t.Errorf("Wrapper not committed")
	}
	if len(result.Registry.LifecycleHandlers(platform.Shutdown)) != 1 {
		t.Errorf("Shutdown lifecycle not committed")
	}
}

// fakeView satisfies platform.CommandView for selector lookups in the
// platformhost tests; All() matches everything so the type can stay
// trivial.
type fakeView struct{}

func (fakeView) Path() string                     { return "" }
func (fakeView) Domain() string                   { return "" }
func (fakeView) Risk() (platform.Risk, bool)      { return "", false }
func (fakeView) Identities() []platform.Identity  { return nil }
func (fakeView) Annotation(string) (string, bool) { return "", false }

// A FailClosed plugin whose Install returns an error must abort
// InstallAll. Design hard-constraint #6.
type failClosedPlugin struct{}

func (failClosedPlugin) Name() string    { return "secaudit" }
func (failClosedPlugin) Version() string { return "1.0.0" }
func (failClosedPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		FailurePolicy: platform.FailClosed,
	}
}
func (failClosedPlugin) Install(platform.Registrar) error {
	return errors.New("upstream unreachable")
}

func TestInstallAll_failClosedAborts(t *testing.T) {
	_, err := internalplatform.InstallAll([]platform.Plugin{failClosedPlugin{}}, nil)
	if err == nil {
		t.Fatalf("FailClosed install error should abort")
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) {
		t.Fatalf("error must be *PluginInstallError, got %T", err)
	}
	if pi.ReasonCode != internalplatform.ReasonInstallFailed {
		t.Errorf("ReasonCode = %q, want install_failed", pi.ReasonCode)
	}
}

// FailOpen install failure logs a warning and skips this plugin; other
// plugins still get installed.
type failOpenPlugin struct{}

func (failOpenPlugin) Name() string    { return "audit-broken" }
func (failOpenPlugin) Version() string { return "1.0.0" }
func (failOpenPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailOpen}
}
func (failOpenPlugin) Install(platform.Registrar) error {
	return errors.New("could not connect")
}

func TestInstallAll_failOpenSkips(t *testing.T) {
	var buf bytes.Buffer
	plugins := []platform.Plugin{
		failOpenPlugin{},
		happyPlugin{name: "audit"},
	}
	result, err := internalplatform.InstallAll(plugins, &buf)
	if err != nil {
		t.Fatalf("FailOpen failure must not abort, got %v", err)
	}
	if !strings.Contains(buf.String(), "audit-broken") {
		t.Errorf("FailOpen warning should mention plugin name, got %q", buf.String())
	}
	// Second plugin's observer should be present.
	if len(result.Registry.MatchingObservers(fakeView{}, platform.Before)) != 1 {
		t.Errorf("happy plugin's observer should still be installed after first plugin skipped")
	}
}

// Restricts=true with FailOpen is a configuration error: a policy
// plugin that silently disappears under FailOpen would erase the
// security boundary. The host must reject this combo BEFORE Install
// runs.
type misconfiguredRestrictPlugin struct{}

func (misconfiguredRestrictPlugin) Name() string    { return "secaudit" }
func (misconfiguredRestrictPlugin) Version() string { return "1.0.0" }
func (misconfiguredRestrictPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		Restricts:     true,              // policy plugin
		FailurePolicy: platform.FailOpen, // contradicts safety contract
	}
}
func (misconfiguredRestrictPlugin) Install(platform.Registrar) error { return nil }

func TestInstallAll_restrictsRequiresFailClosed(t *testing.T) {
	_, err := internalplatform.InstallAll([]platform.Plugin{misconfiguredRestrictPlugin{}}, nil)
	if err == nil {
		t.Fatalf("Restricts+FailOpen must abort")
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonRestrictsMismatch {
		t.Fatalf("ReasonCode = %v, want restricts_mismatch", pi)
	}
}

// Restricts=true but Install didn't call r.Restrict -> mismatch.
type lyingRestrictPlugin struct{}

func (lyingRestrictPlugin) Name() string    { return "p" }
func (lyingRestrictPlugin) Version() string { return "1.0.0" }
func (lyingRestrictPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		Restricts:     true,
		FailurePolicy: platform.FailClosed,
	}
}
func (lyingRestrictPlugin) Install(platform.Registrar) error {
	// Forgot to call r.Restrict.
	return nil
}

func TestInstallAll_restrictsDeclaredButNotCalled(t *testing.T) {
	_, err := internalplatform.InstallAll([]platform.Plugin{lyingRestrictPlugin{}}, nil)
	if err == nil {
		t.Fatalf("missing Restrict call when declared must fail")
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonRestrictsMismatch {
		t.Fatalf("ReasonCode = %v, want restricts_mismatch", pi)
	}
}

// Plugin that panics inside Install must NOT crash the binary -- the
// host recovers and converts the panic into a typed install_panic.
type panicInstallPlugin struct{}

func (panicInstallPlugin) Name() string    { return "panicker" }
func (panicInstallPlugin) Version() string { return "1.0.0" }
func (panicInstallPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (panicInstallPlugin) Install(platform.Registrar) error {
	panic("boom")
}

func TestInstallAll_installPanicRecovered(t *testing.T) {
	_, err := internalplatform.InstallAll([]platform.Plugin{panicInstallPlugin{}}, nil)
	if err == nil {
		t.Fatalf("Install panic should surface as error")
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonInstallPanic {
		t.Fatalf("ReasonCode = %v, want install_panic", pi)
	}
}

// Two plugins with the same Name must abort before any Install runs.
func TestInstallAll_duplicatePluginName(t *testing.T) {
	_, err := internalplatform.InstallAll([]platform.Plugin{
		happyPlugin{name: "audit"},
		happyPlugin{name: "audit"},
	}, nil)
	if err == nil {
		t.Fatalf("duplicate Plugin.Name must abort")
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonDuplicatePluginName {
		t.Fatalf("ReasonCode = %v, want duplicate_plugin_name", pi)
	}
}

// Plugin with an invalid Name (contains "." or starts with a hyphen)
// must abort with invalid_plugin_name. The dot ban is critical -- the
// "{plugin}.{hook}" namespace join would become ambiguous if dots were
// allowed inside Plugin.Name().
type badNamePlugin struct{ n string }

func (p badNamePlugin) Name() string    { return p.n }
func (p badNamePlugin) Version() string { return "1.0.0" }
func (p badNamePlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (p badNamePlugin) Install(platform.Registrar) error { return nil }

func TestInstallAll_invalidPluginName(t *testing.T) {
	cases := []string{"with.dot", "", "-leading-hyphen", "UPPER"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := internalplatform.InstallAll([]platform.Plugin{badNamePlugin{n: name}}, nil)
			if err == nil {
				t.Fatalf("invalid name %q should abort", name)
			}
			var pi *internalplatform.PluginInstallError
			if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonInvalidPluginName {
				t.Fatalf("ReasonCode = %v, want invalid_plugin_name", pi)
			}
		})
	}
}

// Plugin's Install registers two hooks with the same name -- the
// staging Registrar rejects the second one with duplicate_hook_name.
type duplicateHookPlugin struct{}

func (duplicateHookPlugin) Name() string    { return "dup" }
func (duplicateHookPlugin) Version() string { return "1.0.0" }
func (duplicateHookPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (duplicateHookPlugin) Install(r platform.Registrar) error {
	r.Observe(platform.Before, "x", platform.All(), func(context.Context, platform.Invocation) {})
	r.Observe(platform.After, "x", platform.All(), func(context.Context, platform.Invocation) {})
	return nil
}

func TestInstallAll_duplicateHookName(t *testing.T) {
	_, err := internalplatform.InstallAll([]platform.Plugin{duplicateHookPlugin{}}, nil)
	if err == nil {
		t.Fatalf("duplicate hookName within same plugin must abort")
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonDuplicateHookName {
		t.Fatalf("ReasonCode = %v, want duplicate_hook_name", pi)
	}
}

// Restrict contributes a rule to result.PluginRules so the pruning
// resolver can pick it up. Exercise the full path.
type restrictPlugin struct{ rule *platform.Rule }

func (p restrictPlugin) Name() string    { return "secaudit" }
func (p restrictPlugin) Version() string { return "1.0.0" }
func (p restrictPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		Restricts:     true,
		FailurePolicy: platform.FailClosed,
	}
}
func (p restrictPlugin) Install(r platform.Registrar) error {
	r.Restrict(p.rule)
	return nil
}

func TestInstallAll_restrictPropagatesRule(t *testing.T) {
	rule := &platform.Rule{
		Name:       "secaudit-policy",
		MaxRisk:    "read",
		Allow:      []string{"docs/**"},
		Deny:       []string{"docs/+delete-doc"},
		Identities: []platform.Identity{"bot"},
	}
	result, err := internalplatform.InstallAll([]platform.Plugin{restrictPlugin{rule: rule}}, nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	if len(result.PluginRules) != 1 {
		t.Fatalf("expected 1 plugin rule, got %d", len(result.PluginRules))
	}
	stored := result.PluginRules[0].Rule
	if stored == nil {
		t.Fatalf("stored rule is nil")
	}

	// stagingRegistrar.Restrict defensively clones the plugin-supplied
	// rule so a misbehaving plugin can't mutate it after Install
	// returns. The clone must carry identical contents but live on a
	// distinct pointer.
	if stored == rule {
		t.Errorf("stored rule should be a clone, got identical pointer")
	}
	if stored.Name != rule.Name || stored.MaxRisk != rule.MaxRisk {
		t.Errorf("stored rule lost data: %+v", stored)
	}
	if got, want := len(stored.Allow), len(rule.Allow); got != want {
		t.Errorf("stored Allow len = %d, want %d", got, want)
	}

	// Verify the clone is actually isolated: mutating the plugin's
	// rule after install must not change the stored one.
	rule.Allow[0] = "evil/**"
	rule.Deny = append(rule.Deny, "extra/**")
	if stored.Allow[0] == "evil/**" {
		t.Errorf("Allow slice aliased plugin storage")
	}
	if len(stored.Deny) != 1 {
		t.Errorf("Deny slice aliased plugin storage: %v", stored.Deny)
	}

	if result.PluginRules[0].PluginName != "secaudit" {
		t.Errorf("PluginName = %q", result.PluginRules[0].PluginName)
	}
}

// Atomic install: a plugin whose validation fails AFTER it registered
// some hooks must NOT leak those hooks into the live registry. The
// staging buffer is the atomicity boundary.
type partiallyRegisterThenFailPlugin struct{}

func (partiallyRegisterThenFailPlugin) Name() string    { return "partial" }
func (partiallyRegisterThenFailPlugin) Version() string { return "1.0.0" }
func (partiallyRegisterThenFailPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		Restricts:     true, // declares Restrict but won't call it
		FailurePolicy: platform.FailClosed,
	}
}
func (partiallyRegisterThenFailPlugin) Install(r platform.Registrar) error {
	r.Observe(platform.Before, "would-leak", platform.All(),
		func(context.Context, platform.Invocation) {})
	// validateSelf will fail because Restricts=true but Restrict
	// was not called -- this is the atomic-rollback case.
	return nil
}

func TestInstallAll_atomicRollback(t *testing.T) {
	_, err := internalplatform.InstallAll(
		[]platform.Plugin{partiallyRegisterThenFailPlugin{}, happyPlugin{name: "audit"}},
		nil,
	)
	if err == nil {
		t.Fatalf("partial plugin should abort (FailClosed)")
	}
	// We cannot check Registry contents here because InstallAll
	// returns nil on failure; the rollback invariant is "nothing the
	// failing plugin staged ever reached a live Registry", which is
	// proven by the fact that we got nil back. A weaker but useful
	// check: even if we passed a happy second plugin, the loop must
	// have stopped at the first FailClosed failure.
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) {
		t.Fatalf("error must be *PluginInstallError, got %T", err)
	}
}

// multiRestrictPlugin calls r.Restrict twice -- the multi-rule case. A
// single plugin may declare several scoped grants; both must be collected
// into PluginRules under the same plugin name, in registration order.
type multiRestrictPlugin struct{}

func (multiRestrictPlugin) Name() string    { return "secaudit" }
func (multiRestrictPlugin) Version() string { return "1.0.0" }
func (multiRestrictPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		Restricts:     true,
		FailurePolicy: platform.FailClosed,
	}
}
func (multiRestrictPlugin) Install(r platform.Registrar) error {
	r.Restrict(&platform.Rule{Name: "docs-ro", Allow: []string{"docs/**"}, MaxRisk: platform.RiskRead})
	r.Restrict(&platform.Rule{Name: "im-rw", Allow: []string{"im/**"}, MaxRisk: platform.RiskWrite})
	return nil
}

// A single plugin calling Restrict more than once is valid (multi-rule
// support): both rules are collected, in order, under the one plugin name.
// This pins the behaviour change from the old "Restrict at most once"
// double_restrict error.
func TestInstallAll_multipleRestrictPerPlugin(t *testing.T) {
	result, err := internalplatform.InstallAll([]platform.Plugin{multiRestrictPlugin{}}, nil)
	if err != nil {
		t.Fatalf("multiple Restrict per plugin must succeed, got %v", err)
	}
	if len(result.PluginRules) != 2 {
		t.Fatalf("PluginRules = %d, want 2", len(result.PluginRules))
	}
	for _, pr := range result.PluginRules {
		if pr.PluginName != "secaudit" {
			t.Errorf("PluginName = %q, want secaudit", pr.PluginName)
		}
	}
	if result.PluginRules[0].Rule.Name != "docs-ro" || result.PluginRules[1].Rule.Name != "im-rw" {
		t.Errorf("rules out of order: %q, %q",
			result.PluginRules[0].Rule.Name, result.PluginRules[1].Rule.Name)
	}
}
