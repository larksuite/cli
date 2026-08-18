// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/update"
	"github.com/spf13/cobra"
)

// runRootUpgrade locates the registered `update` subcommand and runs it, so the
// interactive root-command upgrade reuses exactly `lark-cli update` behavior
// (install-method detection, output, error handling). Package-level var so
// tests can stub it and avoid real network / self-update.
var runRootUpgrade = func(cmd *cobra.Command) {
	for _, c := range cmd.Root().Commands() {
		if c.Name() == "update" && c.RunE != nil {
			_ = c.RunE(c, nil) // update prints its own output/errors; swallow here
			return
		}
	}
}

var checkRootCachedUpdate = update.CheckCached

// isBareRootInvocation reports whether this is a bare `lark-cli` (no subcommand,
// no flags) — the only invocation that triggers the interactive upgrade prompt.
// Mirrors unknownSubcommandRunE's "bare group prints help" branch: args empty
// AND no flag tokens in the raw invocation.
func isBareRootInvocation(args []string) bool {
	return len(args) == 0 && len(flagTokensInArgs(rawInvocationArgs)) == 0
}

// offerRootUpgrade prompts for an interactive upgrade when running bare
// `lark-cli` in an interactive terminal with a cached newer version. Every
// failure is swallowed — it must never affect help output or the exit code.
func offerRootUpgrade(f *cmdutil.Factory, cmd *cobra.Command, projector *recovery.Projector) {
	if f == nil || !projector.CanReference(recovery.TargetUpdate) {
		return
	}
	ios := f.IOStreams
	// Gates 1/2/3: need to read stdin AND show the prompt on stderr, and require
	// stdout TTY too so this only fires in a pure foreground terminal session.
	if !ios.IsTerminal || !ios.OutIsTerminal || !ios.StderrIsTerminal {
		return
	}
	// Gate 4: cached newer version. CheckCached applies opt-out (shouldSkip)
	// and the IsNewer/semver validation chain; it reads the on-disk cache that
	// the 24h-throttled RefreshCache maintains (CheckCached itself has no TTL).
	info := checkRootCachedUpdate(build.Version)
	if info == nil {
		return
	}
	// Deliberately no target version here: info.Latest comes from the on-disk
	// cache, which has no expiry (the 24h TTL only throttles refreshes, and a
	// failed refresh leaves the old value in place), so it can name a version
	// that is no longer the one npm would install. The version actually
	// installed is resolved live by the update subcommand, which prints
	// "Updating lark-cli <cur> -> <latest> via <pm> ..." before installing —
	// that is where the user sees the real target. Keep going through the
	// update subcommand rather than calling RunNpmInstall directly, otherwise
	// that line disappears and the user approves a global install without ever
	// being told what gets installed.
	fmt.Fprintf(ios.ErrOut, "A newer lark-cli is available (current %s). Updating automatically...\n", info.Current)
	runRootUpgrade(cmd)
}

// installRootUpgradePrompt wraps the root command's RunE (set to
// unknownSubcommandRunE by installUnknownSubcommandGuard) so a bare `lark-cli`
// invocation offers an interactive upgrade before printing help. Non-bare
// invocations are passed straight through, unchanged.
func installRootUpgradePrompt(
	f *cmdutil.Factory,
	root *cobra.Command,
	projector *recovery.Projector,
) {
	inner := root.RunE
	if inner == nil {
		return
	}
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if isBareRootInvocation(args) {
			offerRootUpgrade(f, cmd, projector)
		}
		return inner(cmd, args)
	}
}
