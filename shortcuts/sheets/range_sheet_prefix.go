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

// splitRangeSheetPrefix splits "Sheet1!A1:D20" into ("Sheet1", "A1:D20"),
// following the front-end ref lexer so a reference copied out of a formula or
// a sheet UI parses here the way it does there:
//
//   - The separator has four equal spellings: "!", the full-width "！"
//     (TractorLexer.ts, ExclamationMark = `[ \t\r\n]*(?:!|！)[ \t\r\n]*`), and
//     the backslash-escaped forms of both, which survive shell history
//     expansion. Legacy v2 normalizes the same set (backward/helpers.go,
//     sheetRangeSeparatorReplacer).
//   - Quoted names ('My Sheet'!A1) are unwrapped, doubled-quote escape
//     collapsed. The quotes are what delimit the name, so one may contain a
//     "!"; escapeSheetName quotes everything that is not pure a-z, so any name
//     with a space, a digit or CJK arrives in this form.
//   - Unquoted names split on the first separator — the lexer's Identifier
//     production excludes "!" at both widths, so a name cannot contain one.
//
// One deliberate divergence: the lexer also bars whitespace from an unquoted
// name so that `SUM(My Sheet!A1)` tokenizes; a flag has no such ambiguity, so
// `My Sheet!A1` is accepted rather than rejected over missing quotes.
//
// ok is false with no separator, an empty side ("!A1", "Sheet1!"), or an
// unclosed quote — malformed rather than prefixed, and the flag's own
// validation names those better than a half-applied rewrite would.
func splitRangeSheetPrefix(rng string) (sheet, rest string, ok bool) {
	rng = strings.TrimSpace(rng)
	sheet, end, ok := scanSheetQualifier(rng)
	if !ok {
		return "", "", false
	}
	rest = strings.TrimSpace(rng[end:])
	if sheet == "" || rest == "" {
		return "", "", false
	}
	return sheet, rest, true
}

// scanSheetQualifier is the grammar itself. It returns the sheet the qualifier
// names (unquoted, escapes collapsed) and the byte offset just past its
// separator, so rng[:end] is the verbatim qualifier and rng[end:] is the A1
// part. rng is expected pre-trimmed.
//
// The offset is what callers re-rendering a range need: parseCellRange's output
// is both shipped to the server and printed for the caller to paste back, so
// the qualifier has to survive EXACTLY as written — full-width separator,
// quotes and all — which a parsed-and-reassembled name cannot promise.
//
// ok is false with no qualifier at all or an unclosed quote. An empty side is
// the caller's to judge: "!A1" is malformed to the selector rewrite but has
// always been tolerated by the dimension parser.
func scanSheetQualifier(rng string) (sheet string, end int, ok bool) {
	if strings.HasPrefix(rng, "'") {
		return scanQuotedSheetQualifier(rng)
	}
	// The lexer's unquoted name production (Identifier) excludes both widths
	// of the separator, so the first one is necessarily the boundary.
	for i, r := range rng {
		if r != '!' && r != '！' {
			continue
		}
		nameEnd := i
		// A backslash immediately before is part of the separator's spelling,
		// not of the name: `Sheet1\!A1` survives shell history expansion.
		if i > 0 && rng[i-1] == '\\' {
			nameEnd = i - 1
		}
		return strings.TrimSpace(rng[:nameEnd]), i + len(string(r)), true
	}
	return "", 0, false
}

// scanQuotedSheetQualifier handles the 'Sheet name'!A1 form. Scanning byte by
// byte is safe for multi-byte names: only the ASCII quote is compared, and
// every other byte is copied through verbatim. Since the quotes are what
// delimit the name, one may contain a separator.
func scanQuotedSheetQualifier(rng string) (sheet string, end int, ok bool) {
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
		sepStart := i + 1
		tail := rng[sepStart:]
		ws := len(tail) - len(strings.TrimLeft(tail, " \t\r\n"))
		tail = tail[ws:]
		sepLen := ws
		if strings.HasPrefix(tail, `\`) {
			tail, sepLen = tail[1:], sepLen+1
		}
		switch {
		case strings.HasPrefix(tail, "!"):
			sepLen++
		case strings.HasPrefix(tail, "！"):
			sepLen += len("！")
		default:
			return "", 0, false
		}
		return strings.TrimSpace(name.String()), sepStart + sepLen, true
	}
	return "", 0, false
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
