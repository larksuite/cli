// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

// pruneForStrictMode removes commands incompatible with the active strict mode.
func pruneForStrictMode(root *cobra.Command, mode core.StrictMode) {
	pruneIncompatible(root, mode)
	pruneAuthCommands(root, mode)
	pruneEmpty(root)
}

// pruneIncompatible recursively removes commands whose annotation declares
// identities incompatible with the forced identity. Commands without annotation are kept.
func pruneIncompatible(parent *cobra.Command, mode core.StrictMode) {
	forced := string(mode.ForcedIdentity())
	var toRemove []*cobra.Command
	for _, child := range parent.Commands() {
		ids := cmdutil.GetSupportedIdentities(child)
		if ids != nil && !contains(ids, forced) {
			toRemove = append(toRemove, child)
			continue
		}
		pruneIncompatible(child, mode)
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
	}
}

// pruneAuthCommands removes auth login when strict mode is bot.
func pruneAuthCommands(root *cobra.Command, mode core.StrictMode) {
	if mode != core.StrictModeBot {
		return
	}
	for _, child := range root.Commands() {
		if child.Name() != "auth" {
			continue
		}
		var toRemove []*cobra.Command
		for _, sub := range child.Commands() {
			if sub.Name() == "login" {
				toRemove = append(toRemove, sub)
			}
		}
		if len(toRemove) > 0 {
			child.RemoveCommand(toRemove...)
		}
	}
}

// pruneEmpty recursively removes group commands (no Run/RunE) that have
// no remaining subcommands after pruning.
func pruneEmpty(parent *cobra.Command) {
	var toRemove []*cobra.Command
	for _, child := range parent.Commands() {
		pruneEmpty(child)
		// Only remove non-runnable group commands with no children left.
		if child.Run == nil && child.RunE == nil && !child.HasAvailableSubCommands() {
			toRemove = append(toRemove, child)
		}
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
