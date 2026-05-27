// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !darwin

package config

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// NewCmdConfigKeychainDowngrade is registered on all platforms so that
// `lark-cli config --help` reads the same everywhere. On non-macOS it
// refuses with a clear message.
func NewCmdConfigKeychainDowngrade(f *cmdutil.Factory) *cobra.Command {
	_ = f
	cmd := &cobra.Command{
		Use:   "keychain-downgrade",
		Short: "Downgrade keychain storage to a local file (macOS only)",
		Long:  `Downgrade keychain storage to a local file. This subcommand is only supported on macOS; on this platform the keychain layer already uses local files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return output.ErrValidation("keychain-downgrade is only supported on macOS")
		},
	}
	return cmd
}
