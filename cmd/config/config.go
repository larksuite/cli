// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

// NewCmdConfig creates the config command with subcommands.
func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Global CLI configuration management",
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(NewCmdConfigInit(f, nil))
	cmd.AddCommand(NewCmdConfigRemove(f, nil))
	cmd.AddCommand(NewCmdConfigShow(f, nil))
	cmd.AddCommand(NewCmdConfigDefaultAs(f))
	cmd.AddCommand(NewCmdConfigStrictMode(f))
	return cmd
}

func parseBrand(value string) core.LarkBrand {
	return core.ParseBrand(value)
}
