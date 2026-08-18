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
