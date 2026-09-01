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
	"github.com/larksuite/cli/internal/beforeyouedit"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/deprecation"
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
	sheetsbackward "github.com/larksuite/cli/shortcuts/sheets/backward"
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
	allShortcuts = append(allShortcuts, wrapSheetsBeforeYouEdit(sheets.Shortcuts())...)
	// Backward-compatible sheets shortcuts (pre-refactor command names),
	// kept under shortcuts/sheets/backward so external callers relying on the
	// old `+create`, `+read`, `+write`, ... commands keep working alongside the
	// refactored ones. Command names are disjoint from sheets.Shortcuts().
	allShortcuts = append(allShortcuts, wrapSheetsBackwardDeprecation(sheetsbackward.Shortcuts())...)
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

// AllShortcutsWithExternal returns one isolated shortcut snapshot after validating external path collisions.
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

// RegisterShortcutSnapshotWithContext mounts one build-local shortcut snapshot.
func RegisterShortcutSnapshotWithContext(ctx context.Context, program *cobra.Command, f *cmdutil.Factory, registered []common.Shortcut) {
	// Factory.Config may be nil in tests that pass a zero-value factory.
	var brand core.LarkBrand
	if f != nil && f.Config != nil {
		if cfg, err := f.Config(); err == nil && cfg != nil {
			brand = cfg.Brand
		}
	}

	// Group by service
	byService := make(map[string][]common.Shortcut)
	for _, s := range registered {
		byService[s.Service] = append(byService[s.Service], s)
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
		for _, shortcut := range shortcuts {
			shortcut.MountWithContext(ctx, svc, f)
		}
		if service == "apps" {
			apps.InstallOnApps(svc, f)
		}
		if service == "mail" {
			mail.InstallOnMail(svc)
		}
		if service == "sheets" {
			applySheetsCompatGroups(svc)
			sheets.InstallUnknownSubcommandHints(svc)
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

// Sheets backward-compatibility grouping.
//
// shortcuts/sheets/backward keeps the pre-refactor command names alive so that
// users whose lark-sheets skill predates the refactor keep working even after
// upgrading only the binary. applySheetsCompatGroups tags each alias into a
// dedicated deprecated cobra group. The refactored commands have been the
// default for over a month, so `sheets --help` no longer lists these aliases:
// sheetsUsageTemplate renders every group except the deprecated one. The
// grouping is still applied for two reasons — the unknown-subcommand path
// (cmd/root.go) keys off it to classify a mistyped legacy alias, and each
// alias's own `sheets <alias> --help` still surfaces the "(→ +new-command)"
// migration pointer appended below. The aliases stay fully executable.
const (
	sheetsCurrentGroupID = "sheets-current"
	// sheetsDeprecatedGroupID aliases the shared deprecated-group id so both
	// `sheets --help` grouping and the generic unknown-subcommand path
	// (cmd/root.go) classify these aliases the same way.
	sheetsDeprecatedGroupID = cmdutil.DeprecatedGroupID
)

// sheetsAliasReplacement maps each pre-refactor sheets alias to the current
// command(s) that replace it, shown as a "(→ ...)" suffix in the alias's own
// --help and reused by wrapSheetsBackwardDeprecation for the on-execution
// _notice. Aliases absent from this map still land in the deprecated group,
// just without a pointer, so a missing entry degrades gracefully.
var sheetsAliasReplacement = map[string]string{
	// spreadsheet / sheet management
	"+create":       "+workbook-create",
	"+info":         "+workbook-info",
	"+export":       "+workbook-export",
	"+create-sheet": "+sheet-create",
	"+copy-sheet":   "+sheet-copy",
	"+delete-sheet": "+sheet-delete",
	"+update-sheet": "+sheet-rename / +sheet-move / …",
	// cell data
	"+read":    "+cells-get",
	"+write":   "+cells-set",
	"+append":  "+cells-set",
	"+find":    "+cells-search",
	"+replace": "+cells-replace",
	// cell style / merge / image
	"+set-style":       "+cells-set-style",
	"+batch-set-style": "+cells-batch-set-style",
	"+merge-cells":     "+cells-merge",
	"+unmerge-cells":   "+cells-unmerge",
	"+write-image":     "+cells-set-image",
	// row / column dimensions
	"+add-dimension":    "+dim-insert",
	"+insert-dimension": "+dim-insert",
	"+update-dimension": "+rows-resize / +dim-hide / …",
	"+move-dimension":   "+dim-move",
	"+delete-dimension": "+dim-delete",
	// filter views (conditions folded into the view flags)
	"+create-filter-view":           "+filter-view-create",
	"+update-filter-view":           "+filter-view-update",
	"+list-filter-views":            "+filter-view-list",
	"+get-filter-view":              "+filter-view-list",
	"+delete-filter-view":           "+filter-view-delete",
	"+create-filter-view-condition": "+filter-view-update",
	"+update-filter-view-condition": "+filter-view-update",
	"+list-filter-view-conditions":  "+filter-view-list",
	"+get-filter-view-condition":    "+filter-view-list",
	"+delete-filter-view-condition": "+filter-view-update",
	// dropdowns
	"+set-dropdown":    "+dropdown-set",
	"+update-dropdown": "+dropdown-update",
	"+get-dropdown":    "+dropdown-get",
	"+delete-dropdown": "+dropdown-delete",
	// float images (media-upload folded into create)
	"+media-upload":       "+float-image-create",
	"+create-float-image": "+float-image-create",
	"+update-float-image": "+float-image-update",
	"+get-float-image":    "+float-image-list",
	"+list-float-images":  "+float-image-list",
	"+delete-float-image": "+float-image-delete",
}

// sheetsUsageTemplate is cobra v1.10.2's stock usage template with a single
// change: the group loop is guarded by {{if ne $group.ID "deprecated"}} so the
// deprecated pre-refactor aliases are omitted from `sheets --help` altogether.
// Everything else — current commands, ungrouped metaapi subcommands under
// "Additional Commands", flags — renders exactly as cobra's default. Keep in
// sync with cobra's defaultUsageTemplate on upgrade.
var sheetsUsageTemplate = fmt.Sprintf(`Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}{{if ne $group.ID %q}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`, sheetsDeprecatedGroupID)

func applySheetsCompatGroups(svc *cobra.Command) {
	svc.AddGroup(
		&cobra.Group{ID: sheetsCurrentGroupID, Title: "Available Commands:"},
		&cobra.Group{
			ID:    sheetsDeprecatedGroupID,
			Title: "Deprecated pre-refactor commands (still work) — update your lark-sheets skill, then: lark-cli update",
		},
	)

	deprecated := make(map[string]struct{})
	for _, s := range sheetsbackward.Shortcuts() {
		deprecated[s.Command] = struct{}{}
	}

	for _, c := range svc.Commands() {
		name := c.Name()
		if _, ok := deprecated[name]; ok {
			c.GroupID = sheetsDeprecatedGroupID
			if repl := sheetsAliasReplacement[name]; repl != "" {
				c.Short = c.Short + "  (→ " + repl + ")"
			}
			continue
		}
		// Only the refactored shortcuts (all "+"-prefixed) belong in the current
		// group. Leave the OpenAPI metaapi subcommands (spreadsheets, ...) and the
		// auto-added help/completion ungrouped so cobra files them under
		// "Additional Commands".
		if len(name) > 0 && name[0] == '+' {
			c.GroupID = sheetsCurrentGroupID
		}
	}

	// Refactored commands have been the default for over a month: drop the
	// deprecated group from `sheets --help` (see sheetsUsageTemplate). The
	// aliases remain grouped and executable, just no longer advertised here.
	svc.SetUsageTemplate(sheetsUsageTemplate)
}

// sheetsBeforeYouEdit maps sheet-mutating commands to the skill reference an
// agent should read before making that edit. Grouped by edit intent; commands
// not listed here carry no notice. Read the reference with:
//
//	lark-cli skills read lark-sheets references/<file>
var sheetsBeforeYouEdit = map[string]string{
	// Bringing a local file online / starting a new workbook: the imported
	// sheet is about to be edited in place — keep originals intact, prefer
	// formulas over precomputed values, and preserve existing styles.
	"+workbook-import": "before editing the imported sheet: write results to new sheets/columns and keep original rows intact; prefer formulas over precomputed static values (Lark formula syntax differs from Excel — read references/lark-sheets-formula-translation.md before writing formulas); preserve untouched styles. Style inheritance checklist: references/lark-sheets-visual-standards.md (run: lark-cli skills read lark-sheets references/lark-sheets-visual-standards.md)",
	"+workbook-create": "before filling the new workbook: prefer formulas over precomputed static values (Lark formula syntax differs from Excel — read references/lark-sheets-formula-translation.md first); visual/style standards: references/lark-sheets-visual-standards.md (run: lark-cli skills read lark-sheets references/lark-sheets-visual-standards.md)",

	// Data writes into an existing table: extending without inheriting the
	// neighboring styles (fonts / borders / banding / row heights) is the most
	// common visual regression; verify by re-reading the written range.
	"+table-put":    "when writing into an existing table, inherit adjacent styles (fonts/borders/banding/row heights) and re-read the written range to verify — checklist: references/lark-sheets-write-cells.md (run: lark-cli skills read lark-sheets references/lark-sheets-write-cells.md)",
	"+csv-put":      "when writing into an existing table, inherit adjacent styles (fonts/borders/banding/row heights) and re-read the written range to verify — checklist: references/lark-sheets-write-cells.md (run: lark-cli skills read lark-sheets references/lark-sheets-write-cells.md)",
	"+cells-set":    "when writing into an existing table, inherit adjacent styles (fonts/borders/banding/row heights) and re-read the written range to verify — checklist: references/lark-sheets-write-cells.md (run: lark-cli skills read lark-sheets references/lark-sheets-write-cells.md)",
	"+batch-update": "batch writes are fail-fast without a unified rollback: on failure, re-read the touched ranges before re-sending — semantics: references/lark-sheets-batch-update.md (run: lark-cli skills read lark-sheets references/lark-sheets-batch-update.md)",

	// Structure edits: +dim-insert inherits styles but NOT row heights — new
	// rows fall back to the default height and clip long text.
	"+dim-insert": "--inherit-style does not inherit row heights: read the neighbor row_height first and follow up with +rows-resize when inserting rows for long text — details: references/lark-sheets-sheet-structure.md (run: lark-cli skills read lark-sheets references/lark-sheets-sheet-structure.md)",

	// Object creation/update: each object kind has a dedicated reference with
	// the exact property vocabulary; guessing properties from Excel/OpenAPI
	// habits is the dominant failure mode for these commands.
	"+chart-create":       "before configuring the chart, get a minimal working --properties template via +chart-create --print-example <type>, and read references/lark-sheets-chart.md for refs/index vocabulary (run: lark-cli skills read lark-sheets references/lark-sheets-chart.md)",
	"+chart-update":       "chart property vocabulary and update semantics: references/lark-sheets-chart.md (run: lark-cli skills read lark-sheets references/lark-sheets-chart.md)",
	"+pivot-create":       "pivot field/aggregation vocabulary and placement rules: references/lark-sheets-pivot-table.md (run: lark-cli skills read lark-sheets references/lark-sheets-pivot-table.md)",
	"+pivot-update":       "pivot field/aggregation vocabulary and update semantics: references/lark-sheets-pivot-table.md (run: lark-cli skills read lark-sheets references/lark-sheets-pivot-table.md)",
	"+cond-format-create": "rule types, ranges and color conventions: references/lark-sheets-conditional-format.md (run: lark-cli skills read lark-sheets references/lark-sheets-conditional-format.md)",
	"+cond-format-update": "rule types, ranges and color conventions: references/lark-sheets-conditional-format.md (run: lark-cli skills read lark-sheets references/lark-sheets-conditional-format.md)",
	"+sparkline-create":   "sparkline group config and data refs: references/lark-sheets-sparkline.md (run: lark-cli skills read lark-sheets references/lark-sheets-sparkline.md)",
	"+sparkline-update":   "sparkline group config and data refs: references/lark-sheets-sparkline.md (run: lark-cli skills read lark-sheets references/lark-sheets-sparkline.md)",
	"+float-image-create": "float image placement/size/z-order: references/lark-sheets-float-image.md (run: lark-cli skills read lark-sheets references/lark-sheets-float-image.md)",
	"+float-image-update": "float image placement/size/z-order: references/lark-sheets-float-image.md (run: lark-cli skills read lark-sheets references/lark-sheets-float-image.md)",
	"+filter-create":      "filter condition vocabulary: references/lark-sheets-filter.md (run: lark-cli skills read lark-sheets references/lark-sheets-filter.md)",
	"+filter-update":      "filter condition vocabulary: references/lark-sheets-filter.md (run: lark-cli skills read lark-sheets references/lark-sheets-filter.md)",
	"+filter-view-create": "filter view semantics (view-scoped, does not mutate the sheet): references/lark-sheets-filter-view.md (run: lark-cli skills read lark-sheets references/lark-sheets-filter-view.md)",
	"+filter-view-update": "filter view semantics: references/lark-sheets-filter-view.md (run: lark-cli skills read lark-sheets references/lark-sheets-filter-view.md)",
	"+dropdown-set":       "dropdown options + colors live in the batch-update vocabulary: references/lark-sheets-batch-update.md (run: lark-cli skills read lark-sheets references/lark-sheets-batch-update.md)",
	"+dropdown-update":    "dropdown options + colors: references/lark-sheets-batch-update.md (run: lark-cli skills read lark-sheets references/lark-sheets-batch-update.md)",

	// Range operations and style delivery.
	"+cells-replace":         "check replacement scope and residue before/after replacing: references/lark-sheets-search-replace.md (run: lark-cli skills read lark-sheets references/lark-sheets-search-replace.md)",
	"+range-sort":            "sort scope pitfalls (header rows, partial-range tearing): references/lark-sheets-range-operations.md (run: lark-cli skills read lark-sheets references/lark-sheets-range-operations.md)",
	"+range-copy":            "range operation semantics: references/lark-sheets-range-operations.md (run: lark-cli skills read lark-sheets references/lark-sheets-range-operations.md)",
	"+range-fill":            "fill semantics (series vs copy): references/lark-sheets-range-operations.md (run: lark-cli skills read lark-sheets references/lark-sheets-range-operations.md)",
	"+range-move":            "move semantics and reference impact: references/lark-sheets-range-operations.md (run: lark-cli skills read lark-sheets references/lark-sheets-range-operations.md)",
	"+styles-put":            "one-shot style spec vocabulary (cell_styles/merges/sizes/freeze): references/lark-sheets-styles-put.md (run: lark-cli skills read lark-sheets references/lark-sheets-styles-put.md)",
	"+cells-set-style":       "style attribute vocabulary: references/lark-sheets-styles-put.md (run: lark-cli skills read lark-sheets references/lark-sheets-styles-put.md)",
	"+cells-batch-set-style": "style attribute vocabulary: references/lark-sheets-styles-put.md (run: lark-cli skills read lark-sheets references/lark-sheets-styles-put.md)",
	"+sheet-copy":            "template-sheet copy inherits formulas/merges/banding/widths — appending to an existing workbook: references/lark-sheets-workbook.md (run: lark-cli skills read lark-sheets references/lark-sheets-workbook.md)",
	"+sheet-create":          "when to copy a template sheet instead of creating blank: references/lark-sheets-workbook.md (run: lark-cli skills read lark-sheets references/lark-sheets-workbook.md)",
	"+dim-delete":            "structure edit semantics and reference impact: references/lark-sheets-sheet-structure.md (run: lark-cli skills read lark-sheets references/lark-sheets-sheet-structure.md)",

	// Structure reads are the natural pre-edit checkpoint: point the agent at
	// the per-scenario routing table before it starts mutating.
	"+workbook-info": "pick the per-scenario reference before you act: the scenario-to-command table in SKILL.md lists which references/lark-sheets-*.md to read for each edit (run: lark-cli skills read lark-sheets SKILL.md)",
	"+sheet-info":    "pick the per-scenario reference before you act: the scenario-to-command table in SKILL.md lists which references/lark-sheets-*.md to read for each edit (run: lark-cli skills read lark-sheets SKILL.md)",
}

// wrapSheetsBeforeYouEdit decorates sheet commands so that invoking one records
// a process-level "read this before you edit" pointer, surfaced in the JSON
// "_notice.before_you_edit" envelope block. Same decoration pattern as
// wrapSheetsBackwardDeprecation below: Validate/Execute/OnInvoke all store the
// same pointer so the notice survives validation failures and --dry-run.
func wrapSheetsBeforeYouEdit(list []common.Shortcut) []common.Shortcut {
	for i := range list {
		message, ok := sheetsBeforeYouEdit[list[i].Command]
		if !ok {
			continue
		}
		notice := &beforeyouedit.Notice{Command: list[i].Command, Message: message}
		if origValidate := list[i].Validate; origValidate != nil {
			list[i].Validate = func(ctx context.Context, runtime *common.RuntimeContext) error {
				beforeyouedit.SetPending(notice)
				return origValidate(ctx, runtime)
			}
		}
		if origExecute := list[i].Execute; origExecute != nil {
			list[i].Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
				beforeyouedit.SetPending(notice)
				return origExecute(ctx, runtime)
			}
		}
		if origOnInvoke := list[i].OnInvoke; origOnInvoke != nil {
			list[i].OnInvoke = func() {
				beforeyouedit.SetPending(notice)
				origOnInvoke()
			}
		} else {
			list[i].OnInvoke = func() { beforeyouedit.SetPending(notice) }
		}
	}
	return list
}

// wrapSheetsBackwardDeprecation decorates each backward-compatibility sheets
// alias so that invoking it records a process-level deprecation notice, which
// cmd/root.go surfaces in the JSON "_notice" envelope. This reaches the users
// the --help grouping cannot: those whose pre-refactor skill calls +read /
// +write directly and never reads --help. Replacement targets come from
// sheetsAliasReplacement — the same single source of truth that drives the
// "(→ +new)" help pointers.
func wrapSheetsBackwardDeprecation(list []common.Shortcut) []common.Shortcut {
	for i := range list {
		notice := &deprecation.Notice{
			Command:     list[i].Command,
			Replacement: sheetsAliasReplacement[list[i].Command],
			Skill:       "lark-sheets",
		}
		// Record the notice as soon as the command's own logic runs, so it is
		// surfaced even when Validate rejects the call — an out-of-date skill
		// can pass pre-refactor argument shapes (e.g. a range without the new
		// sheet-id prefix) and fail validation before Execute — and when
		// --dry-run short-circuits before Execute. Both hooks store the same
		// pointer, so setting it twice is harmless.
		if origValidate := list[i].Validate; origValidate != nil {
			list[i].Validate = func(ctx context.Context, runtime *common.RuntimeContext) error {
				deprecation.SetPending(notice)
				return origValidate(ctx, runtime)
			}
		}
		if origExecute := list[i].Execute; origExecute != nil {
			list[i].Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
				deprecation.SetPending(notice)
				return origExecute(ctx, runtime)
			}
		}
		// The Validate/Execute wrappers above miss one path: a cobra-level
		// required flag (MarkFlagRequired) that is absent fails at
		// ValidateRequiredFlags, before RunE — so neither hook runs and the
		// notice would be lost on exactly the "stale skill calls the old command
		// and mis-supplies flags" case it exists for. OnInvoke runs from PreRunE,
		// ahead of ValidateRequiredFlags, so the notice still surfaces there.
		list[i].OnInvoke = func() { deprecation.SetPending(notice) }
	}
	return list
}
