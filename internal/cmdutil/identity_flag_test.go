// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestAddAPIIdentityFlag_NonStrictMode(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(context.Background(), cmd, f, nil)

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible outside strict mode")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
}

func TestAddAPIIdentityFlag_StrictModeHidesFlagAndLocksDefault(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{
		AppID: "a", AppSecret: "s", SupportedIdentities: 2,
	})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(context.Background(), cmd, f, nil)

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

func TestAddShortcutIdentityFlag_NoDefault(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(context.Background(), cmd, f, []string{"bot"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible outside strict mode")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
}

// TC-10: AuthTypes=["user"] → usage contains "identity type: user" and NOT "bot".
func TestAddShortcutIdentityFlag_UserOnlyAuthTypes(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(context.Background(), cmd, f, []string{"user"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible")
	}
	wantUsage := "identity type: user"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
	if strings.Contains(flag.Usage, "bot") {
		t.Errorf("Usage should not contain \"bot\" for user-only shortcut, got %q", flag.Usage)
	}
}

// TC-11: AuthTypes=["user","bot"] → usage == "identity type: user | bot".
func TestAddShortcutIdentityFlag_UserBotAuthTypes(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(context.Background(), cmd, f, []string{"user", "bot"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
	wantUsage := "identity type: user | bot"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
}
