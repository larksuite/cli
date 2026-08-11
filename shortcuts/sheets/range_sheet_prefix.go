// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"

	"github.com/spf13/cobra"
)

// ─── sheet prefix inside --range ────────────────────────────────────────
//
// Eval traces: 707 calls died on "specify at least one of --sheet-id or
// --sheet-name", 53% of them with the sheet already named inside --range
// (`--range "Sheet1!A1:D20"`). The prefix is read as the selector and the bare
// A1 part reaches the tool — same tier as the silent aliases in
// flag_ergonomics.go, the intent being unambiguous.
//
// Two deliberate limits:
//
//   - Only the no-selector case is rewritten. An explicit --sheet-id /
//     --sheet-name stays authoritative and --range passes through untouched,
//     so a disagreeing prefix can never silently retarget a write.
//   - Only --range. +range-copy / +range-move / +range-fill name their
//     destination with --target-sheet-id, so a prefix on --source-range /
//     --target-range need not mean the sheet the selector picks.
//
// It lands in --sheet-name because `Sheet1!A1` is the Excel / Lark-formula
// spelling, where the prefix is a name. A prefix that is really an id hits the
// tool's "sheet not found", which lists every {id, name} pair.

const rangeSheetPrefixFlag = "range"

// rangeSheetPrefixApplies reports whether a command carries the trio the
// rewrite needs: a plain-string --range plus the --sheet-id / --sheet-name
// pair. Commands outside it are untouched — +formula-verify takes repeated
// --range values, +pivot-create's selector is the placement target
// (--target-sheet-*), and the fan-out shortcuts locate every range inside
// --ranges.
func rangeSheetPrefixApplies(command string) bool {
	defs, err := loadFlagDefs()
	if err != nil {
		return false
	}
	spec, ok := defs[command]
	if !ok {
		return false
	}
	var hasRange, hasID, hasName bool
	for _, df := range spec.Flags {
		switch df.Name {
		case rangeSheetPrefixFlag:
			hasRange = df.Type == "string"
		case "sheet-id":
			hasID = true
		case "sheet-name":
			hasName = true
		}
	}
	return hasRange && hasID && hasName
}

// sheetRangeSeparators normalizes the spellings of the "!" that divides a sheet
// name from its range. The front-end ref lexer treats the full-width ！ as an
// equal alternative (byted-sheet TractorLexer.ts, ExclamationMark =
// `[ \t\r\n]*(?:!|！)[ \t\r\n]*`), and the backslash-escaped forms survive
// shell history expansion — the legacy v2 shortcuts already normalize the same
// three (backward/helpers.go, sheetRangeSeparatorReplacer).
//
// Safe to apply to an unquoted prefix because the lexer's unquoted sheet-name
// production (Identifier) excludes every one of these characters; a quoted name
// may legally contain them, which is why splitRangeSheetPrefix runs this on the
// tail only, never across the quotes.
var sheetRangeSeparators = strings.NewReplacer(`\！`, "!", `\!`, "!", "！", "!")

// splitRangeSheetPrefix splits "Sheet1!A1:D20" into ("Sheet1", "A1:D20").
//
// The grammar mirrors the front end's ref lexer, so a reference copied out of
// a formula or a sheet UI parses here the same way it does there:
//
//   - Quoted names ('My Sheet'!A1) are unwrapped, with the doubled-quote escape
//     collapsed back to one — QuotedSingleSheetPrefix admits any run of
//     non-quote characters plus that escape, and the visitor undoes it the same
//     way. Since the quotes are what delimit the name, one may contain a "!".
//     escapeSheetName quotes every name that is not pure a-z, so anything with
//     a space, a digit or CJK arrives in this form.
//   - Unquoted names split on the first separator: the lexer's Identifier
//     production excludes "!" (both widths), so a name can never contain one.
//
// The one deliberate divergence: the lexer also excludes whitespace from an
// unquoted name, because in a formula `SUM(My Sheet!A1)` has to tokenize.
// A --range flag has no such ambiguity, so `My Sheet!A1` is accepted here
// rather than rejected over a missing pair of quotes.
//
// ok is false when there is no separator at all, when either side is empty
// ("!A1", "Sheet1!"), or when a quote is left open — those are malformed rather
// than prefixed, and the flag's own validation names the problem better than a
// half-applied rewrite would.
func splitRangeSheetPrefix(rng string) (sheet, rest string, ok bool) {
	rng = strings.TrimSpace(rng)
	if strings.HasPrefix(rng, "'") {
		return splitQuotedRangeSheetPrefix(rng)
	}
	rng = sheetRangeSeparators.Replace(rng)
	idx := strings.Index(rng, "!")
	if idx < 0 {
		return "", "", false
	}
	sheet = strings.TrimSpace(rng[:idx])
	rest = strings.TrimSpace(rng[idx+1:])
	if sheet == "" || rest == "" {
		return "", "", false
	}
	return sheet, rest, true
}

// splitQuotedRangeSheetPrefix handles the 'Sheet name'!A1 form. Scanning byte
// by byte is safe for multi-byte names: only the ASCII quote is compared, and
// every other byte is copied through verbatim.
func splitQuotedRangeSheetPrefix(rng string) (sheet, rest string, ok bool) {
	var name strings.Builder
	for i := 1; i < len(rng); i++ {
		if rng[i] != '\'' {
			name.WriteByte(rng[i])
			continue
		}
		if i+1 < len(rng) && rng[i+1] == '\'' {
			// A doubled quote is one literal quote, not the terminator.
			name.WriteByte('\'')
			i++
			continue
		}
		tail := sheetRangeSeparators.Replace(strings.TrimSpace(rng[i+1:]))
		if !strings.HasPrefix(tail, "!") {
			return "", "", false
		}
		sheet = strings.TrimSpace(name.String())
		rest = strings.TrimSpace(tail[1:])
		if sheet == "" || rest == "" {
			return "", "", false
		}
		return sheet, rest, true
	}
	return "", "", false
}

// chainRangeSheetPrefix installs a PreRunE stage (composed onto any prior
// PreRunE, which runs first) that moves a sheet prefix found in --range into
// --sheet-name and leaves the bare A1 range behind. cobra runs PreRunE before
// ValidateRequiredFlags / ValidateFlagGroups, and every later reader — the
// shortcut's Validate, DryRun and Execute — reads the flag set live, so the
// completed pair is what the whole call sees.
func chainRangeSheetPrefix(cmd *cobra.Command) {
	if !rangeSheetPrefixApplies(cmd.Name()) {
		return
	}
	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		// --print-schema is pure local introspection; leave the flags alone.
		if want, err := c.Flags().GetBool("print-schema"); err == nil && want {
			return nil
		}
		applyRangeSheetPrefixToFlags(c)
		return nil
	}
}

// applyRangeSheetPrefixToFlags performs the rewrite on a parsed flag set.
// Every failed read is a silent no-op: the goal is to complete a call that
// would otherwise fail, never to invent a second failure mode.
func applyRangeSheetPrefixToFlags(c *cobra.Command) {
	rng, err := c.Flags().GetString(rangeSheetPrefixFlag)
	if err != nil || strings.TrimSpace(rng) == "" {
		return
	}
	sheetID, err := c.Flags().GetString("sheet-id")
	if err != nil {
		return
	}
	sheetName, err := c.Flags().GetString("sheet-name")
	if err != nil {
		return
	}
	if strings.TrimSpace(sheetID) != "" || strings.TrimSpace(sheetName) != "" {
		return
	}
	sheet, rest, ok := splitRangeSheetPrefix(rng)
	if !ok {
		return
	}
	if err := c.Flags().Set("sheet-name", sheet); err != nil {
		return
	}
	_ = c.Flags().Set(rangeSheetPrefixFlag, rest)
}
