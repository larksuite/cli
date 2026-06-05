// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

// recorder Registrar captures everything a builder schedules so the
// test can assert what Install produced without involving the host.
type recorder struct {
	observers  int
	wrappers   int
	lifecycles int
	rule       *platform.Rule   // last rule (existing single-rule assertions)
	rules      []*platform.Rule // every rule, in Restrict order
}

func (r *recorder) Observe(platform.When, string, platform.Selector, platform.Observer) {
	r.observers++
}
func (r *recorder) Wrap(string, platform.Selector, platform.Wrapper)              { r.wrappers++ }
func (r *recorder) On(platform.LifecycleEvent, string, platform.LifecycleHandler) { r.lifecycles++ }
func (r *recorder) Restrict(rule *platform.Rule) {
	r.rule = rule
	r.rules = append(r.rules, rule)
}

// Restrict must snapshot each rule: a caller that reuses and mutates the
// same *Rule object across two Restrict calls must still get two distinct
// rules at Install time, not two pointers to the last mutation.
func TestBuilder_restrictClonesEachRule(t *testing.T) {
	shared := &platform.Rule{Name: "docs-ro", Allow: []string{"docs/**"}, MaxRisk: platform.RiskRead}
	b := platform.NewPlugin("p", "0").Restrict(shared)
	// Reuse and mutate the same object, then register it again.
	shared.Name = "im-rw"
	shared.Allow[0] = "im/**"
	shared.MaxRisk = platform.RiskWrite
	p, err := b.Restrict(shared).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r := &recorder{}
	if err := p.Install(r); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(r.rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(r.rules))
	}
	if r.rules[0].Name != "docs-ro" || r.rules[0].Allow[0] != "docs/**" || r.rules[0].MaxRisk != platform.RiskRead {
		t.Errorf("rule[0] leaked later mutation: %+v", r.rules[0])
	}
	if r.rules[1].Name != "im-rw" || r.rules[1].Allow[0] != "im/**" {
		t.Errorf("rule[1] = %+v, want im-rw / im/**", r.rules[1])
	}
}

func TestBuilder_basicAssembly(t *testing.T) {
	p, err := platform.NewPlugin("audit", "0.1.0").
		Observer(platform.Before, "pre", platform.All(),
			func(context.Context, platform.Invocation) {}).
		Observer(platform.After, "post", platform.All(),
			func(context.Context, platform.Invocation) {}).
		Wrap("policy", platform.All(),
			func(next platform.Handler) platform.Handler { return next }).
		On(platform.Startup, "boot",
			func(context.Context, *platform.LifecycleContext) error { return nil }).
		FailOpen().
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.Name() != "audit" || p.Version() != "0.1.0" {
		t.Errorf("metadata = %q/%q", p.Name(), p.Version())
	}
	if p.Capabilities().FailurePolicy != platform.FailOpen {
		t.Errorf("FailurePolicy = %v, want FailOpen", p.Capabilities().FailurePolicy)
	}

	r := &recorder{}
	if err := p.Install(r); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if r.observers != 2 || r.wrappers != 1 || r.lifecycles != 1 {
		t.Errorf("Install dispatch = observers=%d wrappers=%d lifecycles=%d",
			r.observers, r.wrappers, r.lifecycles)
	}
}

// Restrict() flips Restricts=true and FailClosed automatically — a
// policy plugin can't accidentally ship under FailOpen.
func TestBuilder_restrictForcesFailClosed(t *testing.T) {
	p, err := platform.NewPlugin("policy-plugin", "0.1.0").
		Restrict(&platform.Rule{Name: "read-only", MaxRisk: platform.RiskRead}).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	caps := p.Capabilities()
	if !caps.Restricts {
		t.Errorf("Restricts = false, want true (Restrict() should flip it)")
	}
	if caps.FailurePolicy != platform.FailClosed {
		t.Errorf("FailurePolicy = %v, want FailClosed (Restrict() implies it)", caps.FailurePolicy)
	}

	r := &recorder{}
	if err := p.Install(r); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if r.rule == nil || r.rule.Name != "read-only" {
		t.Errorf("Install did not propagate Rule: %+v", r.rule)
	}
}

// Invalid name surfaces at Build time, not at NewPlugin.
func TestBuilder_invalidPluginName(t *testing.T) {
	_, err := platform.NewPlugin("Has_Underscore_And_Caps", "0.1").Build()
	if err == nil {
		t.Fatalf("Build must reject malformed plugin name")
	}
	if !strings.Contains(err.Error(), "invalid plugin name") {
		t.Errorf("error should mention plugin name, got: %v", err)
	}
}

// Duplicate hookName within the same builder is rejected.
func TestBuilder_duplicateHookName(t *testing.T) {
	noopObs := func(context.Context, platform.Invocation) {}
	_, err := platform.NewPlugin("dup", "0").
		Observer(platform.Before, "h", platform.All(), noopObs).
		Observer(platform.After, "h", platform.All(), noopObs).
		Build()
	if err == nil {
		t.Fatalf("Build must reject duplicate hookName")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Errorf("error should mention duplicate hookName, got %v", err)
	}
}

func TestBuilder_invalidHookName(t *testing.T) {
	_, err := platform.NewPlugin("p", "0").
		Observer(platform.Before, "Bad.Name", platform.All(),
			func(context.Context, platform.Invocation) {}).
		Build()
	if err == nil {
		t.Fatalf("Build must reject hookName with dot")
	}
}

// MustBuild panics on builder error.
func TestBuilder_mustBuildPanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("MustBuild must panic when Build would fail")
		}
	}()
	_ = platform.NewPlugin("BadName", "0").MustBuild()
}

func TestBuilder_restrictNilRejected(t *testing.T) {
	_, err := platform.NewPlugin("p", "0").Restrict(nil).Build()
	if err == nil {
		t.Fatalf("Restrict(nil) must produce error")
	}
}

func TestBuilder_capabilitiesSetters(t *testing.T) {
	p, err := platform.NewPlugin("p", "0.1").
		RequireCLI(">=1.0.0").
		FailClosed().
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	caps := p.Capabilities()
	if caps.RequiredCLIVersion != ">=1.0.0" {
		t.Errorf("RequiredCLIVersion = %q, want >=1.0.0", caps.RequiredCLIVersion)
	}
	if caps.FailurePolicy != platform.FailClosed {
		t.Errorf("FailurePolicy = %v, want FailClosed", caps.FailurePolicy)
	}
}

func TestBuilder_restrictThenFailOpenRejected(t *testing.T) {
	rule := &platform.Rule{Name: "r", MaxRisk: platform.RiskRead}
	_, err := platform.NewPlugin("p", "0").Restrict(rule).FailOpen().Build()
	if err == nil {
		t.Fatalf("Build must reject Restrict()+FailOpen() mismatch")
	}
	if !strings.Contains(err.Error(), "FailClosed") {
		t.Errorf("error should mention FailClosed, got: %v", err)
	}
}

// Restrict() flips FailurePolicy to FailClosed; the previous FailOpen()
// is overridden. Pin it so the Build-time validation does not over-reject.
func TestBuilder_failOpenThenRestrictOK(t *testing.T) {
	rule := &platform.Rule{Name: "r", MaxRisk: platform.RiskRead}
	p, err := platform.NewPlugin("p", "0").FailOpen().Restrict(rule).Build()
	if err != nil {
		t.Fatalf("FailOpen()+Restrict() must succeed (Restrict flips to FailClosed): %v", err)
	}
	if p.Capabilities().FailurePolicy != platform.FailClosed {
		t.Errorf("FailurePolicy = %v, want FailClosed", p.Capabilities().FailurePolicy)
	}
}
