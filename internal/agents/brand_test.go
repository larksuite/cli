// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// TestDeriveCapabilitiesBrandScoped pins the brand-aware matrix: a wired op that
// declares Brands is a live capability only under a listed brand. reporter-like
// spec wires CancelTask feishu-only, so task_cancel is true under feishu and
// false under lark (the op is excluded), while the mandatory task_get (no
// Brands) stays true under both.
func TestDeriveCapabilitiesBrandScoped(t *testing.T) {
	s := coreSpec("reporter")
	s.CancelTask = TaskCancelOp{
		Brands:  []core.LarkBrand{core.BrandFeishu},
		Handler: func(context.Context, Runtime, string) error { return nil },
	}

	feishu := DeriveCapabilities(&s, core.BrandFeishu)
	if !feishu.TaskCancel {
		t.Error("feishu: task_cancel should be true (wired + feishu-scoped)")
	}
	if !feishu.TaskGet {
		t.Error("feishu: task_get (core, no Brands) should always be true")
	}

	lark := DeriveCapabilities(&s, core.BrandLark)
	if lark.TaskCancel {
		t.Error("lark: task_cancel should be false (op excluded from lark)")
	}
	if !lark.TaskGet {
		t.Error("lark: task_get (core, no Brands) should still be true")
	}
}

// TestSpecAvailableForBrand pins the whole-agent visibility rule: empty Brands
// means every brand; a scoped list restricts to its members.
func TestSpecAvailableForBrand(t *testing.T) {
	empty := coreSpec("a") // no Brands
	if !SpecAvailableForBrand(&empty, core.BrandFeishu) || !SpecAvailableForBrand(&empty, core.BrandLark) {
		t.Error("empty Brands should be available under every brand")
	}

	scoped := coreSpec("b")
	scoped.Brands = []core.LarkBrand{core.BrandFeishu}
	if !SpecAvailableForBrand(&scoped, core.BrandFeishu) {
		t.Error("[feishu] should be available under feishu")
	}
	if SpecAvailableForBrand(&scoped, core.BrandLark) {
		t.Error("[feishu] should NOT be available under lark")
	}
}

// TestRegisterPanicsInvalidBrand pins the Register-time fail-fast on a bad brand
// value in both the whole-agent spec.Brands and a per-op Op.Brands.
func TestRegisterPanicsInvalidBrand(t *testing.T) {
	swapRegistry(t, map[string]Provider{})

	// Whole-agent spec.Brands with a value that is neither feishu nor lark.
	badSpec := catalogProvider("bs", "a")
	badSpec.Catalog[0].Brands = []core.LarkBrand{"weibo"}
	mustPanic(t, "spec invalid Brand", func() { Register(badSpec) })

	// Per-op Op.Brands with a bad value (the op is wired so it is not caught by
	// the params-on-unwired check first).
	badOp := catalogProvider("bo", "a")
	badOp.Catalog[0].CancelTask = TaskCancelOp{
		Brands:  []core.LarkBrand{"nope"},
		Handler: func(context.Context, Runtime, string) error { return nil },
	}
	mustPanic(t, "op invalid Brand", func() { Register(badOp) })
}

// TestListCatalogBrandExclusion pins that ListCatalog(brand) filters out a
// whole-agent-scoped spec: a feishu-only agent is listed under feishu but
// EXCLUDED under lark, while an unrestricted agent is listed under both.
func TestListCatalogBrandExclusion(t *testing.T) {
	p := Provider{
		Scheme: "x",
		Catalog: []AgentSpec{
			{ID: "all", Name: "all-brands"},
			{ID: "feishuonly", Name: "feishu-only", Brands: []core.LarkBrand{core.BrandFeishu}},
		},
	}
	has := func(list []AgentSummary, ref string) bool {
		for _, a := range list {
			if a.AgentRef == ref {
				return true
			}
		}
		return false
	}
	if fe := p.ListCatalog(core.BrandFeishu); !has(fe, "x:all") || !has(fe, "x:feishuonly") {
		t.Errorf("feishu catalog should include both agents, got %v", fe)
	}
	la := p.ListCatalog(core.BrandLark)
	if !has(la, "x:all") {
		t.Errorf("lark catalog should include the unrestricted agent, got %v", la)
	}
	if has(la, "x:feishuonly") {
		t.Errorf("lark catalog should EXCLUDE the feishu-only agent, got %v", la)
	}
}
