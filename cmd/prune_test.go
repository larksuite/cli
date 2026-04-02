// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
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

	if findCmd(root, "im", "+search") != nil {
		t.Error("+search (user-only) should be removed in bot mode")
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
	if findCmd(root, "im", "messages", "search") != nil {
		t.Error("search (user-only method) should be removed in bot mode")
	}
	if findCmd(root, "auth", "login") != nil {
		t.Error("auth login should be removed in bot mode")
	}
}

func TestPruneForStrictMode_User(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeUser)

	if findCmd(root, "im", "+search") == nil {
		t.Error("+search (user-only) should be kept in user mode")
	}
	if findCmd(root, "im", "+subscribe") != nil {
		t.Error("+subscribe (bot-only) should be removed in user mode")
	}
	if findCmd(root, "im", "+send") == nil {
		t.Error("+send (dual) should be kept in user mode")
	}
	if findCmd(root, "auth", "login") == nil {
		t.Error("auth login should be kept in user mode")
	}
}

func TestPruneEmpty(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)

	if findCmd(root, "im", "messages") != nil {
		t.Error("empty resource 'messages' should be pruned")
	}
}
