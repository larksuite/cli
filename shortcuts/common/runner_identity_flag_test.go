// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestShortcutMount_StrictModeHidesAsFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu, SupportedIdentities: 2,
	})
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "docs",
		Command:     "+fetch",
		Description: "fetch doc",
		AuthTypes:   []string{"user", "bot"},
		Execute: func(context.Context, *RuntimeContext) error {
			return nil
		},
	}

	shortcut.Mount(parent, f)
	cmd, _, err := parent.Find([]string{"+fetch"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if !flag.Hidden {
		t.Fatal("expected --as flag to be hidden in strict mode")
	}
	if got := flag.DefValue; got != "bot" {
		t.Fatalf("default value = %q, want %q", got, "bot")
	}
}

func TestShortcutMount_SingleAuthUserShortcutUsesUserWhenDefaultAsBotAndUserConfigured(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		DefaultAs:  core.AsBot,
		UserOpenId: "ou_test_user",
	})
	var gotAs core.Identity
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "apps",
		Command:     "+get",
		Description: "get app",
		AuthTypes:   []string{"user"},
		Execute: func(_ context.Context, rctx *RuntimeContext) error {
			gotAs = rctx.As()
			return nil
		},
	}

	shortcut.Mount(parent, f)
	parent.SetArgs([]string{"+get"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAs != core.AsUser {
		t.Fatalf("resolved identity = %q, want %q", gotAs, core.AsUser)
	}
}

func TestShortcutMount_SingleAuthUserShortcutRejectsExplicitBot(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		DefaultAs:  core.AsBot,
		UserOpenId: "ou_test_user",
	})
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "apps",
		Command:     "+get",
		Description: "get app",
		AuthTypes:   []string{"user"},
		Execute: func(context.Context, *RuntimeContext) error {
			return nil
		},
	}

	shortcut.Mount(parent, f)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	parent.SetArgs([]string{"+get", "--as", "bot"})
	if err := parent.Execute(); err == nil {
		t.Fatal("Execute(--as bot) = nil, want validation error")
	}
}

func TestCanImplicitlyUseShortcutIdentity(t *testing.T) {
	if !canImplicitlyUseShortcutIdentity(nil, core.AsBot) {
		t.Fatal("expected AsBot to be implicitly usable")
	}
	if canImplicitlyUseShortcutIdentity(nil, core.AsUser) {
		t.Fatal("expected nil factory to reject AsUser")
	}
	fNoUser, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	if canImplicitlyUseShortcutIdentity(fNoUser, core.AsUser) {
		t.Fatal("expected missing UserOpenId to reject AsUser")
	}
}
