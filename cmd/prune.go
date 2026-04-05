// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"slices"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// pruneForStrictMode removes commands incompatible with the active strict mode.
func pruneForStrictMode(root *cobra.Command, mode core.StrictMode) {
	pruneIncompatible(root, mode)
	pruneEmpty(root)
}

// pruneIncompatible recursively replaces commands whose annotation declares
// identities incompatible with the forced identity. Commands without annotation are kept.
// Hidden stubs preserve direct execution so users get a strict-mode error instead
// of Cobra's generic "unknown flag" fallback from the parent command.
func pruneIncompatible(parent *cobra.Command, mode core.StrictMode) {
	forced := string(mode.ForcedIdentity())
	var toRemove []*cobra.Command
	var toAdd []*cobra.Command
	for _, child := range parent.Commands() {
		ids := cmdutil.GetSupportedIdentities(child)
		if ids != nil && !slices.Contains(ids, forced) {
			toRemove = append(toRemove, child)
			toAdd = append(toAdd, strictModeStubFrom(child, mode))
			continue
		}
		pruneIncompatible(child, mode)
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
		parent.AddCommand(toAdd...)
	}
}

func strictModeStubFrom(child *cobra.Command, mode core.StrictMode) *cobra.Command {
	return &cobra.Command{
		Use:                child.Use,
		Aliases:            append([]string(nil), child.Aliases...),
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return output.Errorf(output.ExitValidation, "strict_mode",
				"strict mode is %q, only %s identity is allowed. "+
					"This setting is managed by the administrator and must not be modified by AI agents.",
				mode, mode.ForcedIdentity())
		},
	}
}

// pruneEmpty recursively removes group commands (no Run/RunE) that have
// no remaining subcommands after pruning. If only hidden stubs remain, keep
// the group hidden so direct execution still resolves to the stub path.
func pruneEmpty(parent *cobra.Command) {
	var toRemove []*cobra.Command
	for _, child := range parent.Commands() {
		pruneEmpty(child)
		if child.Run != nil || child.RunE != nil {
			continue
		}
		switch {
		case child.HasAvailableSubCommands():
		case len(child.Commands()) > 0:
			child.Hidden = true
		default:
			toRemove = append(toRemove, child)
		}
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
	}
}
