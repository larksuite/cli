// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestAddAPIIdentityFlag_NonStrictMode(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(cmd, f, nil)

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible outside strict mode")
	}
	if got := flag.DefValue; got != "auto" {
		t.Fatalf("default value = %q, want %q", got, "auto")
	}
}

func TestAddAPIIdentityFlag_StrictModeHidesFlagAndLocksDefault(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{
		AppID: "a", AppSecret: "s", SupportedIdentities: 2,
	})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(cmd, f, nil)

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

func TestAddShortcutIdentityFlag_UsesAuthTypes(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(cmd, f, []string{"bot"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible outside strict mode")
	}
	if got := flag.DefValue; got != "bot" {
		t.Fatalf("default value = %q, want %q", got, "bot")
	}
}
