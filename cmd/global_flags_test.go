// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/pflag"
)

func testStreams() BuildOption { return WithIO(os.Stdin, os.Stdout, os.Stderr) }

func TestRegisterGlobalFlags_PolicyVisible(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts := &GlobalOptions{}
	RegisterGlobalFlags(fs, opts)

	flag := fs.Lookup("profile")
	if flag == nil {
		t.Fatal("profile flag should be registered")
	}
	if flag.Hidden {
		t.Fatal("profile flag should be visible when HideProfile is false")
	}
}

func TestRegisterGlobalFlags_PolicyHidden(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts := &GlobalOptions{HideProfile: true}
	RegisterGlobalFlags(fs, opts)

	flag := fs.Lookup("profile")
	if flag == nil {
		t.Fatal("profile flag should be registered")
	}
	if !flag.Hidden {
		t.Fatal("profile flag should be hidden when HideProfile is true")
	}
	if err := fs.Parse([]string{"--profile", "x"}); err != nil {
		t.Fatalf("Parse() error = %v; hidden flag should still parse", err)
	}
	if opts.Profile != "x" {
		t.Fatalf("opts.Profile = %q, want %q", opts.Profile, "x")
	}
}

func TestIsSingleAppMode_NoConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if !isSingleAppMode() {
		t.Fatal("isSingleAppMode() = false, want true when no config exists")
	}
}

func TestIsSingleAppMode_SingleApp(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	saveAppsForTest(t, []core.AppConfig{
		{Name: "default", AppId: "cli_a", AppSecret: core.PlainSecret("x"), Brand: core.BrandFeishu},
	})
	if !isSingleAppMode() {
		t.Fatal("isSingleAppMode() = false, want true for single-app config")
	}
}

func TestIsSingleAppMode_MultiApp(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	saveAppsForTest(t, []core.AppConfig{
		{Name: "a", AppId: "cli_a", AppSecret: core.PlainSecret("x"), Brand: core.BrandFeishu},
		{Name: "b", AppId: "cli_b", AppSecret: core.PlainSecret("y"), Brand: core.BrandFeishu},
	})
	if isSingleAppMode() {
		t.Fatal("isSingleAppMode() = true, want false for multi-app config")
	}
}

func TestBuildInternal_HideProfileOption(t *testing.T) {
	_, root := buildInternal(context.Background(), cmdutil.InvocationContext{}, testStreams(), HideProfile(true))

	flag := root.PersistentFlags().Lookup("profile")
	if flag == nil {
		t.Fatal("profile flag should be registered")
	}
	if !flag.Hidden {
		t.Fatal("profile flag should be hidden when HideProfile(true) is applied")
	}
}

func TestBuildInternal_DefaultShowsProfileFlag(t *testing.T) {
	_, root := buildInternal(context.Background(), cmdutil.InvocationContext{}, testStreams())

	flag := root.PersistentFlags().Lookup("profile")
	if flag == nil {
		t.Fatal("profile flag should be registered by default")
	}
	if flag.Hidden {
		t.Fatal("profile flag should be visible by default")
	}
}

func saveAppsForTest(t *testing.T, apps []core.AppConfig) {
	t.Helper()
	multi := &core.MultiAppConfig{CurrentApp: apps[0].Name, Apps: apps}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
}
