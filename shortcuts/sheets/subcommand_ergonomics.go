// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
)

// ─── sheets subcommand ergonomics ───────────────────────────────────────
//
// Callers reach for subcommand names this CLI does not have, borrowed from
// neighbouring ecosystems. Each one names a command that already exists, so the
// fix is a prescription — never a rewrite, which would run a write the caller
// never named.
//
// The framework answers an unknown name by edit distance over the group's
// children. That ranking is prefix-weighted, so it cannot settle a name whose
// answer shares no prefix with it, or one whose same-prefix siblings crowd the
// answer out. Those are the names below.

// prescription is the command an invented name meant, plus the exact retry form
// so the next attempt needs no --help round trip. Both are required: a name that
// points at more than one command belongs with the ranker.
type prescription struct {
	Command string
	Hint    string
}

const (
	colsResizeForm = `column width is +cols-resize: --range A:E --width 120 for one uniform width, or --widths '{"A":80,"C:E":120}' for many columns in one call (values are pixels, not Excel character units)`
	rowsResizeForm = `row height is +rows-resize: --range 2:10 --height 40 for one uniform height, or --heights '{"1":50,"2:20":30}' for many rows in one call (values are pixels, not points)`
)

// unknownSubcommandHints maps a name callers reach for onto the command it
// meant. A rare spelling stays with the ranker rather than growing this table.
var unknownSubcommandHints = map[string]prescription{
	"+sheet-add": {
		Command: "+sheet-create",
		Hint:    `create a sub-sheet with +sheet-create --title "…" — this domain spells the verb create, never add; --url / --spreadsheet-token carry over unchanged`,
	},
	"+col-resize": {Command: "+cols-resize", Hint: colsResizeForm},
	"+row-resize": {Command: "+rows-resize", Hint: rowsResizeForm},
	"+cells-put": {
		Command: "+cells-set",
		Hint:    `write cell values with +cells-set --range A1:B2 --cells '[["a","b"],["c","d"]]' — one inner array per row, sized to the range; a cell slot takes a bare scalar or an object like {"formula":"=A1*2"}`,
	},
	"+freeze-rows": {
		Command: "+dim-freeze",
		Hint:    `freeze the first N rows with +dim-freeze --rows N (add --cols M to hold columns too — one call states the whole freeze state)`,
	},
	// 08-29..31 reflow, long-tail table. Reading styles back and listing
	// sub-sheets are both spelled as an option on a command that already
	// exists, so the invented name has no sibling the ranker could reach.
	"+cells-get-style": {
		Command: "+cells-get",
		Hint:    `read styles with +cells-get --include style (add value / formula / comment to the same list to get them in one call)`,
	},
	"+dropdown-list": {
		Command: "+dropdown-get",
		Hint:    `read the dropdown on a range with +dropdown-get --range A1 — there is no list form; a range covering several cells reports each one`,
	},
	"+sheets-list": {
		Command: "+workbook-info",
		Hint:    `+workbook-info lists every sub-sheet with its id, title, and grid size — that is the sheet list`,
	},
	"+list-sheets": {
		Command: "+workbook-info",
		Hint:    `+workbook-info lists every sub-sheet with its id, title, and grid size — that is the sheet list`,
	},
	"+meta": {
		Command: "+workbook-info",
		Hint:    `workbook metadata (title, sub-sheets, grid sizes, merged regions) comes from +workbook-info`,
	},
	// 09-11..15 catalog, unknown-subcommand rows. Each of these outranks every
	// entry above it in frequency and none is reachable by the distance ranker:
	// the answer either shares no prefix with what was typed (+merge-cells →
	// +cells-merge is a word SWAP, which Levenshtein scores as far apart) or is
	// crowded out by same-prefix siblings (+sheet-update sits among eleven
	// +sheet-* commands).
	"+sheet-update": {
		Command: "+sheet-rename",
		Hint:    `there is no single "update a sheet" command — rename with +sheet-rename --title "…", resize with +cols-resize / +rows-resize, freeze with +dim-freeze, hide with +sheet-hide`,
	},
	"+dropdown-create": {
		Command: "+dropdown-set",
		Hint:    `create a dropdown with +dropdown-set --range A1:A10 --options '["Yes","No"]' — the same command also replaces an existing one`,
	},
	"+workbook-rename": {
		Command: "+sheet-rename",
		Hint:    `+sheet-rename renames a SUB-SHEET; the workbook's own title is a Drive property — rename it with ` + "`lark-cli drive +file-rename`",
	},
	"+merge-cells": {
		Command: "+cells-merge",
		Hint:    `merge with +cells-merge --range A1:D1 — this domain puts the object first and the verb second (+cells-merge, +cells-clear, +cells-set)`,
	},
	"+unmerge-cells": {
		Command: "+cells-unmerge",
		Hint:    `unmerge with +cells-unmerge --range A1:D1 — this domain puts the object first and the verb second`,
	},
	"+create": {
		Command: "+workbook-create",
		Hint:    `create a spreadsheet with +workbook-create --title "…"; create a sub-sheet inside an existing one with +sheet-create`,
	},
	"+workbook-get": {
		Command: "+workbook-info",
		Hint:    `+workbook-info reads the workbook: its sub-sheets with ids, titles, and grid sizes. Cell data comes from +csv-get / +cells-get`,
	},
	"+workbook-update": {
		Command: "+cells-set",
		Hint:    `there is no single "update a workbook" command — write cells with +cells-set, write a whole typed table with +table-put, restyle with +styles-put`,
	},
	"+read": {
		Command: "+csv-get",
		Hint:    `read a range as CSV with +csv-get --range A1:F50 (omit --range for the whole sheet); +cells-get returns per-cell values, formulas and styles instead`,
	},
	"+range-read": {
		Command: "+csv-get",
		Hint:    `read a range as CSV with +csv-get --range A1:F50; +cells-get returns per-cell values, formulas and styles instead`,
	},
	"+range-clear": {
		Command: "+cells-clear",
		Hint:    `clear a range with +cells-clear --range A1:F50 --yes; several ranges in one call go through +cells-batch-clear`,
	},
	"+sheet-freeze": {
		Command: "+dim-freeze",
		Hint:    `freeze with +dim-freeze --rows N (add --cols M to hold columns too — one call states the whole freeze state)`,
	},
	"+cells-write": {
		Command: "+cells-set",
		Hint:    `write cell values with +cells-set --range A1:B2 --cells '[["a","b"],["c","d"]]'`,
	},
	"+cells-update": {
		Command: "+cells-set",
		Hint:    `write cell values with +cells-set --range A1:B2 --cells '[["a","b"],["c","d"]]'`,
	},
	"+values-update": {
		Command: "+cells-set",
		Hint:    `write cell values with +cells-set --range A1:B2 --cells '[["a","b"],["c","d"]]'`,
	},
	"+cells-copy": {
		Command: "+range-copy",
		Hint:    `copy a range with +range-copy --source-range A1:D10 --target-range F1 (both may name a sheet as an A1 prefix)`,
	},
	"+cols-hide": {
		Command: "+dim-hide",
		Hint:    `hide with +dim-hide --range C:E for columns, --range 3:5 for rows; +dim-unhide reverses it`,
	},
	"+rows-hide": {
		Command: "+dim-hide",
		Hint:    `hide with +dim-hide --range 3:5 for rows, --range C:E for columns; +dim-unhide reverses it`,
	},
	"+data-validation-create": {
		Command: "+dropdown-set",
		Hint:    `the only validation this surface carries is the dropdown: +dropdown-set --range A1:A10 --options '["Yes","No"]'`,
	},
	"+dimension-set": {
		Command: "+cols-resize",
		Hint:    colsResizeForm,
	},
	// 09-30..10-06 catalog: 2,300 calls to a bare read named +fetch. It is a
	// docs-domain habit (docs +fetch reads a whole document); the sheets
	// surface has no one "read everything" call — workbook metadata is
	// +workbook-info, and sheet data needs a named target sheet. Kept as a
	// prescription: the callers' --doc / --detail / --token flags are not
	// cells-get flags, and cells-get requires a non-omitted --range, so a
	// silent redirect would fail again.
	"+fetch": {
		Command: "+cells-get",
		Hint:    `read sheet data with +cells-get --sheet-name "<name>" --range A1:F50 (--range is required), or +csv-get --sheet-name "<name>" to read the WHOLE used sheet with --range omitted; workbook structure (sub-sheets/ids) comes from +workbook-info`,
	},
}

// InstallUnknownSubcommandHints hooks the sheets group's Args validator, which
// cobra runs before the group's RunE. That ordering is what keeps this inside
// sheets: the framework's unknown-subcommand guard installs on RunE and never
// touches Args, so the two compose and every name this table does not claim
// still reaches the framework's ranked "did you mean one of: …".
func InstallUnknownSubcommandHints(svc *cobra.Command) {
	if svc == nil {
		return
	}
	inherited := svc.Args
	svc.Args = func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			if rx, ok := unknownSubcommandHints[canonicalSubcommand(args[0])]; ok && prescriptionIsReachable(c, rx) {
				return prescriptionError(c, args[0], rx)
			}
		}
		if inherited != nil {
			return inherited(c, args)
		}
		return cobra.ArbitraryArgs(c, args)
	}
}

// prescriptionIsReachable resolves the target against the live tree, not the
// table. Every target is a write command, so a concealed distribution or a user
// policy of max_risk: read replaces it with a hidden deny stub; prescribing it
// then would name a command that can only answer command_unavailable. The check
// mirrors the framework ranker's filter, so a target the ranker would refuse to
// suggest is one this table refuses to prescribe. A target that vanished in a
// rename fails it too, backstopping TestPrescribedTargetsExist at runtime.
func prescriptionIsReachable(group *cobra.Command, rx prescription) bool {
	for _, c := range group.Commands() {
		if c.Name() == rx.Command {
			return !c.Hidden && c.IsAvailableCommand()
		}
	}
	return false
}

// prescriptionError keeps the framework guard's message verbatim — the name
// genuinely does not exist, so callers keyed on that wording must not have to
// change — and replaces only the hint and the machine-readable suggestion.
func prescriptionError(group *cobra.Command, typed string, rx prescription) error {
	group.SilenceUsage = true
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unknown subcommand %q for %q", typed, group.CommandPath()).
		WithParams(errs.InvalidParam{
			Name:        typed,
			Reason:      "unknown subcommand",
			Suggestions: []string{rx.Command},
		}).
		WithHint("%s", rx.Hint)
}

// canonicalSubcommand folds case and the wire-vocabulary underscore form
// (+sheet_add) onto the hyphenated key. No sheets shortcut has an underscore in
// its canonical name, so the fold cannot collide with a real command.
func canonicalSubcommand(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", "-")
}
