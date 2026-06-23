// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

// testCatalog builds a small embedded catalog: services drive (no resources)
// and im with a dotted resource (chat.members), a multi-method resource
// (reactions, where list is user-only), and images.
func testCatalog() apicatalog.Catalog {
	im := meta.ServiceFromMap(map[string]interface{}{
		"name": "im",
		"resources": map[string]interface{}{
			"chat.members": map[string]interface{}{
				"methods": map[string]interface{}{"create": map[string]interface{}{}},
			},
			"reactions": map[string]interface{}{
				"methods": map[string]interface{}{
					"create": map[string]interface{}{},
					"list":   map[string]interface{}{"accessTokens": []interface{}{"user"}},
				},
			},
			"images": map[string]interface{}{
				"methods": map[string]interface{}{"create": map[string]interface{}{}},
			},
		},
	})
	drive := meta.ServiceFromMap(map[string]interface{}{"name": "drive"})
	return apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{drive, im}) // already name-sorted
}

func TestNew_PreservesOrderAndLookup(t *testing.T) {
	c := testCatalog()
	if c.Source() != apicatalog.SourceEmbedded {
		t.Fatalf("source = %q", c.Source())
	}
	names := []string{}
	for _, s := range c.Services() {
		names = append(names, s.Name)
	}
	if !reflect.DeepEqual(names, []string{"drive", "im"}) {
		t.Errorf("Services order = %v, want [drive im]", names)
	}
	if _, ok := c.Service("im"); !ok {
		t.Error("Service(im) not found")
	}
	if _, ok := c.Service("nope"); ok {
		t.Error("Service(nope) should not be found")
	}
}

// TestNew_SortsAndIsolatesInput pins the ordering contract New owns: it sorts
// arbitrary input by service name and shallow-copies the slice so later caller
// mutation can't reorder the Catalog.
func TestNew_SortsAndIsolatesInput(t *testing.T) {
	in := []meta.Service{
		meta.ServiceFromMap(map[string]interface{}{"name": "zeta"}),
		meta.ServiceFromMap(map[string]interface{}{"name": "alpha"}),
	}
	c := apicatalog.New(apicatalog.SourceEmbedded, in)

	names := func() []string {
		var out []string
		for _, s := range c.Services() {
			out = append(out, s.Name)
		}
		return out
	}
	if got := names(); !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("New did not sort unsorted input: %v", got)
	}

	// Mutating the caller's slice afterward must not reorder the Catalog.
	in[0] = meta.ServiceFromMap(map[string]interface{}{"name": "MUTATED"})
	if got := names(); !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("Catalog order changed after caller mutated its input slice: %v", got)
	}
}

func TestWalkMethods_AllAndFiltered(t *testing.T) {
	c := testCatalog()

	all := c.WalkMethods(nil)
	got := map[string]bool{}
	for _, r := range all {
		got[r.SchemaPath()] = true
	}
	want := []string{
		"im.chat.members.create",
		"im.images.create",
		"im.reactions.create",
		"im.reactions.list",
	}
	if len(all) != len(want) {
		t.Fatalf("WalkMethods(nil) = %d refs, want %d (%v)", len(all), len(want), got)
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("WalkMethods(nil) missing %q", w)
		}
	}

	// Deterministic order: services by name, resources by name, methods by name.
	var order []string
	for _, r := range all {
		order = append(order, r.SchemaPath())
	}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("WalkMethods order = %v, want %v", order, want)
	}

	// Filter to bot-only ("tenant"): reactions.list (user-only) drops; methods
	// with no accessTokens are permissive and stay.
	botOnly := func(m meta.Method) bool {
		if m.AccessTokens == nil {
			return true
		}
		for _, tok := range m.AccessTokens {
			if tok == "tenant" {
				return true
			}
		}
		return false
	}
	filtered := c.WalkMethods(botOnly)
	for _, r := range filtered {
		if r.SchemaPath() == "im.reactions.list" {
			t.Error("filtered walk should drop user-only im.reactions.list")
		}
	}
	if len(filtered) != len(all)-1 {
		t.Errorf("filtered walk = %d, want %d", len(filtered), len(all)-1)
	}
}

func TestMethodRef_Paths_DottedResourceStaysOneSegment(t *testing.T) {
	c := testCatalog()
	target, err := c.Resolve([]string{"im", "chat.members", "create"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if target.Kind != apicatalog.TargetMethod {
		t.Fatalf("kind = %v", target.Kind)
	}
	m := target.Method
	if m.SchemaPath() != "im.chat.members.create" {
		t.Errorf("SchemaPath = %q", m.SchemaPath())
	}
	if !reflect.DeepEqual(m.CommandPath(), []string{"im", "chat.members", "create"}) {
		t.Errorf("CommandPath = %v", m.CommandPath())
	}
	if m.ResourceName() != "chat.members" {
		t.Errorf("ResourceName = %q, want chat.members (one segment)", m.ResourceName())
	}
	if m.Method.Name != "create" {
		t.Errorf("Method.Name not injected: %q", m.Method.Name)
	}
}

func TestResolve_DottedAndSplitFormsEquivalent(t *testing.T) {
	c := testCatalog()
	// schema.ParsePath splits both "im.chat.members.create" and
	// "im chat.members create" into segments; findResource's longest-prefix
	// must resolve the dotted resource either way.
	a, errA := c.Resolve([]string{"im", "chat", "members", "create"}) // fully split
	b, errB := c.Resolve([]string{"im", "chat.members", "create"})    // resource as one segment
	if errA != nil || errB != nil {
		t.Fatalf("errA=%v errB=%v", errA, errB)
	}
	if a.Method.SchemaPath() != b.Method.SchemaPath() || a.Method.SchemaPath() != "im.chat.members.create" {
		t.Errorf("forms diverged: %q vs %q", a.Method.SchemaPath(), b.Method.SchemaPath())
	}
}

func TestResolve_Targets(t *testing.T) {
	c := testCatalog()
	if tg, _ := c.Resolve(nil); tg.Kind != apicatalog.TargetAll {
		t.Errorf("empty -> %v, want all", tg.Kind)
	}
	if tg, _ := c.Resolve([]string{"im"}); tg.Kind != apicatalog.TargetService || tg.Service.Name != "im" {
		t.Errorf("[im] -> %v/%q", tg.Kind, tg.Service.Name)
	}
	if tg, _ := c.Resolve([]string{"im", "reactions"}); tg.Kind != apicatalog.TargetResource || tg.Resource.SchemaPath() != "im.reactions" {
		t.Errorf("[im reactions] -> %v", tg.Kind)
	}
}

func TestResolve_Errors(t *testing.T) {
	c := testCatalog()
	cases := []struct {
		parts []string
		kind  apicatalog.ResolveErrorKind
	}{
		{[]string{"nope"}, apicatalog.ErrService},
		{[]string{"im", "nope"}, apicatalog.ErrResource},
		{[]string{"im", "reactions", "nope"}, apicatalog.ErrMethod},
		{[]string{"im", "reactions", "list", "extra"}, apicatalog.ErrPath},
	}
	for _, tc := range cases {
		_, err := c.Resolve(tc.parts)
		var re *apicatalog.ResolveError
		if !errors.As(err, &re) {
			t.Errorf("%v -> err %v, want *ResolveError", tc.parts, err)
			continue
		}
		if re.Kind != tc.kind {
			t.Errorf("%v -> kind %q, want %q", tc.parts, re.Kind, tc.kind)
		}
		if tc.kind != apicatalog.ErrPath && len(re.Candidates) == 0 {
			t.Errorf("%v -> expected candidates", tc.parts)
		}
	}
}

// nestedCatalog adds a genuinely nested resource (spaces > items) on top of a
// flat dotted resource (chat.members), so the round-trip contract is exercised
// for real nesting — not just flat dotted keys.
func nestedCatalog() apicatalog.Catalog {
	im := meta.ServiceFromMap(map[string]interface{}{
		"name": "im",
		"resources": map[string]interface{}{
			"chat.members": map[string]interface{}{
				"methods": map[string]interface{}{"create": map[string]interface{}{}},
			},
			"spaces": map[string]interface{}{
				"methods": map[string]interface{}{"create": map[string]interface{}{}},
				"resources": map[string]interface{}{
					"items": map[string]interface{}{
						"methods": map[string]interface{}{"get": map[string]interface{}{}},
					},
				},
			},
		},
	})
	return apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{im})
}

// TestResolve_WalkMethodsRoundTrip is the core catalog contract: every method
// WalkMethods emits must Resolve back to the same method — both from its dotted
// SchemaPath (fully split) and from its CommandPath (resource as one segment).
// This pins findResource's nested-resource descent symmetric to walkResources,
// so "traversable" implies "resolvable".
func TestResolve_WalkMethodsRoundTrip(t *testing.T) {
	for _, c := range []apicatalog.Catalog{testCatalog(), nestedCatalog()} {
		for _, ref := range c.WalkMethods(nil) {
			want := ref.SchemaPath()
			for _, parts := range [][]string{
				strings.Split(want, "."), // fully-split dotted form
				ref.CommandPath(),        // command form (resource stays one segment)
			} {
				tg, err := c.Resolve(parts)
				if err != nil {
					t.Errorf("round-trip %v: %v", parts, err)
					continue
				}
				if tg.Kind != apicatalog.TargetMethod {
					t.Errorf("round-trip %v: kind=%v, want method", parts, tg.Kind)
					continue
				}
				if tg.Method.SchemaPath() != want {
					t.Errorf("round-trip %v: resolved to %q, want %q", parts, tg.Method.SchemaPath(), want)
				}
			}
		}
	}
}

// TestComplete_Nested pins completion closure for genuinely nested resources:
// both the dotted and space forms must reach a nested method, symmetric to
// Resolve (findResource descends, so completion must too).
func TestComplete_Nested(t *testing.T) {
	c := nestedCatalog()

	// dotted: under a resource, offer its methods AND its sub-resources
	if comps, ns := c.Complete(nil, "im.spaces.", nil); !reflect.DeepEqual(comps, []string{"im.spaces.create", "im.spaces.items."}) || ns {
		t.Errorf("Complete([], im.spaces.) = %v noSpace=%v, want [im.spaces.create im.spaces.items.] false", comps, ns)
	}
	// dotted: drill into the nested sub-resource's method
	if comps, ns := c.Complete(nil, "im.spaces.items.", nil); !reflect.DeepEqual(comps, []string{"im.spaces.items.get"}) || ns {
		t.Errorf("Complete([], im.spaces.items.) = %v noSpace=%v, want [im.spaces.items.get] false", comps, ns)
	}
	// dotted: partial sub-resource name -> the sub-resource (NoSpace, more to type)
	if comps, ns := c.Complete(nil, "im.spaces.it", nil); !reflect.DeepEqual(comps, []string{"im.spaces.items."}) || !ns {
		t.Errorf("Complete([], im.spaces.it) = %v noSpace=%v, want [im.spaces.items.] true", comps, ns)
	}
	// space form: under a resource, offer methods AND sub-resources
	if comps, _ := c.Complete([]string{"im", "spaces"}, "", nil); !reflect.DeepEqual(comps, []string{"create", "items"}) {
		t.Errorf("Complete([im spaces], '') = %v, want [create items]", comps)
	}
	// space form: drill into the nested sub-resource's methods
	if comps, _ := c.Complete([]string{"im", "spaces", "items"}, "", nil); !reflect.DeepEqual(comps, []string{"get"}) {
		t.Errorf("Complete([im spaces items], '') = %v, want [get]", comps)
	}
}

func TestComplete(t *testing.T) {
	c := testCatalog()

	// dotted: service prefix -> "im." (NoSpace)
	if comps, ns := c.Complete(nil, "i", nil); !reflect.DeepEqual(comps, []string{"im."}) || !ns {
		t.Errorf("Complete([], i) = %v noSpace=%v", comps, ns)
	}
	// dotted: resource prefix -> "im.reactions." (NoSpace)
	if comps, _ := c.Complete(nil, "im.rea", nil); !reflect.DeepEqual(comps, []string{"im.reactions."}) {
		t.Errorf("Complete([], im.rea) = %v", comps)
	}
	// space form: resource candidates under im (deterministic order)
	comps, ns := c.Complete([]string{"im"}, "", nil)
	if !reflect.DeepEqual(comps, []string{"chat.members", "images", "reactions"}) || ns {
		t.Errorf("Complete([im], '') = %v noSpace=%v", comps, ns)
	}
	// space form: method candidates under reactions
	if comps, _ := c.Complete([]string{"im", "reactions"}, "", nil); !reflect.DeepEqual(comps, []string{"create", "list"}) {
		t.Errorf("Complete([im reactions], '') = %v", comps)
	}
	// filter applied: bot-only hides user-only list
	botOnly := func(m meta.Method) bool {
		if m.AccessTokens == nil {
			return true
		}
		for _, tok := range m.AccessTokens {
			if tok == "tenant" {
				return true
			}
		}
		return false
	}
	if comps, _ := c.Complete([]string{"im", "reactions"}, "", botOnly); !reflect.DeepEqual(comps, []string{"create"}) {
		t.Errorf("Complete with bot filter = %v, want [create]", comps)
	}
}
