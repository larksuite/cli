// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"io/fs"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/completion"
	cmdconfig "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/cmd/doctor"
	cmdevent "github.com/larksuite/cli/cmd/event"
	"github.com/larksuite/cli/cmd/profile"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/cmd/service"
	"github.com/larksuite/cli/cmd/skill"
	cmdupdate "github.com/larksuite/cli/cmd/update"
	_ "github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

// BuildOption configures optional aspects of the command tree construction.
type BuildOption func(*buildConfig)

type buildConfig struct {
	streams  *cmdutil.IOStreams
	keychain keychain.KeychainAccess
	globals  GlobalOptions
}

// WithIO sets the IO streams for the CLI by wrapping raw reader/writers.
// Terminal detection is delegated to cmdutil.NewIOStreams.
func WithIO(in io.Reader, out, errOut io.Writer) BuildOption {
	return func(c *buildConfig) {
		c.streams = cmdutil.NewIOStreams(in, out, errOut)
	}
}

// WithKeychain sets the secret storage backend. If not provided, the platform keychain is used.
func WithKeychain(kc keychain.KeychainAccess) BuildOption {
	return func(c *buildConfig) {
		c.keychain = kc
	}
}

// embeddedSkillContent is the skill tree wired into cmdutil.Factory.SkillContent
// at build time. It is registered by the repo-root package main's init via
// SetEmbeddedSkillContent — it cannot be threaded through main.go without
// breaking the single-file preview build (see skills_embed.go). nil in builds
// that embed no skills; the `skills` commands then return a typed internal error.
var embeddedSkillContent fs.FS

// SetEmbeddedSkillContent registers the embedded skill tree. Called from the
// repo-root package main's init; a wrapper main can call it before Execute to
// supply its own skill content.
func SetEmbeddedSkillContent(fsys fs.FS) { embeddedSkillContent = fsys }

// HideProfile sets the visibility policy for the root-level --profile flag.
// When hide is true the flag stays registered (so existing invocations still
// parse) but is omitted from help and shell completion. Typically called as
// HideProfile(isSingleAppMode()).
func HideProfile(hide bool) BuildOption {
	return func(c *buildConfig) {
		c.globals.HideProfile = hide
	}
}

// Build constructs the full command tree. It also installs registered
// plugins and emits the Startup lifecycle event during assembly --
// so Plugin.On(Startup) handlers run even if the returned command is
// never dispatched. The matching Shutdown event is only emitted by
// Execute; callers that bypass Execute will not see Shutdown fire.
//
// Returns only the cobra.Command; Factory and hook Registry are internal.
// Use Execute for the standard production entry point.
func Build(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) *cobra.Command {
	_, rootCmd, _ := buildInternal(ctx, inv, opts...)
	return rootCmd
}

// buildInternal is a pure assembly function: it wires the command tree from
// inv and BuildOptions alone. Any state-dependent decision (disk, network,
// env) belongs in the caller and must be threaded in via BuildOption.
//
// Returns (factory, rootCmd, registry). The registry is nil when plugin
// install failed (FailClosed guard installed) or when no plugin produced
// hooks; callers that wire Shutdown emit must nil-check before calling
// hook.Emit.
func buildInternal(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) (*cmdutil.Factory, *cobra.Command, *hook.Registry) {
	// cfg.globals.Profile is left zero here; it's bound to the --profile
	// flag in RegisterGlobalFlags and filled by cobra's parse step.
	cfg := &buildConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	// Default streams when WithIO is not supplied so the root command's
	// SetIn/Out/Err calls below don't deref nil. NewDefault also normalizes
	// partial streams internally; keep both in sync so cfg.streams reflects
	// the same values the Factory ends up using.
	if cfg.streams == nil {
		cfg.streams = cmdutil.SystemIO()
	}

	f := cmdutil.NewDefault(cfg.streams, inv)
	if cfg.keychain != nil {
		f.Keychain = cfg.keychain
	}
	f.SkillContent = embeddedSkillContent
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
	// SilenceUsage as a static field (not only in PersistentPreRun) so it also
	// covers flag-parse errors, which fail before PreRun runs — otherwise cobra
	// dumps usage instead of our structured error. SetFlagErrorFunc on root is
	// inherited by every subcommand, turning unknown-flag errors into a
	// structured "did you mean" envelope.
	rootCmd.SilenceUsage = true
	rootCmd.SetFlagErrorFunc(flagDidYouMean)

	RegisterGlobalFlags(rootCmd.PersistentFlags(), &cfg.globals)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		f.CurrentCommand = cmd
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
	rootCmd.AddCommand(skill.NewCmdSkill(f))
	service.RegisterServiceCommandsWithContext(ctx, rootCmd, f)
	shortcuts.RegisterShortcutsWithContext(ctx, rootCmd, f)

	installUnknownSubcommandGuard(rootCmd)

	if mode := f.ResolveStrictMode(ctx); mode.IsActive() {
		pruneForStrictMode(rootCmd, mode)
	}

	installResult, installErr := installPluginsAndHooks(cfg.streams.ErrOut)
	if installErr != nil {
		installPluginInstallErrorGuard(rootCmd, installErr)
		return f, rootCmd, nil
	}
	var pluginRules []cmdpolicy.PluginRule
	var registry *hook.Registry
	if installResult != nil {
		pluginRules = installResult.PluginRules
		registry = installResult.Registry
	}

	// Policy errors fail-CLOSED when a plugin contributed (security
	// intent must not be silently dropped); yaml-only errors fail-OPEN
	// with a warning so a typo can't lock the user out.
	if err := applyUserPolicyPruning(rootCmd, pluginRules); err != nil {
		if len(pluginRules) > 0 {
			installPluginConflictGuard(rootCmd, err)
			return f, rootCmd, nil
		}
		warnPolicyError(cfg.streams.ErrOut, err)
	}

	if registry != nil {
		if err := wireHooks(ctx, rootCmd, registry); err != nil {
			installPluginLifecycleErrorGuard(rootCmd, err)
			return f, rootCmd, nil
		}
	}

	recordInventory(installResult)
	return f, rootCmd, registry
}
