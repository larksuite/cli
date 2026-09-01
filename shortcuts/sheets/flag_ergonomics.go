// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/flagalias"
	"github.com/larksuite/cli/internal/suggest"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ─── sheets flag ergonomics ─────────────────────────────────────────────
//
// Eval traces show two recovery loops that burn agent round-trips on the
// sheets domain specifically: hallucinated flag names (--cols for --range,
// --file for --csv) whose unknown-flag error only points at --help, and
// enum values imported from CSS / Excel vocabulary ("center" for the
// vertical alignment Lark spells "middle"). Both fixes are wired through
// the existing PostMount hook and composed onto any prior PostMount in
// Shortcuts(); exact flag-name aliases use common.Flag.Aliases separately.

// withFlagErgonomics wraps an optional PostMount so that, after it runs,
// the command gets the sheets-specific unknown-flag error (valid flags
// inlined), enum-value normalization (canonical vocabulary auto-applied,
// typos suggested), and the --range sheet-prefix rewrite.
func withFlagErgonomics(prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}
		cmd.SetFlagErrorFunc(sheetsFlagErrorFunc)
		chainEnumNormalization(cmd)
		chainFlagAliases(cmd)
		chainRangeSheetPrefix(cmd)
	}
}

// ─── intuitive flag names: silent aliases & prescriptions ───────────────
//
// Eval traces show unknown-flag failures cluster on a handful of habitual
// names (--file, --cols, --dimension, --start-cell, --bold, --source…) that
// agents import from generic CLI / Excel vocabulary. Two tiers, mirroring
// the enum-normalization contract above: a name whose value semantics are
// identical to the real flag is rewritten silently (zero round-trips); a
// name whose fix changes the value or moves it into a JSON field gets a
// curated prescription on the unknown-flag error instead — never a silent
// rewrite.

// commandFlagAliases maps, per command, habitual flag names onto the flag
// actually registered. Only pairs with identical value semantics belong
// here: the rewrite is invisible, so it must be safe to apply unread
// (+csv-put --file with a path value still trips the file-path guard, which
// prescribes @file / stdin).
var commandFlagAliases = map[string]map[string]string{
	"+csv-put":      {"file": "csv"},
	"+sheet-create": {"name": "title"},
	// The new name is the only name-valued input a rename takes, so the
	// habitual spellings are unambiguous (unlike +sheet-copy, where a name
	// could mean the copy's title or the source selector and gets a
	// prescription instead). 07-28 root-cause report #25: 10/10 wrote
	// --new-name, 24 occurrences.
	"+sheet-rename": {"name": "title", "new-name": "title"},
	// size → width/height: the styles protocol (--styles row_sizes/col_sizes)
	// spells the pixel dimension "size", and pre-2026-07 batches accepted it
	// here too — the rename is the single largest sub-op error cluster in
	// eval traces (15+ hits). Same pixel-count semantics, safe to rewrite.
	"+cols-resize":         {"cols": "range", "size": "width"},
	"+rows-resize":         {"rows": "range", "size": "height"},
	"+range-fill":          {"source": "source-range", "target": "target-range"},
	"+range-copy":          {"source": "source-range", "target": "target-range"},
	"+range-move":          {"source": "source-range", "target": "target-range"},
	"+chart-create-basic":  {"type": "chart-type", "range": "data-range", "x-axis": "x-axis-title", "y-axis": "y-axis-title"},
	"+chart-config-update": {"x-axis": "x-axis-title", "y-axis": "y-axis-title"},
	"+chart-data-update":   {"range": "data-range"},
	// values → cells: gspread spells the write payload values, and this CLI's
	// own +workbook-create uses --values for untyped 2D data. It qualifies for
	// the silent tier only because normalizeCellsFlagValue lifts bare scalars
	// into {"value":…}, so a --values matrix ('[["工作内容"]]') is accepted
	// verbatim as --cells — the name was the only thing wrong.
	"+cells-set": {"values": "cells"},
}

// intuitiveFlagHints carries the prescription for habitual names whose fix
// is not a 1:1 rename — the value belongs to a different flag or to a field
// inside a JSON payload. The hint spells the exact correct form so the
// retry needs no --help round trip.
var intuitiveFlagHints = map[string]map[string]string{
	"+sheet-copy": {
		"new-sheet-name":    "the copy's name goes in --title; --sheet-name / --sheet-id selects the source sheet",
		"target-sheet-name": "the copy's name goes in --title; --sheet-name / --sheet-id selects the source sheet",
		"new-name":          "the copy's name goes in --title; --sheet-name / --sheet-id selects the source sheet",
	},
	"+dim-insert": {
		"dimension": "+dim-insert infers rows vs columns from --position: a row number like 3 inserts rows, a column letter like C inserts columns; pair with --count N",
	},
	// Must prescribe --rows / --cols, never the retired --dimension/--count
	// pair (DEPRECATED(phase-2) on dimFreezeLegacyNote): those flags are hidden
	// from --help, so they do not even appear in the "valid flags" list printed
	// beside this hint, and using them earns a second note steering back here.
	"+dim-freeze": {
		"frozen-rows":         "freeze the first N rows with --rows N (add --cols M to hold columns too — one call states the whole freeze state)",
		"frozen-cols":         "freeze the first N columns with --cols N (add --rows M to hold rows too — one call states the whole freeze state)",
		"frozen-columns":      "freeze the first N columns with --cols N (add --rows M to hold rows too — one call states the whole freeze state)",
		"frozen-row-count":    "freeze the first N rows with --rows N (add --cols M to hold columns too — one call states the whole freeze state)",
		"frozen-col-count":    "freeze the first N columns with --cols N (add --rows M to hold rows too — one call states the whole freeze state)",
		"frozen-column-count": "freeze the first N columns with --cols N (add --rows M to hold rows too — one call states the whole freeze state)",
	},
	"+cells-set-style": {
		"bold":      "use --font-weight bold",
		"italic":    "use --font-style italic",
		"underline": "use --font-line underline",
		"font-bold": "use --font-weight bold",
		"bg-color":  "use --background-color",
		// Google Sheets API vocabulary (wrapStrategy).
		"wrap-strategy": "use --word-wrap (overflow / auto-wrap / word-clip)",
		// The border family: the only border flag is --border-styles (composite
		// JSON); color and per-side variants ride inside it.
		"border-style":  `borders take one composite flag: --border-styles '{"all":{"style":"solid","weight":"thin","color":"#000000"}}' (sides: top/bottom/left/right, or "all" for all four)`,
		"border-color":  `border color rides inside --border-styles JSON, e.g. --border-styles '{"all":{"style":"solid","weight":"thin","color":"#000000"}}'`,
		"border-all":    `use --border-styles '{"all":{"style":"solid","weight":"thin","color":"#000000"}}' — the "all" key applies one spec to all four sides`,
		"border-top":    `per-side borders ride inside --border-styles JSON, e.g. --border-styles '{"top":{"style":"solid","weight":"thin","color":"#000000"}}'`,
		"border-bottom": `per-side borders ride inside --border-styles JSON, e.g. --border-styles '{"bottom":{"style":"solid","weight":"thin","color":"#000000"}}'`,
		"border-left":   `per-side borders ride inside --border-styles JSON, e.g. --border-styles '{"left":{"style":"solid","weight":"thin","color":"#000000"}}'`,
		"border-right":  `per-side borders ride inside --border-styles JSON, e.g. --border-styles '{"right":{"style":"solid","weight":"thin","color":"#000000"}}'`,
	},
	"+cells-set": {
		// Predictable prior from +table-put --styles: models will try to
		// attach range-level styling to a --writes call the same way.
		"styles": `range-level styling goes through +styles-put (same {"styles":[...]} vocabulary); per-cell styles ride inside the cells objects as cell_styles`,
	},
	"+table-put": {
		"start-cell": `anchor each sub-sheet via the "start_cell" field inside --sheets (e.g. {"sheets":[{"name":"Sheet1","start_cell":"B2",…}]}); to paste CSV at a cell use +csv-put --start-cell`,
		"sheet-name": `+table-put has no sheet selector — each --sheets item carries its own "name" field ({"sheets":[{"name":"Sheet1",…}]})`,
		"sheet-id":   `+table-put has no sheet selector — each --sheets item carries its own "name" field ({"sheets":[{"name":"Sheet1",…}]})`,
	},
	"+chart-create-basic": {
		"position":    "use --anchor-cell F2 for the chart anchor; optionally pair --width and --height for its pixel size",
		"show-labels": "use --data-labels value (or any value/category/percentage combination such as value_category_percentage; use series for series names or none to hide labels)",
	},
	"+chart-config-update": {
		"show-labels": "use --data-labels value (or any value/category/percentage combination such as value_category_percentage; use series for series names or none to hide labels)",
	},
}

// chainFlagAliases installs an invisible flag-name rewrite (via
// flagalias.InstallNormalizer — this package must not call SetNormalizeFunc
// directly; see source-contract lint flag_alias_normalizer_owner) composing two
// silent corrections: the wire-vocabulary underscore form of any flag
// (--sheet_name, --border_styles — no sheets flag has an underscore in its
// canonical name), and the command's intuitive-alias table. Either way a
// habitual name parses as the real flag with zero round trips. Both are
// deliberately silent — hidden from --help and absent from the exported
// manifest, unlike the declarative Flag.Aliases the framework binds separately
// (e.g. --token → --spreadsheet-token). Aliases never shadow a registered flag;
// an alias whose target vanished (spec-side rename) is dropped, degrading to the
// unknown-flag prescription.
func chainFlagAliases(cmd *cobra.Command) {
	aliases := commandFlagAliases[cmd.Name()]
	usable := make(map[string]string, len(aliases))
	for alias, target := range aliases {
		if cmd.Flags().Lookup(alias) == nil && cmd.Flags().Lookup(target) != nil {
			usable[alias] = target
		}
	}
	flagalias.InstallNormalizer(cmd, func(name string) string {
		if strings.Contains(name, "_") {
			name = strings.ReplaceAll(name, "_", "-")
		}
		if target, ok := usable[name]; ok {
			name = target
		}
		return name
	})
}

// sheetsFlagErrorFunc overrides the root FlagErrorFunc for sheets commands.
// It keeps the root behavior (typed error, did-you-mean suggestions, the
// offending flag on params) and additionally inlines the full valid-flag
// set: hallucinated sheets flags are usually semantic guesses (--cols for
// --range) that edit distance can't rank, and a --help round trip costs an
// agent a full extra call. One line here lets it re-issue the command
// immediately.
func sheetsFlagErrorFunc(c *cobra.Command, ferr error) error {
	name, isUnknown := unknownFlagFromParseError(ferr)
	// Targeted fix for a high-frequency agent mistake: +batch-update carries no
	// top-level sheet locator (each sub-op names its own sheet inside its input),
	// yet agents reach for --sheet-id / --sheet-name at the top level. An
	// edit-distance suggestion would only mislead here, so skip it and name the
	// real contract instead. Underscore spellings (--sheet_id) are matched too:
	// the error message itself teaches the underscore key names, and sub-op
	// inputs accept them, so agents mix the two styles.
	locatorName := strings.ReplaceAll(name, "_", "-")
	if isUnknown && c.Name() == "+batch-update" && (locatorName == "sheet-id" || locatorName == "sheet-name") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"batch-update has no top-level sheet locator; put sheet_id/sheet_name inside each operation's input").
			WithParams(errs.InvalidParam{Name: "--" + name, Reason: "unknown flag"})
	}
	if !isUnknown {
		return common.ValidationErrorf("%s", ferr.Error()).
			WithHint("run `%s --help` for valid flags", c.CommandPath())
	}
	valid := visibleFlagNames(c)
	suggestions := suggest.Closest(name, valid, 3)
	for i := range suggestions {
		suggestions[i] = "--" + suggestions[i]
	}
	hint := fmt.Sprintf("run `%s --help` to see valid flags", c.CommandPath())
	if list := inlineFlagList(valid); list != "" {
		hint = "valid flags: " + list
		if len(suggestions) > 0 {
			hint = fmt.Sprintf("did you mean %s? valid flags: %s",
				strings.Join(suggestions, ", "), list)
		}
	}
	// A curated prescription beats both: it spells the exact correct form
	// for a habitual name whose fix is not a rename (see intuitiveFlagHints).
	// Edit-distance candidates are dropped with it — they can contradict the
	// prescription (--font-bold ranked --font-color/--font-line/--font-size
	// while the fix is --font-weight), and a machine-readable suggestion that
	// disagrees with the hint sends agents down the wrong retry.
	// The map is keyed hyphenated but the parse error reports the flag as
	// typed, so --frozen_rows must hit the same entry as --frozen-rows.
	if rx, ok := intuitiveFlagHints[c.Name()][strings.ReplaceAll(name, "_", "-")]; ok {
		hint = rx
		if list := inlineFlagList(valid); list != "" {
			hint = rx + "; valid flags: " + list
		}
		suggestions = nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unknown flag %q for %q", "--"+name, c.CommandPath()).
		WithParams(errs.InvalidParam{Name: "--" + name, Reason: "unknown flag", Suggestions: suggestions}).
		WithHint("%s", hint)
}

// unknownFlagFromParseError extracts the offending long-flag name from
// cobra's flag-parse error text ("unknown flag: --query" → "query").
// Returns ok=false for anything else (missing argument, invalid value,
// unknown shorthand) so those stay structured but generic. Mirrors the
// root-level parser in cmd; the prefix contract is cobra's English wording.
func unknownFlagFromParseError(err error) (string, bool) {
	const p = "unknown flag: --"
	msg := err.Error()
	i := strings.Index(msg, p)
	if i < 0 {
		return "", false
	}
	rest := msg[i+len(p):]
	if j := strings.IndexAny(rest, " \t"); j >= 0 {
		rest = rest[:j]
	}
	return rest, true
}

// visibleFlagNames lists the non-hidden flag names registered on c, sorted.
func visibleFlagNames(c *cobra.Command) []string {
	var names []string
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			names = append(names, f.Name)
		}
	})
	sort.Strings(names)
	return names
}

// inlineFlagListLimit caps how many flag names ride inline on an
// unknown-flag hint. Sheets shortcuts stay well under it.
const inlineFlagListLimit = 25

// inlineFlagList renders valid flag names as one comma-separated line for
// the unknown-flag hint, truncating past inlineFlagListLimit. Empty when
// there is nothing to list.
func inlineFlagList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	shown := names
	var suffix string
	if len(names) > inlineFlagListLimit {
		shown = names[:inlineFlagListLimit]
		suffix = fmt.Sprintf(", … (%d more; see --help)", len(names)-inlineFlagListLimit)
	}
	parts := make([]string, len(shown))
	for i, n := range shown {
		parts[i] = "--" + n
	}
	return strings.Join(parts, ", ") + suffix
}

// ─── enum vocabulary normalization ──────────────────────────────────────

// enumAliases maps habitual values agents import from CSS / Excel / Google
// Sheets onto the value the Lark API actually uses, keyed by the wrong
// value. Applied only when the alias target is in the enum (and the wrong
// value is not), so e.g. "center" still stands for horizontal alignment
// (where it is valid) and only maps to "middle" for vertical alignment.
var enumAliases = map[string]string{
	"center": "middle", // CSS vertical-align: center → Lark "middle"
	"centre": "center",
	"middle": "center", // CSS-style middle → Lark horizontal "center"
	// Raw Lark OpenAPI merge vocabulary (MERGE_ALL/…) — agents reproduce it
	// from the API docs; lowercased by canonicalEnumValue before lookup.
	"merge_all":     "all",
	"merge_rows":    "rows",
	"merge_columns": "columns",
	// Boolean-style word-wrap habits: true unambiguously means wrap on;
	// false means "don't wrap", whose Lark default is overflow (word-clip is
	// a distinct truncation mode nobody spells "false").
	"true":  "auto-wrap",
	"false": "overflow",
	// Google Sheets wrapStrategy vocabulary: WRAP / CLIP / OVERFLOW. Only
	// the first two need mapping — overflow is spelled the same in both.
	"wrap": "auto-wrap",
	"clip": "word-clip",
	// Combined chart data-label vocabulary emitted by models. The tool enum
	// spells the same intent as one value.
	"percentage,value": "value_percentage",
	"value,percentage": "value_percentage",
}

// DEPRECATED(phase-2): enum values this CLI used to accept and now expresses
// by omitting the flag. They are dropped from the published enum so the docs
// and --help stop teaching them, but a caller that still passes one must not
// hard-fail: the value was valid — for --inherit-style it was even the
// DEFAULT — so existing scripts and any agent carrying older docs would break
// on a spelling that never meant anything else.
//
// Semantics: a retired value is cleared, making the call identical to omitting
// the flag (pinned by TestRetiredEnumValueMatchesOmitted). It is deliberately
// silent — unlike --dimension/--count there is nothing for the caller to
// migrate to, so a note would only be noise.
//
// Phase 2 removal: drop the entry here and let the normal enum error apply.
var retiredEnumValues = map[string]map[string][]string{
	// +dim-insert --inherit-style dropped "none" when the side mapping was
	// corrected: no inheritance is what omitting the flag already means, so
	// the value was pure redundancy.
	"+dim-insert": {"inherit-style": {"none"}},
}

// clearRetiredFlag makes a retired value indistinguishable from an absent
// flag. Resetting Changed matters as much as the value: the batch path
// expresses "as if omitted" by deleting the key, so Changed() reports false
// there. Leaving cobra's Changed at true would make a flag whose logic reads
// Changed() (rather than the value) behave differently standalone than inside
// +batch-update — and TestBatchOp_BodyMatchesStandalone only catches such a
// split once it reaches the request body.
func clearRetiredFlag(cmd *cobra.Command, name string) {
	_ = cmd.Flags().Set(name, "")
	if f := cmd.Flags().Lookup(name); f != nil {
		f.Changed = false
	}
}

// isRetiredEnumValue reports whether val is a retired spelling for this
// command's flag, i.e. one that should be cleared rather than rejected.
func isRetiredEnumValue(command, flag, val string) bool {
	byFlag, ok := retiredEnumValues[command]
	if !ok {
		return false
	}
	for _, retired := range byFlag[flag] {
		if strings.EqualFold(retired, val) {
			return true
		}
	}
	return false
}

// canonicalEnumValue returns the enum entry an off-vocabulary value
// unambiguously means — exact case-insensitive match first, then the
// cross-vocabulary alias table. Unlike an edit-distance guess, the result
// is safe to apply on the caller's behalf. Returns "" when the value has
// no unambiguous canonical form in this enum.
func canonicalEnumValue(val string, enum []string) string {
	lower := strings.ToLower(val)
	for _, allowed := range enum {
		if strings.ToLower(allowed) == lower {
			return allowed
		}
	}
	aliasKey := strings.ReplaceAll(lower, " ", "")
	if target, ok := enumAliases[aliasKey]; ok {
		if slices.Contains(enum, target) {
			return target
		}
	}
	return ""
}

// closestEnumValue picks the best "did you mean" candidate for an invalid
// enum value: the unambiguous canonical form first, then edit distance.
// For prose suggestions only — an edit-distance match must never be
// auto-applied. Returns "" when nothing is close.
func closestEnumValue(val string, enum []string) string {
	if canon := canonicalEnumValue(val, enum); canon != "" {
		return canon
	}
	if match := suggest.Closest(val, enum, 1); len(match) > 0 {
		return match[0]
	}
	return ""
}

// chainEnumNormalization installs a PreRunE stage (composed onto any
// framework-set PreRunE, which runs first so OnInvoke side effects and the
// --print-schema required-flag relaxation keep their contracts) that
// normalizes the command's flat enum flags before the common runner
// validates them:
//
//   - an unambiguous vocabulary mismatch (casing, or a known alias like CSS
//     "center" for Lark's vertical "middle") IS the value the caller meant —
//     rewrite it in place and proceed instead of failing the call just to
//     have the agent retype the canonical spelling;
//   - anything else fails here with the allowed list plus a "did you mean"
//     hint for edit-distance typos — a guess is never auto-applied.
//
// No-op for commands whose flag defs declare no enums.
func chainEnumNormalization(cmd *cobra.Command) {
	defs, _ := loadFlagDefs()
	spec, ok := defs[cmd.Name()]
	if !ok {
		return
	}
	var enumFlags []flagDef
	for _, df := range spec.Flags {
		if df.Kind != "system" && len(df.Enum) > 0 && df.Type == "string" {
			enumFlags = append(enumFlags, df)
		}
	}
	if len(enumFlags) == 0 {
		return
	}
	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		// --print-schema is pure local introspection; the runner never enum-
		// validates that path, so don't start here.
		if want, err := c.Flags().GetBool("print-schema"); err == nil && want {
			return nil
		}
		for _, df := range enumFlags {
			val, err := c.Flags().GetString(df.Name)
			if err != nil || val == "" || slices.Contains(df.Enum, val) {
				continue
			}
			if canon := canonicalEnumValue(val, df.Enum); canon != "" {
				c.Flags().Set(df.Name, canon)
				continue
			}
			if isRetiredEnumValue(cmd.Name(), df.Name, val) {
				clearRetiredFlag(c, df.Name)
				continue
			}
			verr := common.ValidationErrorf("invalid value %q for --%s, allowed: %s",
				val, df.Name, strings.Join(df.Enum, ", ")).
				WithParam("--" + df.Name)
			if match := suggest.Closest(val, df.Enum, 1); len(match) > 0 {
				verr = verr.WithHint("did you mean %q?", match[0])
			}
			return verr
		}
		return nil
	}
}
