// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// ─── flags the payload already answers ────────────────────────────────
//
// Three commands carry the same shape of rejection in the 09-11..15 catalog:
// a flag the caller passed that the command does not have, whose value the
// PAYLOAD also states.
//
//	+table-put --sheet-name     6,697    +styles-put --sheet-name     925
//	+table-put --sheet-id         830    +cells-unmerge --ranges      884
//	+dim-delete --dimension     4,101    +dim-insert --dimension    1,037
//
// Each has a standing prescription explaining where the value belongs, and
// each prescription is correct — which is why they stay prescriptions when
// the flag says something the payload does not. What changes here is the case
// where the two AGREE: a --sheet-name naming the sub-sheet the payload
// already names, a --dimension restating the axis the position already
// spells, a --ranges holding the single span --range takes. Nothing has to be
// resolved there, and refusing a call whose two halves say the same thing
// spends a turn on a disagreement that does not exist.
//
// A genuine disagreement still fails, and fails LOUDLY: silently preferring
// one side would write to a sheet the caller did not name.

// chainPayloadEchoedFlags installs the reconciliations below. Registration
// happens before chainFlagAliases indexes the flag set, for the same reason
// chainDimDeleteCount does.
func chainPayloadEchoedFlags(cmd *cobra.Command) {
	switch cmd.Name() {
	case "+table-put", "+styles-put":
		registerEchoedSheetSelector(cmd)
	case "+dim-delete", "+dim-insert":
		registerDimensionIndexSpan(cmd)
		registerEchoedDimension(cmd)
	case "+cells-unmerge":
		registerSingletonRanges(cmd)
	case "+cells-set-style", "+cells-batch-set-style":
		registerStyleEnvelope(cmd)
		registerBoldFlags(cmd)
	case "+cells-set":
		registerFormulaFlags(cmd)
	case "+cells-get":
		registerIncludeAliases(cmd)
	case "+workbook-export":
		registerSheetOnlyDocType(cmd)
	}
}

// ─── OpenAPI index / A1 span spellings on the dim commands ───────────
//
// OpenAPI insertion uses zero-based, half-open indexes; deletion uses
// one-based, inclusive indexes. Canonical position/range stay A1-based.
//
//	+dim-insert --range "36:36"            → --position 36
//	+dim-insert --start-index 21 --end-index 23 → --position 22 --count 2
//	+dim-delete --start-index 6 --end-index 22  → --range 6:22
//
// Numeric start-index aliases name rows. Columns retain their A1 letters;
// an explicit conflicting --dimension is rejected after conversion rather
// than changing the axis of the write.
func registerDimensionIndexSpan(cmd *cobra.Command) {
	if cmd.Name() == "+dim-insert" && cmd.Flags().Lookup("range") == nil {
		cmd.Flags().String("range", "", "insert span; the single boundary resolves to --position")
		_ = cmd.Flags().MarkHidden("range")
	}
	if cmd.Flags().Lookup("end-index") == nil {
		usage := "one-based inclusive end of the index span; resolves to --range"
		if cmd.Name() == "+dim-insert" {
			usage = "zero-based exclusive end of the index span; resolves to --count"
		}
		cmd.Flags().String("end-index", "", usage)
		_ = cmd.Flags().MarkHidden("end-index")
	}

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		switch c.Name() {
		case "+dim-insert":
			if flagValueCameFromAlias(c, "position", "start-index") {
				pos, err := insertPositionFromIndex(c.Flags().Lookup("position").Value.String())
				if err != nil {
					return err
				}
				if err := c.Flags().Set("position", pos); err != nil {
					return err
				}
			}
			if c.Flags().Changed("range") {
				if err := reconcileInsertRange(c); err != nil {
					return err
				}
			}
			if c.Flags().Changed("end-index") {
				return deriveCountFromEndIndex(c, "position")
			}
		case "+dim-delete":
			if c.Flags().Changed("end-index") {
				return deriveRangeFromEndIndex(c)
			}
		}
		return nil
	}
}

// insertPositionFromIndex is shared by CLI alias reconciliation and batch
// input normalization. Only start-index changes base; position/index/start
// retain their existing A1 values.
func insertPositionFromIndex(raw string) (string, error) {
	index, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return "", sheetsValidationForFlag("start-index", "start-index must be an integer").WithCause(err)
	}
	if index < 0 || index == math.MaxInt {
		return "", sheetsValidationForFlag("start-index", "start-index must be a non-negative index representable as a one-based row")
	}
	return strconv.Itoa(index + 1), nil
}

// insertRangePosition normalizes a singleton dynamic insertion span onto
// position and refuses an explicit position that names another boundary.
func insertRangePosition(span, position string, positionGiven bool) (string, error) {
	dim, start, end, err := parseA1Range(span)
	if err != nil {
		return "", sheetsValidationForFlag("range", "invalid insert range %q: %v", span, err).WithCause(err)
	}
	if start != end {
		return "", sheetsValidationForFlag("range", "insert range %q must name one position; use --position and --count for multiple rows or columns", span)
	}
	resolved, _, _ := strings.Cut(span, ":")
	resolved = strings.TrimSpace(resolved)
	if !positionGiven {
		return resolved, nil
	}
	positionDim, positionIndex, err := parseA1Position(position)
	if err != nil {
		return "", sheetsValidationForFlag("position", "invalid insert position %q: %v", position, err).WithCause(err)
	}
	if positionDim != dim || positionIndex != start {
		return "", sheetsValidationForFlag("range", "--range %q disagrees with --position %q", span, position)
	}
	return position, nil
}

func reconcileInsertRange(c *cobra.Command) error {
	position, err := insertRangePosition(
		strings.TrimSpace(c.Flags().Lookup("range").Value.String()),
		strings.TrimSpace(c.Flags().Lookup("position").Value.String()),
		c.Flags().Changed("position"),
	)
	if err != nil {
		return err
	}
	if c.Flags().Changed("position") {
		return nil
	}
	return c.Flags().Set("position", position)
}

// deriveCountFromEndIndex compares the exclusive zero-based boundary with
// the parsed A1 position, checking an explicit count instead of ignoring it.
func deriveCountFromEndIndex(c *cobra.Command, startFlag string) error {
	end, err := strconv.Atoi(strings.TrimSpace(c.Flags().Lookup("end-index").Value.String()))
	if err != nil {
		return sheetsValidationForFlag("end-index", "end-index must be an integer").WithCause(err)
	}
	startRaw := strings.TrimSpace(c.Flags().Lookup(startFlag).Value.String())
	_, start, err := parseA1Position(startRaw)
	if err != nil {
		return sheetsValidationForFlag(startFlag, "invalid --%s %q: %v", startFlag, startRaw, err).WithCause(err)
	}
	var count *int
	if c.Flags().Changed("count") {
		value, err := c.Flags().GetInt("count")
		if err != nil {
			return sheetsValidationForFlag("count", "count must be an integer").WithCause(err)
		}
		count = &value
	}
	derived, err := insertCountFromEndIndex(end, start, count)
	if err != nil {
		return err
	}
	if count != nil {
		return nil
	}
	return c.Flags().Set("count", strconv.Itoa(derived))
}

// insertCountFromEndIndex derives the count for an insertion whose start and
// exclusive end both use zero-based indexes. It serves the cobra and
// map-backed batch paths so their conflict semantics stay identical.
func insertCountFromEndIndex(end, start int, givenCount *int) (int, error) {
	if end <= start {
		return 0, sheetsValidationForFlag("end-index", "end-index %d must be the exclusive boundary after start %d", end, start)
	}
	count := end - start
	if givenCount != nil && *givenCount != count {
		return 0, sheetsValidationForFlag("end-index", "end-index %d implies count %d, but --count says %d", end, count, *givenCount)
	}
	return count, nil
}

// deleteRangeFromEndIndex returns the one-based inclusive span closed by a
// dynamic end-index, preserving any sheet qualifier carried by the start.
func deleteRangeFromEndIndex(end int, span string) (string, error) {
	prefix, startRaw := splitDimSpanSheetPrefix(span)
	dim, start, rangeEnd, err := parseA1Range(startRaw)
	if err != nil {
		return "", sheetsValidationForFlag("range", "invalid delete range %q: %v", span, err).WithCause(err)
	}
	if end < 1 || end-1 < start {
		return "", sheetsValidationForFlag("end-index", "end-index %d must be the inclusive boundary at or after start %d", end, start+1)
	}
	if strings.Contains(startRaw, ":") {
		if rangeEnd != end-1 {
			return "", sheetsValidationForFlag("end-index", "end-index %d disagrees with --range %q", end, span)
		}
		return span, nil
	}
	endRaw := strconv.Itoa(end)
	if dim == "column" {
		endRaw = columnIndexToLetter(end - 1)
	}
	return prefix + startRaw + ":" + endRaw, nil
}

// deriveRangeFromEndIndex closes the deletion span at the one-based,
// inclusive end-index, before --count validates its width.
func deriveRangeFromEndIndex(c *cobra.Command) error {
	end, err := strconv.Atoi(strings.TrimSpace(c.Flags().Lookup("end-index").Value.String()))
	if err != nil {
		return sheetsValidationForFlag("end-index", "end-index must be an integer").WithCause(err)
	}
	rangeStr, err := deleteRangeFromEndIndex(end, strings.TrimSpace(c.Flags().Lookup("range").Value.String()))
	if err != nil {
		return err
	}
	return c.Flags().Set("range", rangeStr)
}

// ─── +cells-set --formula → --cells ──────────────────────────────────

func registerFormulaFlags(cmd *cobra.Command) {
	for _, name := range []string{"formula", "formulas"} {
		if cmd.Flags().Lookup(name) != nil {
			continue
		}
		cmd.Flags().String(name, "", "formula written at --range or --start-cell; resolved into --cells")
		_ = cmd.Flags().MarkHidden(name)
	}

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		formula, source, err := formulaFlagValue(c.Flags().Changed, func(name string) string {
			return c.Flags().Lookup(name).Value.String()
		})
		if err != nil {
			return err
		}
		if source == "" {
			return nil
		}
		if c.Flags().Changed("writes") {
			return sheetsValidationForFlag(source, "--%s cannot be combined with --writes", source).
				WithHint("put each formula in the cells of its --writes item")
		}
		if c.Flags().Changed("cells") {
			// File/stdin inputs resolve later in the runner. Check agreement in
			// cellsSetInputWithNote after the owning parser has decoded them.
			return nil
		}
		if !c.Flags().Changed("range") && !c.Flags().Changed("start-cell") {
			return sheetsValidationForFlag(source, "--%s needs a target --range (e.g. --range D19) or --start-cell (e.g. --start-cell D19), or put it in --cells as a cell field", source)
		}
		payload, err := json.Marshal([][]map[string]interface{}{{{"formula": formula}}})
		if err != nil {
			return sheetsValidationForFlag(source, "could not encode the formula").WithCause(err)
		}
		return c.Flags().Set("cells", string(payload))
	}
}

func formulaFlagValue(changed func(string) bool, str func(string) string) (formula, source string, err error) {
	for _, name := range []string{"formula", "formulas"} {
		if !changed(name) {
			continue
		}
		value := str(name)
		if value == "" {
			return "", "", sheetsValidationForFlag(name, "--%s must be non-empty", name)
		}
		if source != "" && value != formula {
			return "", "", sheetsValidationForFlag(name, "--%s and --%s specify different formulas", source, name)
		}
		formula, source = value, name
	}
	return formula, source, nil
}

// checkCellsFormulaFlags accepts a redundant scalar formula only when the
// decoded payload writes that same formula to one cell. Extra style fields
// remain in the payload; a different value or extent must not silently win.
func checkCellsFormulaFlags(runtime flagView, cells []interface{}) error {
	for _, name := range []string{"formula", "formulas"} {
		if !runtime.Changed(name) {
			continue
		}
		if len(cells) == 1 {
			if row, ok := cells[0].([]interface{}); ok && len(row) == 1 {
				if cell, ok := row[0].(map[string]interface{}); ok && cell["formula"] == runtime.Str(name) {
					continue
				}
			}
		}
		return sheetsValidationForFlag(name, "--%s disagrees with --cells; it must match the formula in a single cell", name).
			WithHint("keep one source, or put the formula and other cell fields together in --cells")
	}
	return nil
}

// ─── +cells-set-style --bold / --font-bold → --font-weight ───────────

func registerBoldFlags(cmd *cobra.Command) {
	for _, name := range []string{"bold", "font-bold"} {
		if cmd.Flags().Lookup(name) != nil {
			continue
		}
		cmd.Flags().Bool(name, false, "bold on; resolved to --font-weight")
		_ = cmd.Flags().MarkHidden(name)
	}

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		// Resolve explicit aliases before the envelope sees font-weight.
		if err := reconcileBoldFlags(c); err != nil {
			return err
		}
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		// The envelope itself may carry a bold alias when none was explicit.
		return reconcileBoldFlags(c)
	}
}

func reconcileBoldFlags(c *cobra.Command) error {
	if c.Flags().Changed("font-weight") {
		return nil
	}
	var weight, source string
	for _, name := range []string{"bold", "font-bold"} {
		if !c.Flags().Changed(name) {
			continue
		}
		bold, err := c.Flags().GetBool(name)
		if err != nil {
			return sheetsValidationForFlag(name, "%s must be a boolean", name).WithCause(err)
		}
		candidate, _ := styleOnOffWord("bold", "normal")(bold)
		if source != "" && candidate != weight {
			return sheetsValidationForFlag(source, "--%s and --%s specify conflicting bold values", source, name)
		}
		weight, source = candidate, name
	}
	if source == "" {
		return nil
	}
	return c.Flags().Set("font-weight", weight)
}

// ─── +cells-get --include-* aliases → --include ──────────────────────

func registerIncludeAliases(cmd *cobra.Command) {
	type aliasSpec struct {
		name, member string
	}
	aliases := []aliasSpec{
		{"include-formula", "formula"}, {"include-formulas", "formula"},
		{"include-style", "style"}, {"include-styles", "style"},
	}
	for _, a := range aliases {
		if cmd.Flags().Lookup(a.name) != nil {
			continue
		}
		cmd.Flags().Bool(a.name, false, "resolved onto --include "+a.member)
		_ = cmd.Flags().MarkHidden(a.name)
	}

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		members := c.Flags().Lookup("include").Value.String()
		have := map[string]bool{}
		for _, m := range strings.Split(members, ",") {
			if m = strings.TrimSpace(m); m != "" {
				have[m] = true
			}
		}
		changed := false
		for _, a := range aliases {
			if v, _ := c.Flags().GetBool(a.name); v && !have[a.member] {
				members = strings.TrimPrefix(members+","+a.member, ",")
				have[a.member] = true
				changed = true
			}
		}
		if changed {
			return c.Flags().Set("include", members)
		}
		return nil
	}
}

// ─── small helper: a flag accepted and ignored ───────────────────────

func registerIgnoredFlag(cmd *cobra.Command, name, usage string) {
	if cmd.Flags().Lookup(name) != nil {
		return
	}
	cmd.Flags().String(name, "", usage)
	_ = cmd.Flags().MarkHidden(name)
}

// registerSheetOnlyDocType accepts the generic export spelling when it agrees
// with this shortcut's fixed spreadsheet type. Other values must fail rather
// than parse successfully and silently export a different type.
func registerSheetOnlyDocType(cmd *cobra.Command) {
	registerIgnoredFlag(cmd, "doc-type", "only sheet is supported; this command exports spreadsheet sheets")
	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if !c.Flags().Changed("doc-type") {
			return nil
		}
		docType := strings.ToLower(strings.TrimSpace(c.Flags().Lookup("doc-type").Value.String()))
		if docType == "" || docType == "sheet" {
			return nil
		}
		return sheetsValidationForFlag("doc-type", "+workbook-export exports spreadsheet sheets; --doc-type %q is not supported", docType)
	}
}

// ─── --sheet-name / --sheet-id on the payload-per-sheet commands ──────

func registerEchoedSheetSelector(cmd *cobra.Command) {
	if cmd.Flags().Lookup("sheet-name") != nil {
		return // the command really has a selector; nothing to reconcile
	}
	// Display-name echoes are checked offline after input resolution. The
	// declared --sheet-id constraint is checked separately by the owner
	// against live workbook metadata before any mutation.
	cmd.Flags().String("sheet-name", "", "accepted when it names a sub-sheet the payload already names; the payload's own `name` is the carrier")
	_ = cmd.Flags().MarkHidden("sheet-name")
}

// checkEchoedSheetName compares --sheet-name against the sub-sheets the
// payload names. Called from Validate, NOT from PreRunE: @file and stdin are
// resolved by the shared runner between the two, so a PreRunE check saw
// "@payload.json" as a literal, failed to parse it, and waved the call
// through — `--sheet-name Expected --sheets @other.json` wrote to Other.
func checkEchoedSheetName(runtime *common.RuntimeContext, payloadFlag string) error {
	want := strings.TrimSpace(runtime.Str("sheet-name"))
	if want == "" {
		return nil
	}
	names, ok := payloadSheetNames(runtime.Str(payloadFlag), payloadFlag)
	if !ok || len(names) == 0 {
		// The payload does not parse here either; its own parser reports that
		// with row/column context, and guessing a target from it would be the
		// mis-targeted write this check exists to prevent.
		return nil
	}
	for _, n := range names {
		if n == want {
			return nil // the two agree — nothing to resolve
		}
	}
	return common.ValidationErrorf(
		"--sheet-name %q does not match any sub-sheet in --%s (%s); each --%s item names its own sheet",
		want, payloadFlag, strings.Join(quoteAll(names), ", "), payloadFlag).
		WithParam("--sheet-name")
}

// payloadSheetNames pulls the `name` of every sub-sheet out of the section
// consumed by payloadFlag, in both the envelope and bare-array spellings.
// Sibling sections are deliberately ignored: a `styles` item cannot authorize
// `--sheet-name` for a +table-put whose actual `sheets` target is different.
func payloadSheetNames(raw, payloadFlag string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	// The SAME repair the real parser applies. A strict decode here failed on
	// a single-quoted payload, found no names, and waved the call through to
	// whatever sheet that payload actually named.
	raw = canonicalPayloadJSON(raw)
	type entry struct {
		Name      string `json:"name"`
		Sheet     string `json:"sheet"`
		SheetName string `json:"sheet_name"`
		Title     string `json:"title"`
	}
	entryName := func(e entry) string {
		if e.Name != "" {
			return e.Name
		}
		if payloadFlag == "sheets" {
			return e.SheetName
		}
		// Keep this order aligned with foldStyleItemKeys: when a styles item
		// uses the accepted sheet-name aliases, reconciliation must target the
		// same exact sheet name the payload parser will preserve.
		for _, alias := range []string{e.Sheet, e.SheetName, e.Title} {
			if alias != "" {
				return alias
			}
		}
		return ""
	}
	var entries []entry
	var bare []entry
	if json.Unmarshal([]byte(raw), &bare) == nil {
		entries = bare
	} else {
		// Decode only the active section as RawMessage. A malformed sibling must
		// not make reconciliation ignore a valid target section that its real
		// parser accepts independently.
		var envelope map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &envelope) != nil {
			return nil, false
		}
		section, ok := envelope[payloadFlag]
		if !ok || json.Unmarshal(section, &entries) != nil {
			return nil, false
		}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if n := entryName(e); n != "" {
			out = append(out, n)
		}
	}
	return out, len(out) > 0
}

func quoteAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strconv.Quote(s)
	}
	return out
}

// ─── --dimension on the commands that read the axis off the span ──────

func registerEchoedDimension(cmd *cobra.Command) {
	if cmd.Flags().Lookup("dimension") != nil {
		return
	}
	cmd.Flags().String("dimension", "", "accepted when it agrees with the axis --range / --position already names (rows are numbers, columns are letters)")
	_ = cmd.Flags().MarkHidden("dimension")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if !c.Flags().Changed("dimension") {
			return nil
		}
		raw := c.Flags().Lookup("dimension").Value.String()
		stated := normalizeDimensionWord(raw)
		if stated == "" {
			// This flag is registered here, dynamically, so it is absent from
			// flag-defs and the enum normalizer never sees it — nothing
			// downstream would catch a word like "bananas". Letting it through
			// would run a DELETE the caller never confirmed the axis of.
			return common.ValidationErrorf("invalid value %q for --dimension, allowed: row, column", raw).
				WithParam("--dimension")
		}
		spanFlag := "range"
		if c.Name() == "+dim-insert" {
			spanFlag = "position"
		}
		fl := c.Flags().Lookup(spanFlag)
		if fl == nil {
			return nil
		}
		implied := dimensionOfSpan(fl.Value.String())
		if implied == "" || implied == stated {
			return nil // agrees, or the span does not say
		}
		return common.ValidationErrorf(
			"--dimension %s disagrees with --%s %q, which names a %s; this command takes the axis from the span (rows are numbers like \"3:5\", columns are letters like \"C:E\")",
			stated, spanFlag, fl.Value.String(), implied).
			WithParam("--dimension")
	}
}

// normalizeDimensionWord folds the spellings of the two axes onto "row" /
// "column". Anything else returns "" and is left to the enum normalizer.
func normalizeDimensionWord(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "row", "rows", "row_dimension", "rowdimension":
		return "row"
	case "column", "columns", "col", "cols", "column_dimension", "columndimension":
		return "column"
	}
	return ""
}

// dimensionOfSpan reads the axis out of an A1 span or position: digits are
// rows, letters are columns. Returns "" when the value says neither.
func dimensionOfSpan(v string) string {
	// Dimension reconciliation runs before the range's sheet qualifier is
	// moved into --sheet-name; inspect the A1 span without changing its target.
	_, v = splitDimSpanSheetPrefix(v)
	if v == "" {
		return ""
	}
	if i := strings.Index(v, ":"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	if v == "" {
		return ""
	}
	if _, err := strconv.Atoi(v); err == nil {
		return "row"
	}
	if letterToColumnIndex(v) >= 0 {
		return "column"
	}
	return ""
}

// ─── --ranges holding the one span --range takes ──────────────────────

func registerSingletonRanges(cmd *cobra.Command) {
	if cmd.Flags().Lookup("ranges") != nil || cmd.Flags().Lookup("range") == nil {
		return
	}
	cmd.Flags().String("ranges", "", "accepted when it holds exactly one span; +cells-unmerge takes one span per call")
	_ = cmd.Flags().MarkHidden("ranges")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if !c.Flags().Changed("ranges") {
			return nil
		}
		raw := strings.TrimSpace(c.Flags().Lookup("ranges").Value.String())
		list, ok := jsonStringList(raw)
		if !ok {
			return nil // let the missing --range report itself
		}
		if len(list) > 1 {
			// The standing prescription: several regions need several calls,
			// and picking the first would silently unmerge one of them.
			return common.ValidationErrorf(
				"+cells-unmerge takes one span per call; --ranges holds %d", len(list)).
				WithParam("--ranges").
				WithHint("issue one call per span, or carry them all in a single +batch-update")
		}
		target := c.Flags().Lookup("range")
		if c.Flags().Changed("range") {
			// Two explicit targets. Agreeing is the redundancy this whole file
			// accepts; disagreeing is the caller naming two different spans,
			// and picking one would unmerge a region they did not ask about.
			if strings.EqualFold(strings.TrimSpace(target.Value.String()), strings.TrimSpace(list[0])) {
				return nil
			}
			return common.ValidationErrorf("--range %q and --ranges %q name different spans; keep one", target.Value.String(), list[0]).
				WithParam("--ranges")
		}
		if err := target.Value.Set(list[0]); err != nil {
			return err
		}
		// Changed as well as Value: the required-flag check reads Changed, and
		// a flag filled from the payload is one the caller did supply.
		target.Changed = true
		return nil
	}
}

// ─── --style / --styles, the OpenAPI's envelope, on the flat-flag command ──
//
// +cells-set-style spells every style field as its own flag; the Lark
// OpenAPI's own request nests them under `style`, so a caller writing from
// those docs sends `--style '{"font_weight":"bold"}'`. 865 calls in the
// 09-11..15 catalog. The standing prescription says to pass each field on its
// own, which is right — and the object already says which ones, so it can be
// unpacked instead of described.
//
// Use the declarative style parser so envelopes accept the same scalar and
// border vocabulary as --styles. Validate the complete object before setting
// flags; explicitly supplied flags retain precedence over the envelope.

func registerStyleEnvelope(cmd *cobra.Command) {
	if cmd.Flags().Lookup("style") != nil {
		return
	}
	for _, name := range []string{"style", "styles"} {
		cmd.Flags().String(name, "", "accepted as the OpenAPI's {style:{…}} envelope; its fields are unpacked onto this command's own flags")
		_ = cmd.Flags().MarkHidden(name)
	}

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		raw, source, err := styleEnvelopeValue(c.Flags().Changed, func(name string) string {
			return c.Flags().Lookup(name).Value.String()
		})
		if err != nil || source == "" {
			return err
		}
		unpacked, err := styleEnvelopeFlagsWithExplicit(raw, source, c.Flags().Changed)
		if err != nil {
			return err
		}
		for name, value := range unpacked {
			fl := c.Flags().Lookup(name)
			if fl == nil {
				return sheetsValidationForFlag(source, "--%s.%s is not a field of this command", source, name)
			}
			text, _ := scalarFlagText(value) // the envelope parser validates scalar fields
			if c.Flags().Changed(name) {
				if name == "range" && text != fl.Value.String() {
					return sheetsValidationForFlag(source, "--%s.range conflicts with --range; keep one target range", source)
				}
				continue // an explicit flag outranks the envelope
			}
			if err := fl.Value.Set(text); err != nil {
				return common.ValidationErrorf("--%s.%s: %v", source, name, err).WithParam("--" + source)
			}
			fl.Changed = true
		}
		return nil
	}
}

// styleEnvelopeFlagsWithExplicit decodes an OpenAPI style envelope using the
// shared declarative vocabulary, but first removes fields already
// supplied as flat flags. That precedence has to happen before validation:
// a caller who explicitly sets --font-weight bold has supplied the effective
// value, so an invalid shadowed {"font_weight":"..."} cannot reject the
// command or make an unrelated envelope field silently win.
func styleEnvelopeFlagsWithExplicit(raw, source string, explicit func(string) bool) (map[string]interface{}, error) {
	// Check object shape and retain Unmarshal's error causes without rounding numbers.
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return nil, sheetsValidationForFlag(source, "--%s must be a JSON object of style fields", source).WithCause(err)
	}
	decoded, err := decodeValuesPayload(raw)
	if err != nil {
		return nil, sheetsValidationForFlag(source, "--%s must be a JSON object of style fields", source).WithCause(err)
	}
	obj, _ := decoded.(map[string]interface{}) // Object shape was checked above; null remains empty.
	if len(obj) == 0 {
		return nil, sheetsValidationForFlag(source, "--%s must be a non-empty JSON object of style fields", source)
	}
	filterExplicitStyleEnvelopeFields(obj, explicit)
	// Envelopes previously resolved canonical flag spellings through pflag.
	// Preserve that spelling support and the existing bold precedence before
	// passing the object to the shared declarative parser.
	fields := make(map[string]interface{}, len(obj))
	for _, key := range sortedKeys(obj) {
		field := key
		for _, aliases := range []map[string]string{domainFlagAliases, commandFlagAliases["+cells-set-style"]} {
			for _, alias := range sortedKeys(aliases) {
				target := strings.ReplaceAll(aliases[alias], "-", "_")
				if squashStyleFieldKey(key) == squashStyleFieldKey(alias) && (workbookCreateCellStyleField(target) || target == "border_styles") {
					field = target
					if target == "border_styles" {
						field = strings.ReplaceAll(alias, "-", "_")
					}
					break
				}
			}
		}
		for _, canonical := range workbookCreateCellStyleFieldList {
			if squashStyleFieldKey(key) == squashStyleFieldKey(canonical) {
				field = canonical
				break
			}
		}
		// border-style is the flat command's scalar all-sides shorthand. Keep
		// that spelling until foldBorderFamilyAliases turns it into the nested
		// border_styles object; mapping it to border_styles here would leave the
		// scalar value where an object is required.
		if squashStyleFieldKey(key) == "borderstyle" {
			field = "border_style"
		}
		if squashStyleFieldKey(key) == "bold" {
			field = "bold"
		} else if squashStyleFieldKey(key) == "fontbold" {
			field = "font-bold"
		}
		if _, duplicate := fields[field]; duplicate {
			return nil, sheetsValidationForFlag(source, "--%s.%s conflicts with another spelling of %s", source, key, field)
		}
		fields[field] = obj[key]
	}
	if err := foldSubOpBoldAliases(fields); err != nil {
		var validation *errs.ValidationError
		if errors.As(err, &validation) {
			return nil, validation.WithParam("--" + source)
		}
		return nil, err
	}
	if weight, ok := fields["font-weight"]; ok {
		fields["font_weight"] = weight
		delete(fields, "font-weight")
	}
	flags := map[string]interface{}{}
	if border, ok := fields["border_styles"].(string); ok {
		// String borders already use the composite flag's own parser, including
		// @file/stdin through invocation-scoped FileIO. Keep that path intact.
		if hasBorderFamilyKey(fields) {
			return nil, sheetsValidationForFlag(source, "--%s.border_styles conflicts with another border spelling; keep one", source)
		}
		flags["border-styles"] = border
		delete(fields, "border_styles")
	}
	if rawRange, present := fields["range"]; present {
		rng, ok := rawRange.(string)
		if !ok || strings.TrimSpace(rng) == "" {
			return nil, sheetsValidationForFlag(source, "--%s.range must be a non-empty A1 range string", source)
		}
		flags["range"] = rng
		delete(fields, "range")
	}
	proto, err := normalizeWorkbookCreateStyleObject(fields, "--"+source)
	if err != nil {
		var validation *errs.ValidationError
		if errors.As(err, &validation) {
			return nil, validation.WithParam("--" + source)
		}
		return nil, err
	}
	if scalars, ok := proto["cell_styles"].(map[string]interface{}); ok {
		for _, field := range sortedKeys(scalars) {
			// Numeric flag values retain the float64 contract of batch flag views.
			// Alignment enums have already been checked and mapped exactly above.
			if number, ok := scalars[field].(json.Number); ok {
				value, err := number.Float64()
				if err != nil {
					return nil, sheetsValidationForFlag(source, "--%s.%s must be a representable number", source, field).WithCause(err)
				}
				scalars[field] = value
			}
			_, ok := scalarFlagText(scalars[field])
			if !ok {
				return nil, sheetsValidationForFlag(source, "--%s.%s must be a string or number", source, field)
			}
			flags[strings.ReplaceAll(field, "_", "-")] = scalars[field]
		}
	}
	if border, ok := proto["border_styles"]; ok {
		text, err := json.Marshal(border)
		if err != nil {
			return nil, errs.NewInternalError(errs.SubtypeSDKError, "cannot encode normalized style borders").WithCause(err)
		}
		flags["border-styles"] = string(text)
	}
	return flags, nil
}

func filterExplicitStyleEnvelopeFields(obj map[string]interface{}, explicit func(string) bool) {
	if explicit == nil || len(obj) == 0 {
		return
	}
	for key, value := range obj {
		// A target is not a style default. Keep it so the caller can reject a
		// conflicting explicit range instead of silently discarding a target.
		if squashStyleFieldKey(key) == "range" {
			continue
		}
		if squashStyleFieldKey(key) == "alignment" {
			alignment, ok := value.(map[string]interface{})
			if !ok || len(alignment) == 0 {
				continue
			}
			for name := range alignment {
				field := nestedAlignmentStyleField(name)
				if field != "" && explicit(strings.ReplaceAll(field, "_", "-")) {
					delete(alignment, name)
				}
			}
			if len(alignment) == 0 {
				delete(obj, key)
			}
			continue
		}
		if squashStyleFieldKey(key) == "font" {
			font, ok := value.(map[string]interface{})
			if !ok || len(font) == 0 {
				continue // preserve malformed effective data for normal validation
			}
			for name := range font {
				if flag := nestedFontEnvelopeFlag(name); flag != "" && explicit(flag) {
					delete(font, name)
				}
			}
			if len(font) == 0 {
				delete(obj, key)
			}
			continue
		}
		if flag := styleEnvelopeFlatFlag(key); flag != "" && explicit(flag) {
			delete(obj, key)
			continue
		}
		if isStyleEnvelopeBorderField(key) && explicit("border-styles") {
			delete(obj, key)
		}
	}
}

// isStyleEnvelopeBorderField recognizes exactly the flattened border vocabulary
// foldBorderFamilyAliases accepts. It must not use a broad substring match:
// explicit --border-styles may shadow an accepted alias, never make an
// unrelated unknown `*_border_*` field silently disappear.
func isStyleEnvelopeBorderField(field string) bool {
	key := strings.ToLower(strings.TrimSpace(field))
	key = strings.ReplaceAll(key, "-", "_")
	if key == "border" || key == "borders" || key == "border_styles" || key == "border_type" {
		return true
	}
	parts := strings.Split(key, "_")
	side := func(s string) bool {
		switch s {
		case "top", "bottom", "left", "right", "all":
			return true
		default:
			return false
		}
	}
	attr := func(s string) bool {
		switch s {
		case "style", "color", "weight", "width":
			return true
		default:
			return false
		}
	}
	switch len(parts) {
	case 2:
		return (parts[0] == "border" && (side(parts[1]) || attr(parts[1]))) ||
			(side(parts[0]) && parts[1] == "border")
	case 3:
		return (parts[0] == "border" && side(parts[1]) && attr(parts[2])) ||
			(side(parts[0]) && parts[1] == "border" && attr(parts[2]))
	default:
		return false
	}
}

func styleEnvelopeFlatFlag(field string) string {
	switch squashStyleFieldKey(field) {
	case "fontweight", "bold", "fontbold":
		return "font-weight"
	case "fontstyle", "italic", "fontitalic":
		return "font-style"
	case "fontline", "underline", "fontunderline":
		return "font-line"
	case "fontcolor", "textcolor", "forecolor":
		return "font-color"
	case "fontfamily", "fontname":
		return "font-family"
	case "fontsize":
		return "font-size"
	case "backgroundcolor", "backcolor", "bgcolor", "fillcolor":
		return "background-color"
	case "horizontalalignment", "horizontalalign", "halign", "textalign", "horizontal":
		return "horizontal-alignment"
	case "verticalalignment", "verticalalign", "valign", "vertical":
		return "vertical-alignment"
	case "wordwrap", "wraptext", "textwrap", "wrapstrategy", "wrap":
		return "word-wrap"
	case "numberformat":
		return "number-format"
	case "borderstyles", "bordertype", "border", "borderall", "borderstyle":
		return "border-styles"
	case "range":
		return "range"
	default:
		return ""
	}
}

func nestedFontEnvelopeFlag(field string) string {
	switch squashStyleFieldKey(field) {
	case "size", "fontsize":
		return "font-size"
	case "name", "family", "fontname", "fontfamily":
		return "font-family"
	case "color", "fontcolor", "forecolor", "textcolor":
		return "font-color"
	case "weight", "fontweight", "bold", "fontbold":
		return "font-weight"
	case "italic", "fontitalic":
		return "font-style"
	case "underline", "fontunderline":
		return "font-line"
	default:
		return ""
	}
}

func styleEnvelopeValue(changed func(string) bool, str func(string) string) (raw, source string, err error) {
	for _, name := range []string{"style", "styles"} {
		if !changed(name) {
			continue
		}
		value := str(name)
		if strings.TrimSpace(value) == "" {
			return "", "", sheetsValidationForFlag(name, "--%s must be non-empty", name)
		}
		if source != "" && value != raw {
			return "", "", sheetsValidationForFlag(name, "--%s and --%s specify different envelopes", source, name)
		}
		raw, source = value, name
	}
	return raw, source, nil
}

// scalarFlagText renders a JSON scalar as the text a flag takes. Objects and
// arrays report false: they belong to a composite flag of their own.
func scalarFlagText(v interface{}) (string, bool) {
	switch value := v.(type) {
	case string:
		return value, true
	case bool:
		return strconv.FormatBool(value), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	case json.Number:
		return value.String(), true
	}
	return "", false
}

// jsonStringList reads raw as a non-empty JSON list of strings.
//
// It answers a question, not an error: a --ranges that is not such a list is
// not a --ranges this reconciler can speak for, and the caller's response is
// the same however it failed — stand aside and let the missing --range report
// itself. So the parse failure is reported as ok=false rather than returned.
func jsonStringList(raw string) ([]string, bool) {
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, false
	}
	return list, len(list) > 0
}
