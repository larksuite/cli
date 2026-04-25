// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/completion"
	cmdconfig "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/cmd/doctor"
	cmdevent "github.com/larksuite/cli/cmd/event"
	"github.com/larksuite/cli/cmd/profile"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/cmd/service"
	cmdupdate "github.com/larksuite/cli/cmd/update"
	_ "github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

type BuildOption func(*buildConfig)

type buildConfig struct {
	streams  *cmdutil.IOStreams
	keychain keychain.KeychainAccess
	globals  GlobalOptions
}

func WithIO(in io.Reader, out, errOut io.Writer) BuildOption {
	return func(c *buildConfig) {
		c.streams = cmdutil.NewIOStreams(in, out, errOut)
	}
}

func WithKeychain(kc keychain.KeychainAccess) BuildOption {
	return func(c *buildConfig) {
		c.keychain = kc
	}
}

// HideProfile keeps --profile registered but omits it from help/completion.
func HideProfile(hide bool) BuildOption {
	return func(c *buildConfig) {
		c.globals.HideProfile = hide
	}
}

// Build constructs the full command tree without executing.
func Build(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) *cobra.Command {
	_, rootCmd := buildInternal(ctx, inv, opts...)
	return rootCmd
}

func buildInternal(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) (*cmdutil.Factory, *cobra.Command) {
	cfg := &buildConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	if cfg.streams == nil {
		cfg.streams = cmdutil.SystemIO()
	}

	f := cmdutil.NewDefault(cfg.streams, inv)
	if cfg.keychain != nil {
		f.Keychain = cfg.keychain
	}
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

	RegisterGlobalFlags(rootCmd.PersistentFlags(), &cfg.globals)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}

	rootCmd.AddCommand(cmdconfig.NewCmdConfig(f))
	rootCmd.AddCommand(auth.NewCmdAuth(f))
	rootCmd.AddCommand(profile.NewCmdProfile(f))
	rootCmd.AddCommand(doctor.NewCmdDoctor(f))
	rootCmd.AddCommand(api.NewCmdApiWithContext(ctx, f, nil))
	rootCmd.AddCommand(schema.NewCmdSchema(f, nil))
	rootCmd.AddCommand(completion.NewCmdCompletion(f))
	rootCmd.AddCommand(cmdupdate.NewCmdUpdate(f))
	rootCmd.AddCommand(cmdevent.NewCmdEvents(f))
	service.RegisterServiceCommandsWithContext(ctx, rootCmd, f)
	shortcuts.RegisterShortcutsWithContext(ctx, rootCmd, f)

	if mode := f.ResolveStrictMode(ctx); mode.IsActive() {
		pruneForStrictMode(rootCmd, mode)
	}

	return f, rootCmd
}
