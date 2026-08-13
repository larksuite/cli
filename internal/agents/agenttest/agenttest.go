// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package agenttest provides provider conformance tests: a new integrator calls
// RunConformance in its own test to lock down registration metadata, offline
// resolution, the mandatory core hooks, and single-sourced card derivation. All
// assertions run offline (no runtime, no API calls).
package agenttest

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/core"
)

// CheckParamsBinding locks the declaration↔consumption contract for one
// operation: every `param:"name"` tag on T must reference a parameter declared
// on that verb, and the field kind must be compatible with the declared Type
// (string↔string, int/int64↔integer, float64↔number, bool↔boolean). A provider
// using BindParams[T] calls this once per binding struct in its own tests, so
// a renamed/retyped declaration fails CI instead of silently zero-valuing at
// runtime.
func CheckParamsBinding[T any](t *testing.T, spec *agents.AgentSpec, verb string) {
	t.Helper()
	op, ok := spec.Op(verb)
	if !ok {
		t.Fatalf("params binding: unknown verb %q", verb)
	}
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil || rt.Kind() != reflect.Struct {
		t.Fatalf("params binding: %T is not a struct", zero)
	}
	checkBindingLevel(t, rt, op.Params, verb, "")
}

// checkBindingLevel walks one struct level against one declaration level; a
// nested struct field recurses into the matching object param's Fields.
func checkBindingLevel(t *testing.T, rt reflect.Type, declaredParams []agents.CardParam, verb, where string) {
	t.Helper()
	declared := make(map[string]agents.CardParam, len(declaredParams))
	for _, p := range declaredParams {
		declared[p.Name] = p
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("param")
		if tag == "" || tag == "-" {
			continue
		}
		if !f.IsExported() {
			t.Errorf("params binding: field %s%s is unexported but tagged param %q (BindParams cannot set it)", where, f.Name, tag)
			continue
		}
		cp, ok := declared[tag]
		if !ok {
			t.Errorf("params binding: field %s%s tags param %q which %s does not declare", where, f.Name, tag, verb)
			continue
		}
		if f.Type.Kind() == reflect.Struct {
			if cp.Type != "object" {
				t.Errorf("params binding: field %s%s is a struct but param %q is declared %q (want object)", where, f.Name, tag, cp.Type)
				continue
			}
			checkBindingLevel(t, f.Type, cp.Fields, verb, where+tag+".")
			continue
		}
		typ := cp.Type
		if typ == "" {
			typ = "string"
		}
		compatible := map[string][]reflect.Kind{
			"string":  {reflect.String},
			"integer": {reflect.Int, reflect.Int64},
			"number":  {reflect.Float64},
			"boolean": {reflect.Bool},
		}[typ]
		okKind := false
		for _, k := range compatible {
			if f.Type.Kind() == k {
				okKind = true
			}
		}
		if !okKind {
			t.Errorf("params binding: field %s%s (%s) is incompatible with param %q declared type %q", where, f.Name, f.Type.Kind(), tag, typ)
		}
	}
}

// RunConformance runs the full set of conformance assertions against a
// registered scheme. sampleAgentID must be a valid agent id (catalog: an id from
// the Catalog; instance: any non-empty id).
func RunConformance(t *testing.T, scheme, sampleAgentID string) {
	t.Helper()
	prov, ok := agents.Info(scheme)
	if !ok {
		t.Fatalf("conformance: scheme %q not registered (the top-level agent package must be imported to trigger init registration)", scheme)
	}

	t.Run("metadata", func(t *testing.T) {
		if prov.Scheme != scheme {
			t.Errorf("conformance: Provider.Scheme should be %q, got %q", scheme, prov.Scheme)
		}
		if prov.Label == "" {
			t.Error("conformance: Provider.Label must not be empty")
		}
		if prov.AgentIDSource == "" {
			t.Error("conformance: Provider.AgentIDSource must not be empty")
		}
		if len(prov.Identities) == 0 {
			t.Error("conformance: Identities must not be empty")
		}
		seenType := make(map[agents.IdentityType]bool, len(prov.Identities))
		for i, id := range prov.Identities {
			if id.Type != agents.IdentityUser && id.Type != agents.IdentityBot {
				t.Errorf("conformance: Identities[%d].Type should be user|bot, got %q", i, id.Type)
			}
			if seenType[id.Type] {
				t.Errorf("conformance: Identities contains duplicate type %q", id.Type)
			}
			seenType[id.Type] = true
			seenScope := make(map[string]bool, len(id.Scopes))
			for _, s := range id.Scopes {
				if s == "" {
					t.Errorf("conformance: Identities[%d].Scopes contains an empty scope", i)
				}
				if seenScope[s] {
					t.Errorf("conformance: Identities[%d].Scopes contains duplicate %q", i, s)
				}
				seenScope[s] = true
			}
		}
		// Exactly one of Catalog / Instance is set (Register enforces; re-assert).
		if (len(prov.Catalog) > 0) == (prov.Instance != nil) {
			t.Error("conformance: exactly one of Catalog / Instance must be set")
		}
	})

	t.Run("lookup", func(t *testing.T) {
		gotProv, spec, agentID, err := agents.LookupSpec(scheme + ":" + sampleAgentID)
		if err != nil {
			t.Fatalf("conformance: LookupSpec(%s:%s) offline should succeed, got %v", scheme, sampleAgentID, err)
		}
		if gotProv.Scheme != scheme {
			t.Errorf("conformance: LookupSpec provider scheme should be %q, got %q", scheme, gotProv.Scheme)
		}
		if agentID != sampleAgentID {
			t.Errorf("conformance: LookupSpec should echo the agent id %q, got %q", sampleAgentID, agentID)
		}
		// Core operations are mandatory (the command layer dispatches them without
		// a nil-check); Register enforces this at registration, re-assert here.
		if spec.Send.Handler == nil {
			t.Error("conformance: spec.Send (core) must be wired")
		}
		if spec.GetTask.Handler == nil {
			t.Error("conformance: spec.GetTask (core) must be wired")
		}
	})

	t.Run("card", func(t *testing.T) {
		buildCard := func() *agents.AgentCard {
			t.Helper()
			_, spec, agentID, err := agents.LookupSpec(scheme + ":" + sampleAgentID)
			if err != nil {
				t.Fatalf("conformance: LookupSpec returned error: %v", err)
			}
			// rt=nil: the guaranteed-offline card (caps + registration + static
			// metadata). Describe enrichment is never exercised here.
			return agents.BuildCard(context.Background(), prov, spec, agentID, core.BrandFeishu, nil)
		}
		card := buildCard()
		if card.Provider != scheme {
			t.Errorf("conformance: Card.Provider should be %q, got %q", scheme, card.Provider)
		}
		if card.AgentID != sampleAgentID {
			t.Errorf("conformance: Card.AgentID should echo the input %q, got %q", sampleAgentID, card.AgentID)
		}
		if card.ProviderLabel != prov.Label {
			t.Errorf("conformance: Card.ProviderLabel should equal the registered Label %q, got %q", prov.Label, card.ProviderLabel)
		}
		if !reflect.DeepEqual(card.Identity, prov.Identities) {
			t.Errorf("conformance: Card.Identity should match the registered Identities, expected %+v got %+v", prov.Identities, card.Identity)
		}
		if card.AgentIDSource != prov.AgentIDSource {
			t.Errorf("conformance: Card.AgentIDSource should equal the registered value %q, got %q", prov.AgentIDSource, card.AgentIDSource)
		}
		if card.HasParameters == nil {
			t.Error("conformance: Card.HasParameters must not be nil (always emitted, empty is [])")
		}
		if !card.Capabilities.TaskGet {
			t.Error("conformance: task_get must be true (GetTask is a mandatory core hook)")
		}
		// Single-sourcing: two independent offline builds must DeepEqual.
		if card2 := buildCard(); !reflect.DeepEqual(card, card2) {
			t.Errorf("conformance: two offline BuildCard results should DeepEqual (single source), got\n%+v\nvs\n%+v", card, card2)
		}
	})

	t.Run("params", func(t *testing.T) {
		_, spec, _, err := agents.LookupSpec(scheme + ":" + sampleAgentID)
		if err != nil {
			t.Fatalf("conformance: LookupSpec returned error: %v", err)
		}
		// has_parameters must agree with the per-op declarations (single source).
		has := map[string]bool{}
		for _, v := range agents.HasParameters(spec) {
			has[v] = true
		}
		for _, o := range spec.Ops() {
			if want := o.Wired && len(o.Params) > 0; has[o.Verb] != want {
				t.Errorf("conformance: has_parameters[%s]=%v disagrees with the op declaration (wired=%v, %d params)",
					o.Verb, has[o.Verb], o.Wired, len(o.Params))
			}
		}
	})

	if prov.Kind() == agents.KindCatalog {
		t.Run("enumeration", func(t *testing.T) {
			list := prov.ListCatalog(core.BrandFeishu)
			wantRef := scheme + ":" + sampleAgentID
			found := false
			for i, a := range list {
				r, err := agents.ParseRef(a.AgentRef)
				if err != nil {
					t.Errorf("conformance: ListCatalog[%d].AgentRef %q should be parseable: %v", i, a.AgentRef, err)
					continue
				}
				if r.Scheme != scheme {
					t.Errorf("conformance: ListCatalog[%d].AgentRef %q scheme should be %q, got %q", i, a.AgentRef, scheme, r.Scheme)
				}
				if a.Name == "" {
					t.Errorf("conformance: ListCatalog[%d] (%s) Name must not be empty", i, a.AgentRef)
				}
				if a.AgentRef == wantRef {
					found = true
				}
			}
			if !found {
				t.Errorf("conformance: sampleAgentID should appear in the enumeration (expected %q), got %+v", wantRef, list)
			}
			// stable, sorted by AgentRef.
			list2 := prov.ListCatalog(core.BrandFeishu)
			if !reflect.DeepEqual(list, list2) {
				t.Errorf("conformance: two ListCatalog results should DeepEqual (stable), got\n%+v\nvs\n%+v", list, list2)
			}
			for i := 1; i < len(list); i++ {
				if strings.Compare(list[i-1].AgentRef, list[i].AgentRef) > 0 {
					t.Errorf("conformance: ListCatalog should be sorted by AgentRef, got %q before %q", list[i-1].AgentRef, list[i].AgentRef)
				}
			}
		})
	}
}
