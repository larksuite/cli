// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func newTestTree() *cobra.Command {
	root := &cobra.Command{Use: "root"}

	svc := &cobra.Command{Use: "im"}
	root.AddCommand(svc)

	noop := func(*cobra.Command, []string) error { return nil }

	userOnly := &cobra.Command{Use: "+search", Short: "user only", RunE: noop}
	cmdutil.SetSupportedIdentities(userOnly, []string{"user"})
	svc.AddCommand(userOnly)

	botOnly := &cobra.Command{Use: "+subscribe", Short: "bot only", RunE: noop}
	cmdutil.SetSupportedIdentities(botOnly, []string{"bot"})
	svc.AddCommand(botOnly)

	dual := &cobra.Command{Use: "+send", Short: "dual", RunE: noop}
	cmdutil.SetSupportedIdentities(dual, []string{"user", "bot"})
	svc.AddCommand(dual)

	noAnnotation := &cobra.Command{Use: "+legacy", Short: "no annotation", RunE: noop}
	svc.AddCommand(noAnnotation)

	res := &cobra.Command{Use: "messages"}
	svc.AddCommand(res)
	userMethod := &cobra.Command{Use: "search", RunE: func(*cobra.Command, []string) error { return nil }}
	cmdutil.SetSupportedIdentities(userMethod, []string{"user"})
	res.AddCommand(userMethod)

	auth := &cobra.Command{Use: "auth"}
	root.AddCommand(auth)
	login := &cobra.Command{Use: "login", RunE: noop}
	cmdutil.SetSupportedIdentities(login, []string{"user"})
	auth.AddCommand(login)

	return root
}

func findCmd(root *cobra.Command, names ...string) *cobra.Command {
	cmd := root
	for _, name := range names {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == name {
				cmd = c
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
}

func TestPruneForStrictMode_Bot(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)

	if cmd := findCmd(root, "im", "+search"); cmd == nil || !cmd.Hidden {
		t.Error("+search (user-only) should be replaced by a hidden stub in bot mode")
	}
	if findCmd(root, "im", "+subscribe") == nil {
		t.Error("+subscribe (bot-only) should be kept in bot mode")
	}
	if findCmd(root, "im", "+send") == nil {
		t.Error("+send (dual) should be kept in bot mode")
	}
	if findCmd(root, "im", "+legacy") == nil {
		t.Error("+legacy (no annotation) should be kept")
	}
	if cmd := findCmd(root, "im", "messages", "search"); cmd == nil || !cmd.Hidden {
		t.Error("search (user-only method) should be replaced by a hidden stub in bot mode")
	}
	if cmd := findCmd(root, "auth", "login"); cmd == nil || !cmd.Hidden {
		t.Error("auth login should be replaced by a hidden stub in bot mode")
	}
}

func TestPruneForStrictMode_User(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeUser)

	if findCmd(root, "im", "+search") == nil {
		t.Error("+search (user-only) should be kept in user mode")
	}
	if cmd := findCmd(root, "im", "+subscribe"); cmd == nil || !cmd.Hidden {
		t.Error("+subscribe (bot-only) should be replaced by a hidden stub in user mode")
	}
	if findCmd(root, "im", "+send") == nil {
		t.Error("+send (dual) should be kept in user mode")
	}
	if cmd := findCmd(root, "auth", "login"); cmd == nil || cmd.Hidden {
		t.Error("auth login should be kept in user mode")
	}
}

func TestPruneEmpty(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)

	if cmd := findCmd(root, "im", "messages"); cmd == nil || !cmd.Hidden {
		t.Error("resource 'messages' should be kept hidden when only hidden stubs remain")
	}
}

func TestPruneEmpty_PreservesOriginallyHiddenGroup(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	hidden := &cobra.Command{Use: "hidden", Hidden: true}
	root.AddCommand(hidden)
	hidden.AddCommand(&cobra.Command{
		Use:  "visible",
		RunE: func(*cobra.Command, []string) error { return nil },
	})

	pruneEmpty(root)

	if !hidden.Hidden {
		t.Fatal("expected originally hidden group to remain hidden")
	}
}

func TestPruneForStrictMode_Bot_DirectUserShortcutReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeBot)
	root.SetArgs([]string{"im", "+search", "--query", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "bot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPruneForStrictMode_Bot_DirectNestedUserMethodReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeBot)
	root.SetArgs([]string{"im", "messages", "search", "--query", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "bot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPruneForStrictMode_Bot_DirectAuthLoginReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeBot)
	root.SetArgs([]string{"auth", "login", "--json", "--scope", "im:message.send_as_user"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "bot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPruneForStrictMode_User_DirectBotShortcutReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeUser)
	root.SetArgs([]string{"im", "+subscribe", "--topic", "x"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "user"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
