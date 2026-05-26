// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package config

import (
	"fmt"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// NewCmdConfigKeychainDowngrade creates the macOS-only subcommand that pins
// the master key to the local file fallback (master.key.file) so subsequent
// operations bypass the OS Keychain. Useful inside sandboxes like Codex
// where the system Keychain is unreachable.
func NewCmdConfigKeychainDowngrade(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keychain-downgrade",
		Short: "Downgrade keychain storage to a local file (macOS only)",
		Long: `Materialize the master key from the macOS system Keychain into a local file
under ~/Library/Application Support/lark-cli/master.key.file, then pin all
subsequent reads to that file.

Intended workflow: run this once from an interactive Terminal session on
macOS (where the system Keychain is reachable). After it finishes,
sandboxed / automation / CI runs of lark-cli on the same machine will read
the master key from the local file and no longer need the OS Keychain.

This is the supported fix for environments like the Codex sandbox where the
system Keychain is blocked. Running keychain-downgrade from inside such a
sandbox will itself fail with "keychain access blocked" — that is expected;
run it from an interactive macOS session instead.

The OS Keychain entry is preserved as a cold backup; nothing is deleted there.
The command is idempotent: re-running it on an already-downgraded install
reports "already downgraded" and exits 0.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configKeychainDowngradeRun(f)
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func configKeychainDowngradeRun(f *cmdutil.Factory) error {
	service := keychain.LarkCliService
	keyPath := keychain.MasterKeyFilePath(service)

	result, err := keychain.DowngradeMasterKeyToFile(service)
	if err != nil {
		return output.ErrWithHint(
			output.ExitAPI,
			"config",
			fmt.Sprintf("keychain downgrade failed: %v", err),
			"This command must be run from an interactive macOS session (e.g. Terminal.app or iTerm) where the system Keychain is reachable. Running it from inside a sandbox / automation context that blocks Keychain access cannot succeed by design.",
		)
	}

	switch result {
	case keychain.DowngradeAlreadyDone:
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("keychain already downgraded; subsequent operations read from %s", keyPath))
	case keychain.DowngradeUsedKeychainKey:
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("downgraded: copied master key from system Keychain to %s. Subsequent operations will read from file, bypassing the OS Keychain (useful inside sandboxes like Codex).", keyPath))
	case keychain.DowngradeCreatedNewKey:
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("system Keychain was empty; generated a new master key and wrote it to %s. The OS Keychain was not modified.", keyPath))
	}
	return nil
}
