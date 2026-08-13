// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// swapRegistry replaces the global providerRegistry with the given map (restored
// via t.Cleanup), for isolation. It swaps without a lock, so no t.Parallel.
func swapRegistry(t *testing.T, m map[string]Provider) {
	t.Helper()
	saved := providerRegistry
	providerRegistry = m
	t.Cleanup(func() { providerRegistry = saved })
}

// coreSpec is a minimal valid spec: it wires the two mandatory core hooks so it
// passes Register's checkSpec. Callers set extra hooks / ID on the returned value.
func coreSpec(id string) AgentSpec {
	return AgentSpec{
		ID:      id,
		Send:    SendOp{Handler: func(context.Context, Runtime, SendInput) (*AgentTask, error) { return nil, nil }},
		GetTask: TaskGetOp{Handler: func(context.Context, Runtime, string) (*AgentTask, error) { return nil, nil }},
	}
}

// instanceProvider builds a minimal valid instance Provider for scheme.
func instanceProvider(scheme string) Provider {
	s := coreSpec("")
	return Provider{
		Scheme:        scheme,
		Label:         "test provider",
		AgentIDSource: "test source",
		Identities:    []IdentitySpec{{Type: IdentityUser}},
		Instance:      &s,
	}
}

// catalogProvider builds a minimal valid catalog Provider for scheme with the
// given entry ids.
func catalogProvider(scheme string, ids ...string) Provider {
	specs := make([]AgentSpec, 0, len(ids))
	for _, id := range ids {
		s := coreSpec(id)
		s.Name = "name-" + id
		specs = append(specs, s)
	}
	return Provider{
		Scheme:        scheme,
		Label:         "test provider",
		AgentIDSource: "test source",
		Identities:    []IdentitySpec{{Type: IdentityUser}},
		Catalog:       specs,
	}
}

// mustPanic asserts that fn panics and the message contains wantMsg.
func mustPanic(t *testing.T, wantMsg string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("should panic (want message containing %q)", wantMsg)
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, wantMsg) {
			t.Fatalf("panic message should contain %q, got %q", wantMsg, msg)
		}
	}()
	fn()
}

// TestRegisterPanicBranches table-drives the Register fail-fast branches.
func TestRegisterPanicBranches(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(p *Provider)
		wantMsg string
	}{
		{"empty Scheme", func(p *Provider) { p.Scheme = "" }, "empty Scheme"},
		{"missing Label", func(p *Provider) { p.Label = "" }, "missing Label"},
		{"missing AgentIDSource", func(p *Provider) { p.AgentIDSource = "" }, "missing AgentIDSource"},
		{"missing Identities", func(p *Provider) { p.Identities = nil }, "missing Identities"},
		{"invalid Identity Type", func(p *Provider) { p.Identities = []IdentitySpec{{Type: "robot"}} }, "got: robot"},
		{"duplicate Identity Type", func(p *Provider) {
			p.Identities = []IdentitySpec{{Type: IdentityUser}, {Type: IdentityUser}}
		}, "duplicate Identity Type"},
		{"empty scope in Identity Scopes", func(p *Provider) {
			p.Identities = []IdentitySpec{{Type: IdentityUser, Scopes: []string{""}}}
		}, "empty scope in Identity user"},
		{"duplicate scope in Identity Scopes", func(p *Provider) {
			p.Identities = []IdentitySpec{{Type: IdentityUser, Scopes: []string{"a:b:c", "a:b:c"}}}
		}, "duplicate scope in Identity user"},
		{"neither Catalog nor Instance", func(p *Provider) { p.Instance = nil }, "exactly one of Catalog / Instance"},
		{"both Catalog and Instance", func(p *Provider) { p.Catalog = catalogProvider("x", "a").Catalog }, "exactly one of Catalog / Instance"},
		{"instance template with ID", func(p *Provider) { p.Instance.ID = "oops" }, "instance template must have empty ID"},
		{"missing core Send", func(p *Provider) { p.Instance.Send = SendOp{} }, "missing core Send"},
		{"missing core GetTask", func(p *Provider) { p.Instance.GetTask = TaskGetOp{} }, "missing core GetTask"},
		{"InputRequired without CancelTask", func(p *Provider) { p.Instance.InputRequired = true }, "wires no CancelTask"},
		{"InputRequired with narrower CancelTask brands", func(p *Provider) {
			p.Instance.InputRequired = true
			p.Instance.CancelTask = TaskCancelOp{Brands: []core.LarkBrand{core.BrandFeishu},
				Handler: func(context.Context, Runtime, string) error { return nil }}
		}, "brand-scoped narrower"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			swapRegistry(t, map[string]Provider{})
			p := instanceProvider("bad")
			tc.mutate(&p)
			mustPanic(t, tc.wantMsg, func() { Register(p) })
		})
	}
}

// TestRegisterCatalogIDPanics pins the catalog-specific ID rules.
func TestRegisterCatalogIDPanics(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	missingID := catalogProvider("cat", "")
	mustPanic(t, "catalog spec missing ID", func() { Register(missingID) })

	swapRegistry(t, map[string]Provider{})
	dup := catalogProvider("cat", "a", "a")
	mustPanic(t, "duplicate entry ID", func() { Register(dup) })
}

func TestRegisterDuplicateScheme(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	Register(instanceProvider("dup"))
	mustPanic(t, "called twice for scheme: dup", func() { Register(instanceProvider("dup")) })
}

func TestInfoReturnsRegisteredProvider(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	p := instanceProvider("t1")
	p.Identities = []IdentitySpec{{Type: IdentityUser, Scopes: []string{"t1:chat:write"}}}
	Register(p)
	got, ok := Info("t1")
	if !ok || got.Label != "test provider" || got.Kind() != KindInstance {
		t.Fatalf("Info(t1) = %+v, %v", got, ok)
	}
	if _, ok := Info("nonexistent"); ok {
		t.Fatal("Info(nonexistent) should return ok=false")
	}
}

// TestScopesForIdentity pins the per-identity scope lookup: each declared
// identity resolves its own set, an undeclared identity resolves nil, and a
// declared identity without scopes resolves nil (both mean "no preflight").
func TestScopesForIdentity(t *testing.T) {
	p := instanceProvider("split")
	p.Identities = []IdentitySpec{
		{Type: IdentityUser, Scopes: []string{"u:doc:read"}},
		{Type: IdentityBot, Scopes: []string{"b:doc:read", "b:doc:write"}},
	}
	if got := p.ScopesForIdentity(IdentityUser); !reflect.DeepEqual(got, []string{"u:doc:read"}) {
		t.Errorf("user scopes = %v, want [u:doc:read]", got)
	}
	if got := p.ScopesForIdentity(IdentityBot); !reflect.DeepEqual(got, []string{"b:doc:read", "b:doc:write"}) {
		t.Errorf("bot scopes = %v, want [b:doc:read b:doc:write]", got)
	}

	userOnly := instanceProvider("useronly") // declares user with no scopes
	if got := userOnly.ScopesForIdentity(IdentityUser); got != nil {
		t.Errorf("declared identity without scopes should resolve nil, got %v", got)
	}
	if got := userOnly.ScopesForIdentity(IdentityBot); got != nil {
		t.Errorf("undeclared identity should resolve nil, got %v", got)
	}
}

func TestKindAndAgentRefFormat(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	inst := instanceProvider("inst")
	cat := catalogProvider("cat", "a")
	if inst.Kind() != KindInstance {
		t.Errorf("instance provider Kind should be instance, got %q", inst.Kind())
	}
	if cat.Kind() != KindCatalog {
		t.Errorf("catalog provider Kind should be catalog, got %q", cat.Kind())
	}
	if got := inst.AgentRefFormat(); got != "inst:<agent_id>" {
		t.Errorf("AgentRefFormat should be inst:<agent_id>, got %q", got)
	}
}

func TestListCatalog(t *testing.T) {
	// Catalog: sorted by AgentRef, stable, instance returns nil.
	cat := catalogProvider("cat", "zeta", "alpha")
	got := cat.ListCatalog(core.BrandFeishu)
	if len(got) != 2 || got[0].AgentRef != "cat:alpha" || got[1].AgentRef != "cat:zeta" {
		t.Fatalf("ListCatalog should be sorted by AgentRef, got %+v", got)
	}
	if instanceProvider("inst").ListCatalog(core.BrandFeishu) != nil {
		t.Error("instance ListCatalog should be nil")
	}
}

func TestKnownSchemesEmpty(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	if got := KnownSchemes(); got != "(none)" {
		t.Fatalf("an empty registry should return \"(none)\", got %q", got)
	}
}

func TestRegisteredSchemesSorted(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	Register(instanceProvider("gamma"))
	Register(instanceProvider("alpha"))
	Register(instanceProvider("beta"))
	got := RegisteredSchemes()
	want := []string{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredSchemes should enumerate and sort, want %v got %v", want, got)
	}
	if s := KnownSchemes(); s != "alpha, beta, gamma" {
		t.Fatalf("knownSchemes should be comma-joined, got %q", s)
	}
}

func TestLookupSpecInvalidRef(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	_, _, _, err := LookupSpec("no-colon")
	if !errors.Is(err, ErrInvalidRef) {
		t.Fatalf("an invalid ref should propagate ErrInvalidRef, got %v", err)
	}
}

func TestLookupSpecUnknownScheme(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	_, _, _, err := LookupSpec("nosuch:agt_x")
	if err == nil {
		t.Fatal("an unregistered scheme should return an error")
	}
	if errors.Is(err, ErrInvalidRef) {
		t.Fatalf("an unregistered scheme should not be ErrInvalidRef, got %v", err)
	}
}

func TestLookupSpecInstance(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	Register(instanceProvider("demo"))
	prov, spec, agentID, err := LookupSpec("demo:agt_42")
	if err != nil {
		t.Fatalf("a valid instance ref should succeed, got %v", err)
	}
	if prov.Scheme != "demo" || spec == nil || spec.Send.Handler == nil {
		t.Fatalf("should return the instance template, got prov=%+v spec=%v", prov, spec)
	}
	if agentID != "agt_42" {
		t.Fatalf("should echo the parsed agentID, got %q", agentID)
	}
}

func TestLookupSpecCatalog(t *testing.T) {
	swapRegistry(t, map[string]Provider{})
	Register(catalogProvider("cat", "alpha", "beta"))
	_, spec, agentID, err := LookupSpec("cat:beta")
	if err != nil {
		t.Fatalf("a known catalog id should succeed, got %v", err)
	}
	if spec == nil || spec.ID != "beta" || agentID != "beta" {
		t.Fatalf("should return the matching catalog entry, got %+v (id %q)", spec, agentID)
	}
	// Unknown id → typed validation error.
	_, _, _, err = LookupSpec("cat:nope")
	if err == nil {
		t.Fatal("an unknown catalog id should return an error")
	}
}
