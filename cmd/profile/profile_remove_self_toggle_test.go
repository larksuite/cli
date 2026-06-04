// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: removing the active profile must NOT leave PreviousApp
// pointing at the new active. Pre-fix, if the promoted Apps[0] (after
// the removed slot collapses) happened to equal PreviousApp,
// CurrentApp == PreviousApp held — and `profile use -` would
// short-circuit "Already on profile X" instead of toggling.
//
// Repro:
//   - alpha (current), beta (previous), gamma
//   - remove alpha → Apps[0]==beta → CurrentApp:=beta
//   - PreviousApp was already beta — now CurrentApp == PreviousApp
//   - `profile use -` short-circuits, breaking the toggle workflow
func TestProfileRemoveRun_AvoidsCurrentEqualsPreviousSelfToggle(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "alpha",
		PreviousApp: "beta",
		Apps: []core.AppConfig{
			{Name: "alpha", AppId: "app-alpha", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
			{Name: "beta", AppId: "app-beta", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
			{Name: "gamma", AppId: "app-gamma", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileRemoveRun(f, "alpha"); err != nil {
		t.Fatalf("profileRemoveRun: %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if saved.CurrentApp != "beta" {
		t.Errorf("CurrentApp = %q, want beta (Apps[0] after removal)", saved.CurrentApp)
	}
	// Invariant: CurrentApp != PreviousApp. PreviousApp must be cleared
	// because it would otherwise equal the new CurrentApp, making
	// `profile use -` a no-op.
	if saved.PreviousApp == saved.CurrentApp {
		t.Errorf("self-toggle invariant broken: CurrentApp=%q == PreviousApp=%q",
			saved.CurrentApp, saved.PreviousApp)
	}
	if saved.PreviousApp != "" {
		t.Errorf("PreviousApp = %q, want \"\" (cleared to restore invariant)", saved.PreviousApp)
	}
}

// Counter-test: when the removed profile is unrelated to PreviousApp,
// PreviousApp must NOT be cleared. Don't sweep state we don't have to.
func TestProfileRemoveRun_PreservesUnrelatedPreviousApp(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "alpha",
		PreviousApp: "beta",
		Apps: []core.AppConfig{
			{Name: "alpha", AppId: "app-alpha", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
			{Name: "beta", AppId: "app-beta", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
			{Name: "gamma", AppId: "app-gamma", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileRemoveRun(f, "gamma"); err != nil {
		t.Fatalf("profileRemoveRun: %v", err)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if saved.CurrentApp != "alpha" {
		t.Errorf("CurrentApp = %q, want alpha (untouched)", saved.CurrentApp)
	}
	if saved.PreviousApp != "beta" {
		t.Errorf("PreviousApp = %q, want beta (untouched)", saved.PreviousApp)
	}
}
