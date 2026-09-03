// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"context"
	"fmt"
	"slices"

	"github.com/larksuite/cli/shortcuts/okr"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/application"
	"github.com/larksuite/cli/shortcuts/apps"
	"github.com/larksuite/cli/shortcuts/base"
	"github.com/larksuite/cli/shortcuts/calendar"
	"github.com/larksuite/cli/shortcuts/common"
	contact_shortcuts "github.com/larksuite/cli/shortcuts/contact"
	"github.com/larksuite/cli/shortcuts/doc"
	"github.com/larksuite/cli/shortcuts/drive"
	"github.com/larksuite/cli/shortcuts/event"
	"github.com/larksuite/cli/shortcuts/im"
	"github.com/larksuite/cli/shortcuts/mail"
	"github.com/larksuite/cli/shortcuts/markdown"
	"github.com/larksuite/cli/shortcuts/minutes"
	"github.com/larksuite/cli/shortcuts/note"
	"github.com/larksuite/cli/shortcuts/sheets"
	"github.com/larksuite/cli/shortcuts/slides"
	"github.com/larksuite/cli/shortcuts/task"
	"github.com/larksuite/cli/shortcuts/vc"
	"github.com/larksuite/cli/shortcuts/whiteboard"
	"github.com/larksuite/cli/shortcuts/wiki"
)

// serviceAliases maps singular spellings agents habitually type onto the
// canonical service name. `slide update` was a documented dead end: the service
// is `slides`, so the invocation died before the subcommand was even considered.
var serviceAliases = map[string][]string{
	"slides": {"slide"},
}

// ServiceAliases returns the alternate spellings mounted on service's root
// command, so a routing stub created before the domain is expanded answers the
// same invocations as the expanded command.
func ServiceAliases(service string) []string {
	return append([]string(nil), serviceAliases[service]...)
}

// Empty brand (no config loaded) is treated as no-restriction so bootstrap
// paths and tests without config still see the full service list.
var brandRestrictedServices = map[string][]core.LarkBrand{
	"apps": {core.BrandFeishu},
}

func IsShortcutServiceAvailable(service string, brand core.LarkBrand) bool {
	allowed, ok := brandRestrictedServices[service]
	if !ok {
		return true
	}
	if brand == "" {
		return true
	}
	return slices.Contains(allowed, brand)
}

// allShortcuts aggregates shortcuts from all domain packages.
var allShortcuts []common.Shortcut

func init() {
	allShortcuts = append(allShortcuts, apps.Shortcuts()...)
	allShortcuts = append(allShortcuts, application.Shortcuts()...)
	allShortcuts = append(allShortcuts, calendar.Shortcuts()...)
	allShortcuts = append(allShortcuts, doc.Shortcuts()...)
	allShortcuts = append(allShortcuts, drive.Shortcuts()...)
	allShortcuts = append(allShortcuts, im.Shortcuts()...)
	allShortcuts = append(allShortcuts, contact_shortcuts.Shortcuts()...)
	allShortcuts = append(allShortcuts, sheets.Shortcuts()...)
	allShortcuts = append(allShortcuts, base.Shortcuts()...)
	allShortcuts = append(allShortcuts, event.Shortcuts()...)
	allShortcuts = append(allShortcuts, mail.Shortcuts()...)
	allShortcuts = append(allShortcuts, markdown.Shortcuts()...)
	allShortcuts = append(allShortcuts, slides.Shortcuts()...)
	allShortcuts = append(allShortcuts, minutes.Shortcuts()...)
	allShortcuts = append(allShortcuts, task.Shortcuts()...)
	allShortcuts = append(allShortcuts, vc.Shortcuts()...)
	allShortcuts = append(allShortcuts, note.Shortcuts()...)
	allShortcuts = append(allShortcuts, whiteboard.Shortcuts()...)
	allShortcuts = append(allShortcuts, wiki.Shortcuts()...)
	allShortcuts = append(allShortcuts, okr.Shortcuts()...)
}

// AllShortcuts returns an isolated copy of all registered shortcuts.
//
// This is the isolation boundary, and the only place that needs to deep-copy:
// the package global is filled once by init and never written again, but a
// Shortcut carries slice fields whose backing arrays a shallow copy would still
// share, so an external distribution mutating an element (registered[0].Flags[0])
// would corrupt the global for the whole process. Callers inside this repository
// receive an already-isolated snapshot and must not clone it again -- the copy
// costs ~165us over 500+ shortcuts, which lands on every CLI startup.
//
//go:noinline
func AllShortcuts() []common.Shortcut {
	return common.CloneHostedShortcuts(allShortcuts, commandbridge.Access{})
}

// ShortcutServiceNames returns the sorted domains that provide shortcuts.
func ShortcutServiceNames() []string {
	return shortcutServiceNamesForSnapshot(allShortcuts)
}

// AllShortcutsWithExternal returns one isolated shortcut snapshot after
// validating external path collisions.
func AllShortcutsWithExternal(commands []common.Shortcut) ([]common.Shortcut, error) {
	registered := AllShortcuts()
	external := common.CloneHostedShortcuts(commands, commandbridge.Access{})
	paths := make(map[string]struct{}, len(registered)+len(external))
	for _, shortcut := range registered {
		paths[shortcut.Service+" "+shortcut.Command] = struct{}{}
	}
	for _, shortcut := range external {
		path := shortcut.Service + " " + shortcut.Command
		if _, duplicate := paths[path]; duplicate {
			return nil, fmt.Errorf("external command path %q is already registered", path) //nolint:forbidigo // Intermediate build diagnostic wrapped by the command-set startup guard.
		}
		paths[path] = struct{}{}
	}
	return append(registered, external...), nil
}

// RegisterShortcuts registers all +shortcut commands on the program.
func RegisterShortcuts(program *cobra.Command, f *cmdutil.Factory) {
	RegisterShortcutsWithContext(context.Background(), program, f)
}

func RegisterShortcutsWithContext(ctx context.Context, program *cobra.Command, f *cmdutil.Factory) {
	RegisterShortcutSnapshotWithContext(ctx, program, f, AllShortcuts())
}

// RegisterShortcutsForDomainsWithContext registers shortcuts from the selected
// domains. A nil selection mounts every domain; a non-nil empty selection
// mounts none.
func RegisterShortcutsForDomainsWithContext(
	ctx context.Context,
	program *cobra.Command,
	f *cmdutil.Factory,
	domains []string,
) {
	RegisterShortcutSnapshotForDomainsWithContext(ctx, program, f, AllShortcuts(), domains)
}

// RegisterShortcutSnapshotWithContext mounts one build-local shortcut snapshot.
func RegisterShortcutSnapshotWithContext(
	ctx context.Context,
	program *cobra.Command,
	f *cmdutil.Factory,
	registered []common.Shortcut,
) {
	RegisterShortcutSnapshotForDomainsWithContext(ctx, program, f, registered, nil)
}

// RegisterShortcutSnapshotForDomainsWithContext mounts the selected domains
// from one build-local shortcut snapshot. A nil selection mounts every domain;
// a non-nil empty selection mounts none.
func RegisterShortcutSnapshotForDomainsWithContext(
	ctx context.Context,
	program *cobra.Command,
	f *cmdutil.Factory,
	registered []common.Shortcut,
	domains []string,
) {
	byService := make(map[string][]common.Shortcut)
	for _, shortcut := range registered {
		byService[shortcut.Service] = append(byService[shortcut.Service], shortcut)
	}
	selectedServices := selectShortcutServices(byService, domains)
	if len(selectedServices) == 0 {
		return
	}

	// Factory.Config may be nil in tests that pass a zero-value factory.
	var brand core.LarkBrand
	if f != nil && f.Config != nil {
		if cfg, err := f.Config(); err == nil && cfg != nil {
			brand = cfg.Brand
		}
	}

	for _, service := range selectedServices {
		serviceShortcuts := byService[service]
		// Find existing service command or create one
		var svc *cobra.Command
		for _, c := range program.Commands() {
			if c.Name() == service {
				svc = c
				break
			}
		}
		if svc == nil {
			svc = &cobra.Command{Use: service}
			program.AddCommand(svc)
		}
		// A pre-mounted routing stub carries no description yet; a service
		// command registered by cmd/service keeps its own.
		if svc.Short == "" {
			desc := registry.GetServiceDescription(service, "en")
			if desc == "" {
				desc = service + " operations"
			}
			svc.Short = desc
		}
		// Tag the service group with its domain so platform.ByDomain
		// and Rule.Allow path-globs work without each leaf shortcut
		// having to declare the domain itself: cmdmeta.Domain walks up
		// the parent chain and stops at the first annotated ancestor
		// (this command).
		//
		// Done OUTSIDE the create branch so the tag is still applied
		// when the service command was pre-created by cmd/service
		// (OpenAPI auto-registration adds im, drive, calendar, etc.
		// before shortcuts run). Without this, only pure-shortcut
		// services like `docs` would get tagged.
		cmdmeta.SetDomain(svc, service)
		// Applied OUTSIDE the create branch for the same reason as the domain tag:
		// OpenAPI auto-registration has usually created the service command
		// already, so setting Aliases only where the command is constructed would
		// be dead code — it compiles, the tests pass, and `lark-cli slide …` still
		// answers "unknown command".
		for _, alias := range serviceAliases[service] {
			if !slices.Contains(svc.Aliases, alias) {
				svc.Aliases = append(svc.Aliases, alias)
			}
		}
		for _, shortcut := range serviceShortcuts {
			shortcut.MountWithContext(ctx, svc, f)
		}
		if service == "apps" {
			apps.InstallOnApps(svc, f)
		}
		if service == "mail" {
			mail.InstallOnMail(svc)
		}
		if service == "sheets" {
			applySheetsCommandGroups(svc)
			sheets.InstallUnknownSubcommandHints(svc)
		}

		if !IsShortcutServiceAvailable(service, brand) {
			installBrandRestrictionGuard(svc, service, brand)
		}
	}
}

func selectShortcutServices(byService map[string][]common.Shortcut, domains []string) []string {
	if domains == nil {
		services := make([]string, 0, len(byService))
		for service := range byService {
			services = append(services, service)
		}
		slices.Sort(services)
		return services
	}

	selected := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		if _, ok := byService[domain]; ok {
			selected[domain] = struct{}{}
		}
	}

	services := make([]string, 0, len(selected))
	for service := range selected {
		services = append(services, service)
	}
	slices.Sort(services)
	return services
}

func shortcutServiceNamesForSnapshot(registered []common.Shortcut) []string {
	seen := make(map[string]struct{})
	for _, shortcut := range registered {
		seen[shortcut.Service] = struct{}{}
	}
	services := make([]string, 0, len(seen))
	for service := range seen {
		services = append(services, service)
	}
	slices.Sort(services)
	return services
}

// Mirrors internal/cmdpolicy/apply.go::installDenyStub: DisableFlagParsing +
// ArbitraryArgs keep cobra from short-circuiting with "missing required flag"
// before our RunE runs; leaf-level PersistentPreRunE defeats cobra's "first
// PreRunE wins" walk-up that would otherwise shadow the stub.
func installBrandRestrictionGuard(svc *cobra.Command, service string, brand core.LarkBrand) {
	stub := func(c *cobra.Command, _ []string) error {
		c.SilenceUsage = true
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"the %q feature is not yet supported on the %s brand",
			service, brand,
		)
	}
	noopPreRun := func(c *cobra.Command, _ []string) error {
		c.SilenceUsage = true
		return nil
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Hidden = true
		c.DisableFlagParsing = true
		c.Args = cobra.ArbitraryArgs
		c.PreRunE = nil
		c.PreRun = nil
		c.PersistentPreRunE = noopPreRun
		c.PersistentPreRun = nil
		c.RunE = stub
		c.Run = nil
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(svc)

	// --help bypasses RunE, so surface the restriction in Long too.
	svc.Long = fmt.Sprintf("The %q feature is not yet supported on the %s brand.", service, brand)
}

// Sheets help grouping.
//
// The sheets service mounts two kinds of subcommand on the same cobra parent:
// this repository's "+"-prefixed shortcuts and the auto-registered OpenAPI
// metaapi subcommands (spreadsheets, ...). applySheetsCommandGroups tags only
// the former into a named group so cobra files the latter under its stock
// "Additional Commands" heading instead of interleaving them.
const sheetsCurrentGroupID = "sheets-current"

func applySheetsCommandGroups(svc *cobra.Command) {
	svc.AddGroup(&cobra.Group{ID: sheetsCurrentGroupID, Title: "Available Commands:"})

	for _, c := range svc.Commands() {
		// Only the shortcuts (all "+"-prefixed) belong in the group. Leave the
		// OpenAPI metaapi subcommands (spreadsheets, ...) and the auto-added
		// help/completion ungrouped so cobra files them under "Additional
		// Commands".
		if name := c.Name(); len(name) > 0 && name[0] == '+' {
			c.GroupID = sheetsCurrentGroupID
		}
	}
}
