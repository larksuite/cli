// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
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
// the existing PostMount hook — composed onto any prior PostMount in
// Shortcuts(), same pattern as withTokenAlias — so the common framework
// needs no change at all and no other domain's behavior shifts.

// withFlagErgonomics wraps an optional PostMount so that, after it runs,
// the command gets the sheets-specific unknown-flag error (valid flags
// inlined) and enum-value normalization (canonical vocabulary auto-applied,
// typos suggested).
func withFlagErgonomics(prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}
		cmd.SetFlagErrorFunc(sheetsFlagErrorFunc)
		chainEnumNormalization(cmd)
	}
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
	if target, ok := enumAliases[lower]; ok {
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
