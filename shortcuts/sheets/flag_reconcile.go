// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strconv"
	"strings"

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
		registerEchoedDimension(cmd)
	case "+cells-unmerge":
		registerSingletonRanges(cmd)
	case "+cells-set-style", "+cells-batch-set-style":
		registerStyleEnvelope(cmd)
	}
}

// ─── --sheet-name / --sheet-id on the payload-per-sheet commands ──────

func registerEchoedSheetSelector(cmd *cobra.Command) {
	if cmd.Flags().Lookup("sheet-name") != nil {
		return // the command really has a selector; nothing to reconcile
	}
	// Only --sheet-name. A reference_id cannot be reconciled against a
	// payload that names its sub-sheets by DISPLAY NAME without resolving it
	// against the live workbook, and accepting a targeting parameter that
	// neither selects nor validates would let a write land on a sheet the
	// caller did not mean. --sheet-id keeps its prescription.
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
		"--sheet-name %q does not match any sub-sheet in --%s (%s); this command has no sheet selector — each --%s item names its own sheet",
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
		if e.Name != "" || payloadFlag != "styles" {
			return e.Name
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
		if c.Flags().Lookup(spanFlag) == nil {
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
	v = strings.TrimSpace(v)
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

// ─── --style, the OpenAPI's envelope, on the flat-flag command ────────
//
// +cells-set-style spells every style field as its own flag; the Lark
// OpenAPI's own request nests them under `style`, so a caller writing from
// those docs sends `--style '{"font_weight":"bold"}'`. 865 calls in the
// 09-11..15 catalog. The standing prescription says to pass each field on its
// own, which is right — and the object already says which ones, so it can be
// unpacked instead of described.
//
// Only keys that ARE flags on this command are taken. One that is not keeps
// the object intact and fails through the existing unknown-field path, where
// the style vocabulary's prescriptions can answer it — unpacking half a spec
// and silently dropping the rest is the failure mode this avoids.

func registerStyleEnvelope(cmd *cobra.Command) {
	if cmd.Flags().Lookup("style") != nil {
		return
	}
	cmd.Flags().String("style", "", "accepted as the OpenAPI's {style:{…}} envelope; its fields are unpacked onto this command's own flags")
	_ = cmd.Flags().MarkHidden("style")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if !c.Flags().Changed("style") {
			return nil
		}
		raw := strings.TrimSpace(c.Flags().Lookup("style").Value.String())
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &obj); err != nil || len(obj) == 0 {
			return common.ValidationErrorf("--style must be a JSON object of style fields, e.g. {\"font_weight\":\"bold\",\"font_color\":\"#FF0000\"}").
				WithParam("--style")
		}
		unpacked := make(map[string]string, len(obj))
		for k, v := range obj {
			flagName := strings.ReplaceAll(k, "_", "-")
			target := c.Flags().Lookup(flagName)
			if target == nil || target.Name == "style" {
				return common.ValidationErrorf("--style carries %q, which is not a field of this command; pass each field as its own flag", k).
					WithParam("--style")
			}
			text, ok := scalarFlagText(v)
			if !ok {
				return common.ValidationErrorf("--style.%s must be a string or number; composite fields like border_styles have their own flag", k).
					WithParam("--style")
			}
			unpacked[flagName] = text
		}
		for name, text := range unpacked {
			if c.Flags().Changed(name) {
				continue // an explicit flag outranks the envelope
			}
			fl := c.Flags().Lookup(name)
			if err := fl.Value.Set(text); err != nil {
				return common.ValidationErrorf("--style.%s: %v", name, err).WithParam("--style")
			}
			fl.Changed = true
		}
		return nil
	}
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
