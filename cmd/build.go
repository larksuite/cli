// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/completion"
	cmdconfig "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/cmd/doctor"
	"github.com/larksuite/cli/cmd/profile"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/cmd/service"
	cmdupdate "github.com/larksuite/cli/cmd/update"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

// BuildOption configures optional aspects of the command tree construction.
type BuildOption func(*buildConfig)

type buildConfig struct {
	streams  *cmdutil.IOStreams
	keychain keychain.KeychainAccess
}

// WithIO sets the IO streams for the CLI. If not provided, os.Stdin/Stdout/Stderr are used.
func WithIO(in io.Reader, out, errOut io.Writer) BuildOption {
	return func(c *buildConfig) {
		isTerminal := false
		if f, ok := in.(*os.File); ok {
			isTerminal = term.IsTerminal(int(f.Fd()))
		}
		c.streams = &cmdutil.IOStreams{In: in, Out: out, ErrOut: errOut, IsTerminal: isTerminal}
	}
}

// WithKeychain sets the secret storage backend. If not provided, the platform keychain is used.
func WithKeychain(kc keychain.KeychainAccess) BuildOption {
	return func(c *buildConfig) {
		c.keychain = kc
	}
}

// Build constructs the full command tree without executing.
// Returns only the cobra.Command; Factory is internal.
// Use Execute for the standard production entry point.
func Build(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) *cobra.Command {
	_, rootCmd := buildInternal(ctx, inv, opts...)
	return rootCmd
}

// buildInternal is the internal constructor that also returns Factory for error handling.
func buildInternal(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) (*cmdutil.Factory, *cobra.Command) {
	cfg := &buildConfig{
		streams: cmdutil.SystemIO(),
	}
	for _, o := range opts {
		o(cfg)
	}

	f := cmdutil.NewDefault(cfg.streams, inv)
	if cfg.keychain != nil {
		f.Keychain = cfg.keychain
	}

	globals := &GlobalOptions{Profile: inv.Profile}
	rootCmd := &cobra.Command{
		Use:     "lark-cli",
		Short:   "Lark/Feishu CLI — OAuth authorization, UAT management, API calls",
		Long:    rootLong,
		Version: build.Version,
	}

	rootCmd.SetContext(ctx)
	rootCmd.SetIn(cfg.streams.In)
	rootCmd.SetOut(cfg.streams.Out)
	rootCmd.SetErr(cfg.streams.ErrOut)

	installTipsHelpFunc(rootCmd)
	rootCmd.SilenceErrors = true

	RegisterGlobalFlags(rootCmd.PersistentFlags(), globals)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}

	rootCmd.AddCommand(cmdconfig.NewCmdConfig(f))
	rootCmd.AddCommand(auth.NewCmdAuth(f))
	rootCmd.AddCommand(profile.NewCmdProfile(f))
	rootCmd.AddCommand(doctor.NewCmdDoctor(f))
	rootCmd.AddCommand(api.NewCmdApi(f, nil))
	rootCmd.AddCommand(schema.NewCmdSchema(f, nil))
	rootCmd.AddCommand(completion.NewCmdCompletion(f))
	rootCmd.AddCommand(cmdupdate.NewCmdUpdate(f))
	service.RegisterServiceCommands(rootCmd, f)
	shortcuts.RegisterShortcuts(rootCmd, f)

	// Prune commands incompatible with strict mode.
	if mode := f.ResolveStrictMode(ctx); mode.IsActive() {
		pruneForStrictMode(rootCmd, mode)
	}

	return f, rootCmd
}
