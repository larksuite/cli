// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// NewCmdProfile creates the profile command with subcommands.
func NewCmdProfile(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
		Long: `Profiles are named app identities managed by lark-cli.

Identity diagnostics and profile selection:
  lark-cli whoami --json       Show the app/profile lark-cli is using now.
  lark-cli auth status --json --verify  Verify OAuth login and token state.
  --profile <name>             Use a profile for this command only.
  LARKSUITE_CLI_PROFILE        Use a profile for the current shell / agent session.
  config show / profile list   Inspect saved config, not current usage.
  unset LARKSUITE_CLI_PROFILE  Clear the session profile and fall back to direct app env or configured default.

A selected profile takes precedence over matching direct env credentials and tokens.`,
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.SetTips(cmd, []string{
		"AI agents: Do NOT switch or remove profiles unless the user explicitly asks.",
	})

	cmd.AddCommand(NewCmdProfileList(f))
	cmd.AddCommand(NewCmdProfileUse(f))
	cmd.AddCommand(NewCmdProfileAdd(f))
	cmd.AddCommand(NewCmdProfileRemove(f))
	cmd.AddCommand(NewCmdProfileRename(f))
	return cmd
}
