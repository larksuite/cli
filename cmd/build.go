// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"io/fs"
	"sort"
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
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdmeta"
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
	streams        *cmdutil.IOStreams
	keychain       keychain.KeychainAccess
	globals        GlobalOptions
	invocationArgs []string
	presentation   restrictionPresentationConfig
	skipPlugins    bool
	skipStrictMode bool
	skipService    bool
	deferStartup   bool
	apiCatalog     *apicatalog.Catalog
	// catalogOpener yields this build's lazy Catalog handle. Opening reads only
	// the manifest; service bodies are parsed on first navigation.
	catalogOpener    func() (apicatalog.Catalog, error)
	pluginProvider   func() []platform.Plugin
	afterCatalogOpen func()
	hideProfileSet   bool
	commandSets      []command.Set
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
	help            *service.HelpRenderer
}

// WithStartupBrand is retained for source compatibility with wrapper mains.
// Deprecated: the committed API catalog is brand-independent, so this option
// no longer changes command construction.
func WithStartupBrand(_ core.LarkBrand) BuildOption {
	return func(*buildConfig) {}
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

// WithInvocationArgs enables target-aware assembly for Build. The provided
// arguments are used both to select the Catalog and Shortcut domains and as
// Cobra's execution arguments, keeping assembly and dispatch on one source of
// truth. Build remains a full-tree constructor when this option is omitted.
//
// An explicitly empty slice represents a bare invocation and therefore still
// assembles the complete tree for root help. The slice is copied so later
// caller mutation cannot change the planned or executed command.
func WithInvocationArgs(args []string) BuildOption {
	return func(c *buildConfig) {
		c.invocationArgs = append(make([]string, 0, len(args)), args...)
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
// repository quality gates that should not depend on the generated API command
// surface of the embedded Catalog.
func WithoutServiceCommands() BuildOption {
	return func(c *buildConfig) {
		c.skipService = true
	}
}

// WithServiceCatalog uses catalog as the authoritative metadata for the entire
// command build instead of opening the embedded snapshot. It is primarily
// intended for deterministic inspection tools and tests.
func WithServiceCatalog(catalog apicatalog.Catalog) BuildOption {
	return func(c *buildConfig) {
		c.apiCatalog = &catalog
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

// Build constructs the full command tree by default. When
// WithInvocationArgs is provided, it constructs only the command domains that
// those arguments can reach and configures Cobra to execute the same arguments.
//
// Build also installs registered plugins and emits the Startup lifecycle event
// during assembly -- so Plugin.On(Startup) handlers run even if the returned
// command is never dispatched. The matching Shutdown event is only emitted by
// Execute; callers that bypass Execute will not see Shutdown fire.
//
// Returns only the cobra.Command; Factory and hook Registry are internal.
// Use Execute for the standard production entry point.
func Build(ctx context.Context, inv cmdutil.InvocationContext, opts ...BuildOption) *cobra.Command {
	cfg := resolveBuildConfig(opts)
	if cfg.invocationArgs != nil {
		result, err := buildForArgsWithConfig(ctx, inv, cfg.invocationArgs, cfg)
		if err != nil {
			result = failedCatalogBuild(ctx, inv, cfg, err)
		}
		result.root.SetArgs(append([]string(nil), cfg.invocationArgs...))
		return result.root
	}
	_, rootCmd, _ := buildInternalWithConfig(ctx, inv, cfg)
	return rootCmd
}

// buildResult is what one assembly produces. The registry is nil when plugin
// install failed (a FailClosed guard is installed) or no plugin produced hooks;
// callers that wire Shutdown emit must nil-check before calling hook.Emit.
type buildResult struct {
	runtime  *buildRuntime
	root     *cobra.Command
	registry *hook.Registry
}

// buildForArgsWithConfig constructs only the command domains that args can
// reach. Cobra remains responsible for parsing and executing args after
// assembly. A Catalog failure is returned typed so the caller can substitute
// failedCatalogBuild.
func buildForArgsWithConfig(
	ctx context.Context,
	inv cmdutil.InvocationContext,
	args []string,
	cfg *buildConfig,
) (*buildResult, error) {
	cfg = normalizeBuildConfig(cfg)
	runtime, root, reg, err := assembleInternal(ctx, inv, assemblyRequest{routed: true, args: args}, cfg)
	if err != nil {
		return nil, err
	}
	return &buildResult{runtime: runtime, root: root, registry: reg}, nil
}

// failedCatalogBuild is the fail-closed assembly used when the Catalog cannot
// be opened: a root whose every dispatch reports err, over a runtime with an
// empty surface so recovery hints reference nothing that was not built. Every
// build entry point that can hit a Catalog failure substitutes this result so
// the recovery wiring cannot drift between them.
func failedCatalogBuild(ctx context.Context, inv cmdutil.InvocationContext, cfg *buildConfig, err error) *buildResult {
	f := cmdutil.NewDefault(cfg.streams, inv)
	runtime := &buildRuntime{Factory: f, surface: surface.NewPlan(nil)}
	runtime.recovery = recovery.NewProjector(func() *surface.Plan { return runtime.surface })
	f.Recovery = runtime.recovery
	return &buildResult{runtime: runtime, root: newCatalogFailureRoot(ctx, cfg, err)}
}

func resolveBuildConfig(opts []BuildOption) *buildConfig {
	cfg := &buildConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return normalizeBuildConfig(cfg)
}

func normalizeBuildConfig(cfg *buildConfig) *buildConfig {
	if cfg == nil {
		cfg = &buildConfig{}
	}
	if cfg.streams == nil {
		cfg.streams = cmdutil.SystemIO()
	}
	if cfg.catalogOpener == nil {
		cfg.catalogOpener = openEmbeddedCatalog
	}
	return cfg
}

func openEmbeddedCatalog() (apicatalog.Catalog, error) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		return apicatalog.Catalog{}, err
	}
	return snapshot.Catalog(), nil
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
	return buildInternalWithConfig(ctx, inv, resolveBuildConfig(opts))
}

func frozenPlugins(cfg *buildConfig) []platform.Plugin {
	if cfg.skipPlugins {
		return nil
	}
	if cfg.pluginProvider != nil {
		return cfg.pluginProvider()
	}
	return platform.RegisteredPlugins()
}

// resolveShortcutSnapshot compiles this build's business command sets and returns
// one snapshot carrying built-in and external shortcuts together. On failure the
// built-in snapshot is returned so assembly can install a fail-closed guard.
func resolveShortcutSnapshot(sets []command.Set) ([]common.Shortcut, error) {
	external, err := commandhost.CompileSets(sets)
	if err != nil {
		return shortcuts.AllShortcuts(), err
	}
	return shortcuts.AllShortcutsWithExternal(external)
}

// buildInternalWithConfig assembles the complete command tree from an
// already-applied option snapshot. Target-aware callers use
// buildForArgsWithConfig instead.
func buildInternalWithConfig(ctx context.Context, inv cmdutil.InvocationContext, cfg *buildConfig) (*buildRuntime, *cobra.Command, *hook.Registry) {
	cfg = normalizeBuildConfig(cfg)
	runtime, root, reg, err := assembleInternal(ctx, inv, assemblyRequest{}, cfg)
	if err != nil {
		failed := failedCatalogBuild(ctx, inv, cfg, err)
		return failed.runtime, failed.root, failed.registry
	}
	return runtime, root, reg
}

func openCatalog(cfg *buildConfig) (apicatalog.Catalog, error) {
	if cfg.apiCatalog != nil {
		return *cfg.apiCatalog, nil
	}
	return cfg.catalogOpener()
}

func newCatalogFailureRoot(ctx context.Context, cfg *buildConfig, catalogErr error) *cobra.Command {
	root := &cobra.Command{
		Use:           "lark-cli",
		Short:         "Lark/Feishu CLI — OAuth authorization, UAT management, API calls",
		Long:          rootLong,
		Version:       build.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(*cobra.Command, []string) error {
			return catalogErr
		},
	}
	root.SetContext(ctx)
	root.SetIn(cfg.streams.In)
	root.SetOut(cfg.streams.Out)
	root.SetErr(cfg.streams.ErrOut)
	root.DisableFlagParsing = true
	return root
}

// assemblyRequest says how much of the domain surface one build must expand.
// The zero value assembles every domain; a routed request lets Cobra decide
// which single domain args reach (see routeDomains) and expands only that one.
type assemblyRequest struct {
	routed bool
	args   []string
}

// assembleInternal is a pure assembly function over the frozen plugin snapshot
// and this build's shortcut snapshot. It never reloads either of them.
//
// Domain expansion happens in two steps so routing and dispatch share one
// parser: every domain is first mounted as an empty stub, Cobra's own Find
// picks the stub (or hand-authored command) args reach, and only that domain's
// Catalog shard and shortcuts are expanded onto the very same *cobra.Command.
// Unselected stubs are removed before any later stage sees the tree, so the
// target tree is a subtree of the full tree by construction.
//
// Returns (runtime, rootCmd, registry). The registry is nil when plugin
// install failed (FailClosed guard installed) or when no plugin produced
// hooks; callers that wire Shutdown emit must nil-check before calling
// hook.Emit. A non-nil error is a Catalog open or shard failure; the caller
// substitutes a fail-closed root.
func assembleInternal(
	ctx context.Context,
	inv cmdutil.InvocationContext,
	request assemblyRequest,
	cfg *buildConfig,
) (*buildRuntime, *cobra.Command, *hook.Registry, error) {
	// cfg.globals.Profile is left zero here; it's bound to the --profile
	// flag in RegisterGlobalFlags and filled by cobra's parse step.

	// Reset the legacy process-global diagnostic snapshots before paths that
	// may return early. Distribution presentation state is deliberately not
	// stored here; it belongs to this build's immutable surface plan.
	cmdpolicy.SetActive(nil)
	internalplatform.SetActiveInventory(nil)

	// Plugins are frozen before the Catalog opens. Any registered plugin
	// receives the complete service tree so its policy and hook expectations
	// cannot be bypassed by a target-only assembly; the version-only path is
	// the one exception, since it never opens the Catalog at all.
	plugins := frozenPlugins(cfg)
	registeredShortcuts, commandSetErr := resolveShortcutSnapshot(cfg.commandSets)

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

	// Version is the only deterministic no-Catalog invocation: it must succeed
	// even when the embedded Catalog is corrupt. Plugins are still installed
	// below, and their Startup hooks run against the repository-owned root.
	var catalog apicatalog.Catalog
	versionOnly := request.routed && isVersionOnlyInvocation(rootCmd, request.args)
	if !versionOnly {
		var err error
		catalog, err = openCatalog(cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		if cfg.afterCatalogOpen != nil {
			cfg.afterCatalogOpen()
		}
	}
	f.APICatalog = catalog
	f.Affordance = affordance.NewResolver(affordance.Source(), catalog)

	// Framework-generated help reads this build's final content and exact
	// command surface lazily. A second Build therefore cannot rewrite help
	// rendered by the first tree.
	runtime.help = &service.HelpRenderer{
		Guidance: f.Affordance,
		SkillContent: func() fs.FS {
			if !runtime.surface.CanReference(surface.CommandSkillsRead) {
				return nil
			}
			return runtime.SkillContent
		},
		SkillReferences: func() *skillref.Resolver { return runtime.skillReferences },
		CanReferenceSchema: func() bool {
			return runtime.recovery == nil || runtime.recovery.CanReference(recovery.TargetSchema)
		},
	}
	installTipsHelpFunc(rootCmd, runtime.help)

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

	var catalogNames []string
	if !cfg.skipService {
		catalogNames = catalog.Names()
	}
	shortcutDomains := shortcuts.ServiceNamesOf(registeredShortcuts)
	stubs := mountDomainStubs(rootCmd, catalogNames, shortcutDomains)

	selected := selectAllDomains
	switch {
	case versionOnly:
		selected = selectNoDomains
	case request.routed:
		selected = routeDomains(rootCmd, request.args, catalogNames, shortcutDomains)
		if len(plugins) > 0 {
			selected = selectAllDomains
		}
	}

	if !cfg.skipService {
		names := selected.pick(catalogNames)
		if err := catalog.Preload(names...); err != nil {
			return nil, nil, nil, err
		}
		service.RegisterServiceCommandsForNames(ctx, rootCmd, f, catalog, names)
	}
	shortcuts.RegisterShortcutSnapshotForDomainsWithContext(ctx, rootCmd, f, registeredShortcuts, selected.shortcutSelection())
	for name, stub := range stubs {
		if !selected.includes(name) {
			rootCmd.RemoveCommand(stub)
		}
	}

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
		installResult, installErr = installPluginsAndHooks(plugins, cfg.streams.ErrOut)
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
			return runtime, rootCmd, nil, nil
		}
	}

	recordInventory(installResult)
	return runtime, rootCmd, hookRegistry, nil
}

func finalizeFailedBuild(runtime *buildRuntime, root *cobra.Command) (*buildRuntime, *cobra.Command, *hook.Registry, error) {
	finalizeRootCommandGroups(root, runtime.surface)
	return runtime, root, nil, nil
}

// isVersionOnlyInvocation reports whether args ask Cobra for nothing but the
// root version: the root flag set parses them cleanly, --version is set, --help
// is not, and no positional token remains that could name a command. Cobra's
// own execute path makes the same decision, so this never diverges from what
// Execute would print.
//
// The probe parses on a throwaway root carrying the same RegisterGlobalFlags
// definitions plus Cobra's default help/version flags. Parsing the real root
// would leave its flag values set, and a fail-closed guard later disables flag
// parsing on that root, so a stale value there could let --version bypass the
// guard.
func isVersionOnlyInvocation(root *cobra.Command, args []string) bool {
	// Two Cobra decisions must both land on the root before the Catalog can be
	// skipped. Find runs first, before --help/--version exist, on a root that
	// has subcommands: `--version --profile x` swallows --profile there and
	// leaves x as an unknown command whose error lists the full tree, so it is
	// not a version-only invocation even though the flag parse below says so.
	dispatch := &cobra.Command{Use: root.Use}
	RegisterGlobalFlags(dispatch.PersistentFlags(), &GlobalOptions{})
	dispatch.AddCommand(&cobra.Command{Use: "placeholder"})
	if target, _, err := dispatch.Find(args); err != nil || target != dispatch {
		return false
	}

	probe := &cobra.Command{Use: root.Use, Version: root.Version}
	RegisterGlobalFlags(probe.PersistentFlags(), &GlobalOptions{})
	probe.InitDefaultHelpFlag()
	probe.InitDefaultVersionFlag()
	if err := probe.ParseFlags(args); err != nil {
		return false
	}
	flags := probe.Flags()
	if len(flags.Args()) != 0 {
		return false
	}
	return flags.Changed("version") && !flags.Changed("help")
}

// domainSelection is the outcome of routing: which mounted domains to expand.
// A nil set with all=false expands nothing (hand-authored or version targets).
type domainSelection struct {
	all   bool
	names map[string]struct{}
}

var (
	selectAllDomains = domainSelection{all: true}
	selectNoDomains  = domainSelection{}
)

func selectDomain(name string) domainSelection {
	return domainSelection{names: map[string]struct{}{name: {}}}
}

func (s domainSelection) includes(name string) bool {
	if s.all {
		return true
	}
	_, ok := s.names[name]
	return ok
}

// pick returns the subset of candidates this selection expands, preserving
// candidate order. It never returns nil for an empty selection so callers can
// distinguish "none" from the shortcut registrar's nil-means-all convention.
func (s domainSelection) pick(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if s.includes(name) {
			out = append(out, name)
		}
	}
	return out
}

// shortcutSelection translates to RegisterShortcutSnapshotForDomainsWithContext's
// contract: nil mounts every domain, a non-nil empty slice mounts none.
func (s domainSelection) shortcutSelection() []string {
	if s.all {
		return nil
	}
	out := make([]string, 0, len(s.names))
	for name := range s.names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// mountDomainStubs adds one empty command per targetable domain so Cobra can
// route to it. A domain whose name is already a hand-authored root command
// (event) is a shared root: the shortcuts expand onto the existing command, so
// no stub is needed. Stubs carry only routing identity (name and aliases);
// expansion fills description and children, and unselected stubs are removed.
func mountDomainStubs(root *cobra.Command, catalogNames, shortcutDomains []string) map[string]*cobra.Command {
	existing := make(map[string]struct{})
	for _, cmd := range root.Commands() {
		existing[cmd.Name()] = struct{}{}
	}
	stubs := make(map[string]*cobra.Command)
	mount := func(name string) {
		if _, taken := existing[name]; taken {
			return
		}
		existing[name] = struct{}{}
		stub := &cobra.Command{Use: name, Aliases: shortcuts.ServiceAliases(name)}
		root.AddCommand(stub)
		stubs[name] = stub
	}
	for _, name := range catalogNames {
		mount(name)
	}
	for _, name := range shortcutDomains {
		mount(name)
	}
	return stubs
}

// routeDomains lets Cobra's Find decide which root child args reach and maps
// it to a domain selection. Find is called on the root exactly as ExecuteC
// will call it: no flag is registered here that ExecuteC would not have
// registered at that point (in particular --help/--version are added only
// after Find), so routing cannot disagree with execution. Anything Find cannot
// resolve — the bare root, a leading --help/--version, an unknown first token —
// conservatively expands every domain so Cobra's real error, help, and version
// paths see the complete tree.
func routeDomains(root *cobra.Command, args []string, catalogNames, shortcutDomains []string) domainSelection {
	target, _, err := root.Find(args)
	if err != nil || target == nil || target == root {
		return selectAllDomains
	}
	if cmdmeta.RequiresFullTree(target) {
		return selectAllDomains
	}
	for target.Parent() != root {
		target = target.Parent()
	}
	name := target.Name()
	for _, domain := range catalogNames {
		if domain == name {
			return selectDomain(name)
		}
	}
	for _, domain := range shortcutDomains {
		if domain == name {
			return selectDomain(name)
		}
	}
	return selectNoDomains
}
