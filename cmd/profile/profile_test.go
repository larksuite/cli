// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func setupProfileConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	return dir
}

func TestProfileAddRun_InvalidExistingConfigReturnsError(t *testing.T) {
	dir := setupProfileConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("secret\n")

	err := profileAddRun(f, "test", "app-test", true, "feishu", "zh", false)
	if err == nil {
		t.Fatal("expected error for invalid existing config")
	}
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Fatalf("error = %v, want failed to load config", err)
	}
}

func TestProfileAddRun_UseAfterUpdatesCurrentAndPrevious(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("secret-new\n")

	if err := profileAddRun(f, "target", "app-target", true, "lark", "en", true); err != nil {
		t.Fatalf("profileAddRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "target" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "target")
	}
	if saved.PreviousApp != "default" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "default")
	}
	if len(saved.Apps) != 2 {
		t.Fatalf("len(Apps) = %d, want 2", len(saved.Apps))
	}
}

func TestProfileRemoveRun_RemovesCurrentProfileAndSwitchesToFirstRemaining(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "target",
		PreviousApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
			{Name: "target", AppId: "app-target", AppSecret: core.PlainSecret("secret-target"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileRemoveRun(f, "target"); err != nil {
		t.Fatalf("profileRemoveRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "default" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "default")
	}
	if saved.PreviousApp != "default" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "default")
	}
	if len(saved.Apps) != 1 || saved.Apps[0].ProfileName() != "default" {
		t.Fatalf("remaining apps = %#v, want only default", saved.Apps)
	}
}
