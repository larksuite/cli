// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/meta"
)

func TestResolveStartupBrandCompatibilityPrecedence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliBrand, "")

	if got := ResolveStartupBrand(""); got != core.BrandFeishu {
		t.Fatalf("ResolveStartupBrand(empty state) = %q, want %q", got, core.BrandFeishu)
	}

	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "feishu-profile",
		Apps: []core.AppConfig{
			{Name: "feishu-profile", AppId: "cli_feishu", Brand: core.BrandFeishu},
			{Name: "lark-profile", AppId: "cli_lark", Brand: core.BrandLark},
		},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
	if got := ResolveStartupBrand("lark-profile"); got != core.BrandLark {
		t.Fatalf("ResolveStartupBrand(profile) = %q, want %q", got, core.BrandLark)
	}
	if got := ResolveStartupBrand(""); got != core.BrandFeishu {
		t.Fatalf("ResolveStartupBrand(active profile) = %q, want %q", got, core.BrandFeishu)
	}

	t.Setenv(envvars.CliBrand, "lark")
	if got := ResolveStartupBrand(""); got != core.BrandLark {
		t.Fatalf("ResolveStartupBrand(environment) = %q, want %q", got, core.BrandLark)
	}
}

func TestWithStartupBrandDoesNotOverrideAPICatalog(t *testing.T) {
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{{Name: "compat"}})
	cfg := resolveBuildConfig([]BuildOption{
		WithServiceCatalog(catalog),
		WithStartupBrand(core.BrandLark),
	})

	got, err := openCatalog(cfg)
	if err != nil {
		t.Fatalf("openCatalog: %v", err)
	}
	if !reflect.DeepEqual(got.Names(), catalog.Names()) {
		t.Fatalf("WithStartupBrand changed injected catalog: got %v, want %v", got.Names(), catalog.Names())
	}
}
