// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"context"
	"fmt"
	"slices"

	"github.com/larksuite/cli/shortcuts/okr"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
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
	"github.com/larksuite/cli/shortcuts/sheets"
	"github.com/larksuite/cli/shortcuts/slides"
	"github.com/larksuite/cli/shortcuts/task"
	"github.com/larksuite/cli/shortcuts/vc"
	"github.com/larksuite/cli/shortcuts/whiteboard"
	"github.com/larksuite/cli/shortcuts/wiki"
)

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

// allShortcuts aggregates shortcuts from all domain packages. Each entry is a
// Mountable: both legacy common.Shortcut (via *Shortcut after the descriptor
// accessor methods landed) and the new generic common.TypedShortcut[T] satisfy
// it. The slice element type is ShortcutDescriptor so read-only consumers
// (auth login, scope hint, shortcuts.json generator) can read metadata without
// caring about which concrete implementation backs each entry.
//
// Only legacy shortcuts are registered today; the typed track stays wired
// (ShortcutDescriptor / Mountable dispatch + buildTypedHelp) but currently has
// no domain caller.
var allShortcuts []common.ShortcutDescriptor

// addLegacy boxes a legacy []common.Shortcut into the descriptor slice. We use
// pointer-valued elements because the pointer-receiver scope methods
// (ScopesForIdentity / ConditionalScopesForIdentity / DeclaredScopesForIdentity)
// are required by ShortcutDescriptor — value receivers don't satisfy them.
func addLegacy(list []common.Shortcut) {
	for i := range list {
		allShortcuts = append(allShortcuts, &list[i])
	}
}

func init() {
	addLegacy(apps.Shortcuts())
	addLegacy(calendar.Shortcuts())
	addLegacy(doc.Shortcuts())
	addLegacy(drive.Shortcuts())
	addLegacy(im.Shortcuts())
	addLegacy(contact_shortcuts.Shortcuts())
	addLegacy(sheets.Shortcuts())
	addLegacy(base.Shortcuts())
	addLegacy(event.Shortcuts())
	addLegacy(mail.Shortcuts())
	addLegacy(markdown.Shortcuts())
	addLegacy(slides.Shortcuts())
	addLegacy(minutes.Shortcuts())
	addLegacy(task.Shortcuts())
	addLegacy(vc.Shortcuts())
	addLegacy(whiteboard.Shortcuts())
	addLegacy(wiki.Shortcuts())
	addLegacy(okr.Shortcuts())
}

// AllShortcuts returns a copy of all registered shortcut descriptors (for
// dump-shortcuts and auth/scope-hint consumers).
//
//go:noinline
func AllShortcuts() []common.ShortcutDescriptor {
	return append([]common.ShortcutDescriptor(nil), allShortcuts...)
}

// RegisterShortcuts registers all +shortcut commands on the program.
func RegisterShortcuts(program *cobra.Command, f *cmdutil.Factory) {
	RegisterShortcutsWithContext(context.Background(), program, f)
}

func RegisterShortcutsWithContext(ctx context.Context, program *cobra.Command, f *cmdutil.Factory) {
	// Factory.Config may be nil in tests that pass a zero-value factory.
	var brand core.LarkBrand
	if f != nil && f.Config != nil {
		if cfg, err := f.Config(); err == nil && cfg != nil {
			brand = cfg.Brand
		}
	}

	// Group by service. Each entry is a Mountable so MountWithContext works
	// uniformly across legacy *Shortcut and TypedShortcut[T].
	byService := make(map[string][]common.Mountable)
	for _, d := range allShortcuts {
		m, ok := d.(common.Mountable)
		if !ok {
			panic(fmt.Sprintf("shortcut %s/%s missing Mountable", d.GetService(), d.GetCommand()))
		}
		byService[d.GetService()] = append(byService[d.GetService()], m)
	}

	for service, shortcuts := range byService {
		// Find existing service command or create one
		var svc *cobra.Command
		for _, c := range program.Commands() {
			if c.Name() == service {
				svc = c
				break
			}
		}
		if svc == nil {
			desc := registry.GetServiceDescription(service, "en")
			if desc == "" {
				desc = service + " operations"
			}
			svc = &cobra.Command{
				Use:   service,
				Short: desc,
			}
			program.AddCommand(svc)
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
		if service == "docs" {
			doc.ConfigureServiceHelp(svc)
		}

		for _, m := range shortcuts {
			m.MountWithContext(ctx, svc, f)
		}
		if service == "mail" {
			mail.InstallOnMail(svc)
		}

		if !IsShortcutServiceAvailable(service, brand) {
			installBrandRestrictionGuard(svc, service, brand)
		}
	}
}

// Mirrors internal/cmdpolicy/apply.go::installDenyStub: DisableFlagParsing +
// ArbitraryArgs keep cobra from short-circuiting with "missing required flag"
// before our RunE runs; leaf-level PersistentPreRunE defeats cobra's "first
// PreRunE wins" walk-up that would otherwise shadow the stub.
func installBrandRestrictionGuard(svc *cobra.Command, service string, brand core.LarkBrand) {
	stub := func(c *cobra.Command, _ []string) error {
		c.SilenceUsage = true
		return output.ErrValidation(
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
