// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// ─── +dim-delete: the --position / --count pair ───────────────────────
//
// +dim-insert takes --position (where) and --count (how many). +dim-delete
// names the same two facts as one A1 span in --range. commandFlagAliases
// already renames --position → --range, which covers "delete row 3"; it does
// not cover the pair, and the pair is what callers actually send:
//
//	lark-cli sheets +dim-delete --url … --sheet-name "批量计算" --position V --count 1
//
// 24,896 of the 25,031 --position rejections in the 09-11..15 catalog carry
// --count alongside, so the rename lands the caller on `unknown flag
// "--count"` — a second failure for the same call. Accepting --position alone
// while refusing --position --count is the worst of the two options: it reads
// as if the pair were nearly right.
//
// So --count is accepted here and folded into the span. The arithmetic is the
// sibling command's own definition — N units starting at the position — and it
// is total: "V" + 1 is V:V, "3" + 2 is 3:4. Nothing is guessed, because
// +dim-delete has no other reading of "how many" to confuse it with.
//
// The flag is hidden: --help still teaches --range, which states both facts in
// one value and is what a caller reading the surface should write.

// chainDimDeleteCount registers the hidden --count on +dim-delete and folds it
// into --range before the command runs.
func chainDimDeleteCount(cmd *cobra.Command) {
	if cmd.Name() != "+dim-delete" || cmd.Flags().Lookup("count") != nil {
		return
	}
	cmd.Flags().Int("count", 0, "number of rows/columns to delete, counting from --range's start; --range \"3:5\" states the same thing")
	_ = cmd.Flags().MarkHidden("count")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if !c.Flags().Changed("count") {
			return nil
		}
		n, err := c.Flags().GetInt("count")
		if err != nil {
			// Unreachable while --count is registered as an Int just above.
			// Reported rather than dropped, so a future retype surfaces as a
			// parse failure instead of a silently ignored flag.
			return common.ValidationErrorf("--count must be a whole number").
				WithParam("--count").
				WithCause(err)
		}
		if n < 1 {
			return common.ValidationErrorf("--count must be at least 1, got %d", n).WithParam("--count")
		}
		fl := c.Flags().Lookup("range")
		if fl == nil {
			return nil
		}
		rawSpan := strings.TrimSpace(fl.Value.String())
		// A sheet prefix is moved into --sheet-name by chainRangeSheetPrefix,
		// which runs later in the PreRunE chain — so at this point
		// "Sheet1!V" is still one string and expandDimSpan would call it
		// neither a row nor a column. Split it here with the domain's own
		// qualifier scanner and put it back after, leaving the prefix for
		// that stage to handle as it always has. Not LastIndex("!"): the
		// scanner also reads the full-width "！" and the escaped forms that
		// splitRangeSheetPrefix already accepts, and an ASCII-only split
		// broke those the moment --count was added.
		prefix, start := splitDimSpanSheetPrefix(rawSpan)
		if start == "" {
			return common.ValidationErrorf("--count needs a starting row or column: pass --range (\"3\" or \"C\"), or state the whole span at once (--range \"3:5\")").
				WithParam("--count")
		}
		span, err := expandDimSpan(start, n)
		if err != nil {
			return err
		}
		return fl.Value.Set(prefix + span)
	}
}

// expandDimSpan turns a starting row/column and a unit count into the A1 span
// covering them. A value that already states a span is returned unchanged when
// it is the same width, and refused when the two disagree — silently widening
// or narrowing an explicit range would delete rows the caller did not name.
func expandDimSpan(start string, n int) (string, error) {
	if lo, hi, ok := strings.Cut(start, ":"); ok {
		width, err := dimSpanWidth(lo, hi)
		if err != nil {
			return "", err
		}
		if width != n {
			return "", common.ValidationErrorf("--range %q spans %d, but --count says %d; state the span once (drop --count, or pass --range as the single starting row/column)", start, width, n).
				WithParam("--range")
		}
		return start, nil
	}
	if idx, err := strconv.Atoi(start); err == nil {
		if idx < 1 {
			return "", common.ValidationErrorf("--range %q must be a row number of at least 1", start).WithParam("--range")
		}
		return fmt.Sprintf("%d:%d", idx, idx+n-1), nil
	}
	col := strings.ToUpper(start)
	idx := letterToColumnIndex(col)
	if idx < 0 {
		return "", common.ValidationErrorf("--range %q is neither a row number (\"3\") nor a column letter (\"C\")", start).WithParam("--range")
	}
	return col + ":" + columnIndexToLetter(idx+n-1), nil
}

// dimSpanWidth counts the rows or columns an explicit span covers.
func dimSpanWidth(lo, hi string) (int, error) {
	lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
	if a, err := strconv.Atoi(lo); err == nil {
		b, err := strconv.Atoi(hi)
		if err != nil {
			return 0, common.ValidationErrorf("--range %q:%q mixes a row number with something else", lo, hi).WithParam("--range")
		}
		if b < a {
			a, b = b, a
		}
		return b - a + 1, nil
	}
	a, b := letterToColumnIndex(lo), letterToColumnIndex(hi)
	if a < 0 || b < 0 {
		return 0, common.ValidationErrorf("--range %q:%q is not a row or column span", lo, hi).WithParam("--range")
	}
	if b < a {
		a, b = b, a
	}
	return b - a + 1, nil
}

// splitDimSpanSheetPrefix separates a sheet qualifier from the row/column it
// qualifies, using the same scanner the --range rewrite does so every
// separator spelling that works without --count keeps working with it.
func splitDimSpanSheetPrefix(v string) (prefix, rest string) {
	trimmed := strings.TrimSpace(v)
	if _, end, ok := scanSheetQualifier(trimmed); ok {
		return trimmed[:end], strings.TrimSpace(trimmed[end:])
	}
	return "", trimmed
}
