// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"io/fs"
	"strings"

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
	"github.com/larksuite/cli/cmd/whoami"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandhost"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/keychain"
	internalplatform "github.com/larksuite/cli/internal/platform"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/skillpolicy"
	"github.com/larksuite/cli/internal/skillref"
	"github.com/larksuite/cli/internal/surface"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// BuildOption configures optional aspects of the command tree construction.
type BuildOption func(*buildConfig)

type buildConfig struct {
	streams         *cmdutil.IOStreams
	keychain        keychain.KeychainAccess
	globals         GlobalOptions
	presentation    restrictionPresentationConfig
	skipPlugins     bool
	skipStrictMode  bool
	skipService     bool
	deferStartup    bool
	serviceCatalog  *apicatalog.Catalog
	startupBrand    core.LarkBrand
	startupBrandSet bool
	hideProfileSet  bool
	commandSets     []command.Set
}

// buildRuntime owns presentation state for exactly one command tree. Factory
// remains the business dependency container; distribution policy never enters
// it. The embedded pointer preserves convenient access to Factory fields in
// cmd-internal tests without exposing the surface plan to business packages.
type buildRuntime struct {
	*cmdutil.Factory
	surface         *surface.Plan
	recovery        *recovery.Projector
	skillReferences *skillref.Resolver
}

// WithStartupBrand initializes the API registry with the given brand before
// any command registration touches the runtime catalog. Without it the
// registry's sync.Once locks onto the Feishu default at first catalog access,
// long before the lazily-resolved config brand is known — see
// ResolveStartupBrand for the caller-side resolution.
func WithStartupBrand(brand core.LarkBrand) BuildOption {
	return func(c *buildConfig) {
		c.startupBrand = brand
		c.startupBrandSet = true
	}
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

// SetEmbeddedAffordanceContent registers the per-domain command guidance tree.
// Wrapper mains should wire the repository's affordance directory alongside
// embedded skills so generic --help presentation remains complete and skill
// references follow the composed distribution.
func SetEmbeddedAffordanceContent(fsys fs.FS) { affordance.SetSource(fsys) }

// HideProfile sets the visibility policy for the root-level --profile flag.
// When hide is true the flag stays registered (so existing invocations still
// parse) but is omitted from help and shell completion. Typically called as
// HideProfile(isSingleAppMode()).
func HideProfile(hide bool) BuildOption {
	return func(c *buildConfig) {
		c.globals.HideProfile = hide
		c.hideProfileSet = true
	}
}

// WithoutPlugins builds only repository-owned commands. It is intended for
// inspection tools that need a deterministic command tree.
func WithoutPlugins() BuildOption {
	return func(c *buildConfig) {
		c.skipPlugins = true
	}
}

// WithoutStrictMode builds the complete repository-owned command tree without
// applying user/profile strict-mode pruning. It is intended for offline
// inspection tools, not production execution.
func WithoutStrictMode() BuildOption {
	return func(c *buildConfig) {
		c.skipStrictMode = true
	}
}

// WithoutServiceCommands builds only hand-authored commands. It is intended for
// repository quality gates that should not depend on the remote OpenAPI
// metadata command surface.
func WithoutServiceCommands() BuildOption {
	return func(c *buildConfig) {
		c.skipService = true
	}
}

// WithServiceCatalog builds generated service commands from a specific metadata
// catalog. It is intended for offline inspection tools that need deterministic
// embedded metadata while production execution keeps using the runtime catalog.
func WithServiceCatalog(catalog apicatalog.Catalog) BuildOption {
	return func(c *buildConfig) {
		c.serviceCatalog = &catalog
	}
}

// WithCommandSets adds build-time business commands to an independently built CLI.
// The supplied declarations are copied when this option is created and compiled
// as one atomic contribution during command-tree construction.
func WithCommandSets(sets ...command.Set) BuildOption {
	captured := command.CloneSets(sets)
	return func(c *buildConfig) {
		c.commandSets = append(c.commandSets, command.CloneSets(captured)...)
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
// Returns (runtime, rootCmd, registry). The registry is nil when plugin
// install failed (FailClosed guard installed) or when no plugin produced
// hooks; callers that wire Shutdown emit must nil-check before calling
// hook.Emit.
func buildInternal(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) (*buildRuntime, *cobra.Command, *hook.Registry) {
	// cfg.globals.Profile is left zero here; it's bound to the --profile
	// flag in RegisterGlobalFlags and filled by cobra's parse step.
	cfg := &buildConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return buildInternalWithConfig(ctx, inv, cfg)
}

// resolveShortcutSnapshot compiles this build's business command sets and returns
// one snapshot carrying built-in and external shortcuts together. The error is
// returned rather than raised so the caller still mounts a root command able to
// report it; on failure the snapshot holds whatever the host could resolve.
func resolveShortcutSnapshot(sets []command.Set) ([]common.Shortcut, error) {
	external, err := commandhost.CompileSets(sets)
	if err != nil {
		return shortcuts.AllShortcuts(), err
	}
	return shortcuts.AllShortcutsWithExternal(external)
}

// buildInternalWithConfig assembles one command tree from an already-applied
// option snapshot. Execute uses this boundary so stateful BuildOptions are
// never evaluated once for bootstrap inspection and a second time for Build.
func buildInternalWithConfig(ctx context.Context, inv cmdutil.InvocationContext, cfg *buildConfig) (*buildRuntime, *cobra.Command, *hook.Registry) {
	if cfg == nil {
		cfg = &buildConfig{}
	}
	registeredShortcuts, commandSetErr := resolveShortcutSnapshot(cfg.commandSets)
	// Default streams when WithIO is not supplied so the root command's
	// SetIn/Out/Err calls below don't deref nil. NewDefault also normalizes
	// partial streams internally; keep both in sync so cfg.streams reflects
	// the same values the Factory ends up using.
	if cfg.streams == nil {
		cfg.streams = cmdutil.SystemIO()
	}
	// Initialize the registry brand before anything touches the runtime
	// catalog (its sync.Once would otherwise lock onto the Feishu default).
	if cfg.startupBrand != "" {
		registry.InitWithBrand(cfg.startupBrand)
	}

	// Reset the legacy process-global diagnostic snapshots before paths that
	// may return early. Distribution presentation state is deliberately not
	// stored here; it belongs to this build's immutable surface plan.
	cmdpolicy.SetActive(nil)
	internalplatform.SetActiveInventory(nil)

	f := cmdutil.NewDefault(cfg.streams, inv)
	if cfg.keychain != nil {
		f.Keychain = cfg.keychain
	}
	f.SkillContent = embeddedSkillContent
	runtime := &buildRuntime{Factory: f}
	runtime.recovery = recovery.NewProjectorWithContext(func() *surface.Plan {
		return runtime.surface
	}, recovery.RenderContext{Profile: inv.Profile})
	f.Recovery = runtime.recovery
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

	// Root-only usage template (curated Usage synopsis + skills footer); see
	// rootUsageTemplate.
	rootCmd.SetUsageTemplate(rootUsageTemplate)

	// Framework-generated skill pointers read this build's final content and
	// exact command surface lazily. A second Build therefore cannot rewrite
	// help rendered by the first tree.
	installTipsHelpFunc(rootCmd, func() fs.FS {
		if !runtime.surface.CanReference(surface.CommandSkillsRead) {
			return nil
		}
		return runtime.SkillContent
	}, func() *skillref.Resolver {
		return runtime.skillReferences
	}, runtime.recovery)
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

	rootCmd.AddCommand(cmdconfig.NewCmdConfigWithRecovery(f, runtime.recovery))
	rootCmd.AddCommand(auth.NewCmdAuthWithRecoveryAndShortcuts(f, runtime.recovery, registeredShortcuts))
	rootCmd.AddCommand(profile.NewCmdProfile(f))
	rootCmd.AddCommand(doctor.NewCmdDoctorWithRecovery(f, runtime.recovery))
	rootCmd.AddCommand(whoami.NewCmdWhoamiWithRecovery(f, runtime.recovery))
	rootCmd.AddCommand(api.NewCmdApiWithContext(ctx, f, nil))
	rootCmd.AddCommand(schema.NewCmdSchemaWithVisibility(f, func(path []string) bool {
		return runtime.surface.CanReference(surface.CommandID(strings.Join(path, "/")))
	}, nil))
	rootCmd.AddCommand(completion.NewCmdCompletion(f))
	rootCmd.AddCommand(cmdupdate.NewCmdUpdate(f))
	rootCmd.AddCommand(cmdevent.NewCmdEvents(f))
	rootCmd.AddCommand(skill.NewCmdSkill(f))
	if !cfg.skipService {
		if cfg.serviceCatalog != nil {
			service.RegisterServiceCommandsFromCatalog(ctx, rootCmd, f, *cfg.serviceCatalog)
		} else {
			service.RegisterServiceCommandsWithContext(ctx, rootCmd, f)
		}
	}
	shortcuts.RegisterShortcutSnapshotWithContext(ctx, rootCmd, f, registeredShortcuts)
	if commandSetErr != nil {
		installCommandSetErrorGuard(rootCmd, commandSetErr)
		return finalizeFailedBuild(runtime, rootCmd)
	}

	classifyRootCommands(rootCmd)

	installUnknownSubcommandGuard(rootCmd)
	// Bare `lark-cli` in an interactive terminal offers an interactive upgrade
	// before printing help; non-bare invocations and non-TTY are unaffected.
	installRootUpgradePrompt(f, rootCmd, runtime.recovery)

	if mode := f.ResolveStrictMode(ctx); mode.IsActive() && !cfg.skipStrictMode {
		pruneForStrictMode(rootCmd, mode)
	}

	var (
		installResult *internalplatform.InstallResult
		pluginRules   []cmdpolicy.PluginRule
		pluginSkills  []skillpolicy.PluginSkill
		hookRegistry  *hook.Registry
		denied        map[string]cmdpolicy.Denial
	)

	if !cfg.skipPlugins {
		var installErr error
		installResult, installErr = installPluginsAndHooks(cfg.streams.ErrOut)
		if installErr != nil {
			installPluginInstallErrorGuard(rootCmd, installErr)
			return finalizeFailedBuild(runtime, rootCmd)
		}
		if installResult != nil {
			pluginRules = installResult.PluginRules
			pluginSkills = installResult.PluginSkills
			hookRegistry = installResult.Registry
		}

		// Policy errors fail-CLOSED when a plugin contributed (security
		// intent must not be silently dropped); yaml-only errors fail-OPEN
		// with a warning so a typo can't lock the user out.
		var policyErr error
		denied, policyErr = applyUserPolicyPruning(rootCmd, pluginRules)
		if policyErr != nil {
			if len(pluginRules) > 0 {
				installPluginConflictGuard(rootCmd, policyErr)
				return finalizeFailedBuild(runtime, rootCmd)
			}
			warnPolicyError(cfg.streams.ErrOut, policyErr)
		}
	}

	// Presentation is an explicit host projection over the exact enforcement
	// decisions. With no opt-in, legacy Restrict and YAML policy behavior is
	// mechanically unchanged.
	var hasConcealedCommands bool
	runtime.surface, hasConcealedCommands = applyDistributionPresentation(rootCmd, cfg.presentation, denied)

	// Resolve skill assets and canonical references before installing hooks.
	// A declared customization is a build-integrity boundary: failure must
	// happen before Startup so no lifecycle side effect is stranded.
	skillResolution, skillErr := skillpolicy.ResolveWithReferences(embeddedSkillContent, pluginSkills)
	if skillErr != nil {
		installPluginSkillErrorGuard(rootCmd, skillErr)
		return finalizeFailedBuild(runtime, rootCmd)
	}
	f.SkillContent = skillResolution.Content
	runtime.skillReferences = skillResolution.References
	f.SkillReferences = skillResolution.References

	// Global flags and their environment equivalents belong to the same
	// distribution capability. Flag tokens are rejected by applyPluginFlagGate;
	// install the equivalent guard for an environment-origin profile before
	// hooks, Startup, or business commands can observe the invocation.
	if installEnvironmentProfileGate(rootCmd, inv, runtime.surface) {
		recordInventory(installResult)
		return finalizeFailedBuild(runtime, rootCmd)
	}

	// Install hooks only on business commands. The concealment-specific help
	// command is attached afterwards, preserving Cobra's historical contract
	// that help is not observed or wrapped by plugins.
	if hookRegistry != nil {
		installHooks(rootCmd, hookRegistry)
	}
	if hasConcealedCommands {
		installHelpCommand(rootCmd)
	}
	finalizeRootCommandGroups(rootCmd, runtime.surface)

	if hookRegistry != nil && !cfg.deferStartup {
		if err := emitStartup(ctx, hookRegistry); err != nil {
			installPluginLifecycleErrorGuard(rootCmd, err)
			recordInventory(installResult)
			return runtime, rootCmd, nil
		}
	}

	recordInventory(installResult)
	return runtime, rootCmd, hookRegistry
}

func finalizeFailedBuild(runtime *buildRuntime, root *cobra.Command) (*buildRuntime, *cobra.Command, *hook.Registry) {
	finalizeRootCommandGroups(root, runtime.surface)
	return runtime, root, nil
}
