// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sec exposes the `lark-cli sec` command tree that bootstraps the
// lark-sec-cli sidecar daemon: install, run, stop, status, and `config init`.
// The internal/sec package owns the implementation; this package is a thin
// Cobra wrapper that mirrors the conventions in cmd/auth.
//
// After bootstrap install, lark-sec-cli handles its own upgrade lifecycle —
// lark-cli is not in the update path, which is why there's no `sec update`
// subcommand here.
package sec

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// NewCmdSec builds the parent `sec` command and registers all subcommands.
//
// The persistent --verbose / -v flag is inherited by every subcommand:
// `sec run -v`, `sec status -v`, etc. all emit step-by-step trace output to
// stderr.
//
// There is no `sec install` subcommand — `sec run` auto-installs lark-sec-cli
// if no binary is on disk, so a separate install verb was redundant.
func NewCmdSec(f *cmdutil.Factory) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "sec",
		Short: "Manage the lark-sec-cli security sidecar (run, status, stop, config)",
		Long: `Manage the lark-sec-cli security sidecar.

lark-sec-cli is a local HTTPS proxy daemon that intercepts lark-cli's traffic,
injects BDMS risk-control signatures, and manages credentials via the OS
keychain. These subcommands handle the runtime lifecycle from lark-cli's side:
start the daemon (auto-installing on first run), inspect its state, register
an app with it, and stop it. Updates after the first install are managed by
lark-sec-cli itself.`,
	}
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"print step-by-step pipeline output to stderr")
	cmd.AddCommand(NewCmdSecRun(f, nil))
	cmd.AddCommand(NewCmdSecStop(f, nil))
	cmd.AddCommand(NewCmdSecStatus(f, nil))
	cmd.AddCommand(NewCmdSecConfig(f))
	return cmd
}
