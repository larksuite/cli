// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── style vocabulary acceptance layer ────────────────────────────────
//
// The single home for how the sheets domain ACCEPTS style vocabulary, across
// all three carrier paths that end in set_cell_range bodies:
//
//   flag path       +cells-set-style / +cells-batch-set-style flat flags
//   typed cells     +cells-set --cells cell objects (incl. batch sub-ops)
//   styles payload  --styles on +workbook-create / +table-put / +styles-put
//
// Design contract (established in the 2026-07 batch-update overhaul; see the
// acceptance tests in styles_acceptance_test.go):
//
//   - ONE canonical form, documented; a WIDE acceptance layer, undocumented.
//     Model priors are divergent (one eval batch produced six different
//     border spellings), so no canonical structure can make first tries
//     succeed — acceptance is normalized here instead, never per-call-site.
//   - Every rewrite must be unambiguous; ambiguous guesses (fore_color) get
//     a targeted prescription, never a silent pick. Silent ignoring and
//     bare rejection are both bugs.
//   - SILENT-ALIAS ADMISSION BAR (2026-07-21): only words from REAL external
//     vocabularies (Excel/openpyxl, CSS, Google Sheets API), recurring
//     across batches or ≥3 tasks in one, with zero semantic ambiguity.
//     Spelling/word-order permutations do NOT get aliases — they are
//     absorbed by the universal did-you-mean rejection (one self-healing
//     retry, zero per-variant code). Real vocabularies are a finite set;
//     permutations are not. Earlier permutation aliases are grandfathered.
//   - Closure is enforced by two test properties: vocabulary parity (every
//     flag-path style must be accepted on the payload paths) and the prior
//     corpus (every observed model spelling either normalizes or
//     prescribes). New eval finding → corpus row → fix HERE → locked.

// sortedKeys returns a map's keys in sorted order, so any loop that can abort
// with an error reports a deterministic one. Used throughout this file: the
// acceptance layer is all map-shaped vocabulary, and "which of my three bad
// fields did it complain about" must not change between runs.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ─── style flags (shared by +cells-set-style and +cells-batch-set-style) ─

// buildCellStyleFromFlags reads the 12 flat style flags and returns the
// cell_styles map expected by set_cell_range. Skips any flag the user
// didn't set so partial styles work.
func buildCellStyleFromFlags(runtime flagView) map[string]interface{} {
	style := map[string]interface{}{}
	if v := runtime.Str("background-color"); v != "" {
		style["background_color"] = v
	}
	if v := runtime.Str("font-color"); v != "" {
		style["font_color"] = v
	}
	if v := runtime.Str("font-family"); v != "" {
		style["font_family"] = v
	}
	if runtime.Changed("font-size") && runtime.Float64("font-size") > 0 {
		style["font_size"] = runtime.Float64("font-size")
	}
	if v := runtime.Str("font-style"); v != "" {
		style["font_style"] = v
	}
	if v := runtime.Str("font-weight"); v != "" {
		style["font_weight"] = v
	}
	if v := runtime.Str("font-line"); v != "" {
		style["font_line"] = v
	}
	if v := runtime.Str("horizontal-alignment"); v != "" {
		style["horizontal_alignment"] = v
	}
	if v := runtime.Str("vertical-alignment"); v != "" {
		style["vertical_alignment"] = v
	}
	if v := runtime.Str("word-wrap"); v != "" {
		style["word_wrap"] = v
	}
	if v := runtime.Str("number-format"); v != "" {
		style["number_format"] = v
	}
	return style
}

// cellStyleAliases maps shorthand cell_styles field names that models commonly
// hallucinate (Excel / openpyxl / CSS conventions) onto the canonical field
// names the backend expects. Entries here are pure renames; a word whose
// VALUE also has to be read (bold, italic) belongs in cellStyleValueAliases.
// Ambiguous guesses (e.g. "color", "bg_color", "text_align") are intentionally
// left out either way, so a wrong guess still surfaces as an error rather than
// being silently reinterpreted.
var cellStyleAliases = []struct{ alias, canonical string }{
	// openpyxl's Font(name=…) and xlsxwriter's {'font_name': …} name this
	// CLI's font_family and nothing else — a real external vocabulary with
	// one reading, which is what the admission bar asks for. 09-04..07: 511
	// rejections.
	{"font_name", "font_family"},
	{"horizontal_align", "horizontal_alignment"},
	{"halign", "horizontal_alignment"},
	{"vertical_align", "vertical_alignment"},
	{"valign", "vertical_alignment"},
	// wrap family: word_wrap is the sole wrap concept, no ambiguity. 07-20
	// eval: wrap_text alone produced an 88-issue retry loop on --styles;
	// wrap_strategy (the Google Sheets API word) followed on 07-21.
	{"wrap_text", "word_wrap"},
	{"text_wrap", "word_wrap"},
	{"wrap_strategy", "word_wrap"},
}

// cellStyleValueAliases carries the style words that name a field this
// contract does have while spelling its VALUE as a flag rather than an enum:
// openpyxl's Font(bold=True, italic=True, underline="single") and
// xlsxwriter's {'bold': 1} are the two vocabularies every model has read, and
// both put the concept in the key and the on/off in the value. The rename
// alone would land a boolean under font_weight, so each entry carries its own
// value reading and only fires when that reading succeeds — italic:true is
// font_style:"italic", while italic:"sort of" keeps the prescription below.
// 09-04..07: italic 1546 rejections, font_bold 135, on payloads whose intent
// the error text already spelled out.
var cellStyleValueAliases = []struct {
	alias, canonical string
	value            func(interface{}) (string, bool)
}{
	{"bold", "font_weight", styleOnOffWord("bold", "normal")},
	{"font_bold", "font_weight", styleOnOffWord("bold", "normal")},
	{"italic", "font_style", styleOnOffWord("italic", "normal")},
	{"font_italic", "font_style", styleOnOffWord("italic", "normal")},
	{"underline", "font_line", styleUnderlineWord},
	{"font_underline", "font_line", styleUnderlineWord},
}

// styleOnOffWord reads the flag-shaped value of a two-state style field: the
// booleans and 0/1 both vocabularies use, and the enum word itself for a
// caller who wrote the right value under the wrong key. Anything else returns
// false and leaves the field to its prescription.
func styleOnOffWord(on, off string) func(interface{}) (string, bool) {
	return func(raw interface{}) (string, bool) {
		switch v := raw.(type) {
		case bool:
			if v {
				return on, true
			}
			return off, true
		case float64:
			switch v {
			case 1:
				return on, true
			case 0:
				return off, true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", on:
				return on, true
			case "false", "0", "no", "none", off:
				return off, true
			}
		}
		return "", false
	}
}

// styleUnderlineWord reads openpyxl's underline vocabulary, where the value is
// which underline rather than whether: single / double / the accounting
// variants all draw one, and this contract carries the one line style.
func styleUnderlineWord(raw interface{}) (string, bool) {
	if s, isStr := raw.(string); isStr {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "single", "double", "singleaccounting", "doubleaccounting":
			return "underline", true
		}
	}
	return styleOnOffWord("underline", "none")(raw)
}

// styleFieldPrescriptions carries the exact fix for high-frequency
// unsupported cell_styles field names where the edit-distance suggester is
// actively misleading (07-28 root-cause report: font_bold drew "did you mean
// font_color?" and nested font drew "font_line" — an agent that follows
// either burns a second failed round trip). Keyed by lowercased field name;
// the text replaces the did-you-mean on the unsupported-field error. These
// stay prescriptions, not silent aliases: text_align is on the deliberate
// no-alias list above, and the bold / italic / underline rows now answer only
// the values cellStyleValueAliases could not read.
var styleFieldPrescriptions = map[string]string{
	"bold":       `bold text is font_weight:"bold"`,
	"font_bold":  `bold text is font_weight:"bold"`,
	"italic":     `italic text is font_style:"italic"`,
	"underline":  `underline is font_line:"underline"`,
	"text_align": "horizontal text alignment is horizontal_alignment (left/center/right)",
	"font":       `cell_styles has no nested font object — use the flat font_* fields (font:{"bold":true,"size":18,"color":"#000"} becomes font_weight:"bold", font_size:18, font_color:"#000")`,
	// The OpenAPI's own request shape is {range, style:{…}}, so a cell_styles
	// item written from the API docs nests one level too deep. Distance-based
	// suggestion is useless here (the fix is structural, not a rename) and the
	// bare "style is not a supported style field" reads like the whole payload
	// shape is wrong. 08-18..24 eval, --styles group.
	"style": `cell_styles has no nested style object — the style fields sit directly on the item, next to range ({"range":"A1:B2","style":{"font_weight":"bold"}} becomes {"range":"A1:B2","font_weight":"bold"})`,
	// bg_color / text_color read unambiguously (unlike fore_color, which is
	// rejected as ambiguous above) but stay prescriptions rather than silent
	// aliases: they are spelling permutations, not words from a real external
	// vocabulary, and the silent-alias admission bar excludes those.
	"bg_color":   "the cell fill is background_color",
	"fill_color": "the cell fill is background_color",
	"text_color": "the text color is font_color",
	// 08-29..31 reflow, --styles unknown-field group (96 rejections on
	// +styles-put alone). These name real concepts that the payload does
	// carry — just not inside a cell_styles item, so the fix is structural
	// and the field list alone does not spell it.
	"row_height":    `row height is a sheet-level row_sizes entry, not a cell style ({"row_sizes":[{"range":"1:1","type":"pixel","size":30}]})`,
	"row_heights":   `row height is a sheet-level row_sizes entry, not a cell style ({"row_sizes":[{"range":"1:1","type":"pixel","size":30}]})`,
	"column_width":  `column width is a sheet-level col_sizes entry, not a cell style ({"col_sizes":[{"range":"A:C","type":"pixel","size":120}]})`,
	"col_width":     `column width is a sheet-level col_sizes entry, not a cell style ({"col_sizes":[{"range":"A:C","type":"pixel","size":120}]})`,
	"wrap":          `automatic line wrapping is word_wrap ("auto-wrap" to wrap, "overflow" to spill, "word-clip" to truncate)`,
	"unmerge_cells": "a styles payload only adds merges (cell_merges); undo an existing one with +cells-unmerge --range <A1 range>",
}

// styleItemKeyPrescriptions answers a key written on a styles[N] item that
// belongs one level deeper, on a cell_styles entry. The distance ranker cannot
// help: border_styles is six edits from cell_styles, and the fix is structural
// anyway. 08-29..31 reflow, +workbook-create and +table-put --styles.
var styleItemKeyPrescriptions = map[string]string{
	"borderstyles": `borders belong on a cell_styles entry, next to its range ({"cell_styles":[{"range":"A1:C1","border_styles":{"top":{"style":"solid"}}}]})`,
	"borders":      `borders belong on a cell_styles entry, next to its range ({"cell_styles":[{"range":"A1:C1","border":{"style":"solid","color":"#000000"}}]})`,
	"border":       `borders belong on a cell_styles entry, next to its range ({"cell_styles":[{"range":"A1:C1","border":{"style":"solid","color":"#000000"}}]})`,
	"style":        `a styles item carries cell_styles / cell_merges / row_sizes / col_sizes / freeze; the style fields themselves sit on a cell_styles entry next to its range`,
	"styles":       `a styles item carries cell_styles / cell_merges / row_sizes / col_sizes / freeze; the style fields themselves sit on a cell_styles entry next to its range`,
}

// borderFieldPrescription answers any unsupported border-family spelling that
// survived foldBorderFamilyAliases, which absorbs border / borders /
// border_<side> / border_<attr> / border_type / border_width and their
// word-order twins, in both the attribute-object and one-value forms. What
// reaches this text is a name none of those cover, so it gets the two
// canonical shapes rather than a did-you-mean: the distance ranker's answer
// for a border name is another border name, and the retry comes back with the
// same unusable value (08-18..24 eval).
const borderFieldPrescription = `borders go in border ({"border":{"style":"solid","weight":"thin","color":"#000000"}} — all four sides, or just {"border":"solid"}) or border_styles for per-side control ({"border_styles":{"bottom":{"style":"solid"}}}); style is solid/dashed/dotted/double/none, weight is thin/medium/thick`

// styleFieldPrescriptionsSquashed keys the curated table by letters alone, so
// every separator spelling of one mistake (border_type / borderType /
// border-type) resolves to the same prescription. Built once at init; the
// parity test asserts no two entries collide after squashing.
var styleFieldPrescriptionsSquashed = func() map[string]string {
	out := make(map[string]string, len(styleFieldPrescriptions))
	for k, v := range styleFieldPrescriptions {
		out[squashStyleFieldKey(k)] = v
	}
	return out
}()

// squashStyleFieldKey reduces a field name to its letters and digits, lowercased.
func squashStyleFieldKey(field string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(field) {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// styleFieldPrescriptionFor returns the curated fix for an unsupported
// cell_styles field name, or "" when the generic did-you-mean should answer
// instead. The border family gets one shared answer: enumerating its spelling
// permutations is endless, but every one of them has the same two-form fix.
func styleFieldPrescriptionFor(field string) string {
	key := squashStyleFieldKey(field)
	if rx, ok := styleFieldPrescriptionsSquashed[key]; ok {
		return rx
	}
	if strings.HasPrefix(key, "border") || strings.HasSuffix(key, "border") {
		return borderFieldPrescription
	}
	return ""
}

// cellStyleEnumFields sources the enum vocabulary for enum-bearing
// cell_styles fields from the +cells-set-style flag-defs, so the payload path
// (--styles / typed --cells) validates and canonicalizes values the same way
// the cobra flag path does. 07-20 eval: "vertical_alignment":"center" (CSS
// vocabulary; Lark spells it "middle") passed the CLI and burned a
// server-side round trip ~10 times — the flag path had normalized it since
// round 2, the payload path never did.
func cellStyleEnumFields() map[string][]string {
	defs, err := loadFlagDefs()
	if err != nil {
		return nil
	}
	spec, ok := defs["+cells-set-style"]
	if !ok {
		return nil
	}
	out := map[string][]string{}
	for _, df := range spec.Flags {
		if df.Kind != "own" || df.Type != "string" || len(df.Enum) == 0 {
			continue
		}
		out[strings.ReplaceAll(df.Name, "-", "_")] = df.Enum
	}
	return out
}

// cellStyleScalarTypes maps each scalar cell-style field to the JSON type
// flag-defs declares for it ("string" / "number"), derived from
// +cells-set-style's own flags so the two never drift. Composite fields
// (border / border_styles, whose values are objects) are excluded — they have
// their own structural validation.
func cellStyleScalarTypes() map[string]string {
	defs, err := loadFlagDefs()
	if err != nil {
		return nil
	}
	spec, ok := defs["+cells-set-style"]
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, df := range spec.Flags {
		if df.Kind != "own" {
			continue
		}
		name := strings.ReplaceAll(df.Name, "-", "_")
		switch name {
		case "range", "border_styles", "border":
			continue // locator / composite: validated structurally elsewhere
		}
		switch df.Type {
		case "string":
			out[name] = "string"
		case "float64", "int":
			out[name] = "number"
		}
	}
	return out
}

// normalizeCellStyleAliases renames known shorthand keys in a single
// cell_styles map to their canonical equivalents, in place, so a model that
// writes e.g. "horizontal_align" instead of "horizontal_alignment" still
// applies the style instead of hitting an "unsupported field" error (--styles)
// or having the field silently dropped by the backend (typed --cells). If both
// the shorthand and its canonical key are present it returns a validation error
// rather than picking one. It then canonicalizes enum VALUES (casing + known
// cross-vocabulary aliases like CSS "center" → Lark "middle"; boolean
// word_wrap → the enum) and rejects off-enum values client-side instead of
// letting the server fail the whole batch. path labels the map for errors.
// numericStyleValue reads a quoted number ("16", " 10.5 ") as the number a
// numeric style field wants. The test is "is this a JSON number literal", not
// "does Go parse it": strconv.ParseFloat also takes Inf / NaN / hex floats,
// none of which survive as JSON in the request body.
func numericStyleValue(raw interface{}) (float64, bool) {
	s, ok := raw.(string)
	if !ok {
		return 0, false
	}
	// Decoding into interface{} rather than float64: the literal "null"
	// unmarshals into a float64 without an error and leaves it at 0, which
	// would turn {"font_size":"null"} into a zero-point font.
	var decoded interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &decoded); err != nil {
		return 0, false
	}
	f, ok := decoded.(float64)
	if !ok {
		return 0, false
	}
	return f, true
}

func normalizeCellStyleAliases(style map[string]interface{}, path string) error {
	if len(style) == 0 {
		return nil
	}
	for _, a := range cellStyleAliases {
		v, ok := style[a.alias]
		if !ok {
			continue
		}
		if _, exists := style[a.canonical]; exists {
			return common.ValidationErrorf("%s.%s conflicts with %s; pass only %s", path, a.alias, a.canonical, a.canonical)
		}
		style[a.canonical] = v
		delete(style, a.alias)
	}
	for _, a := range cellStyleValueAliases {
		raw, ok := style[a.alias]
		if !ok {
			continue
		}
		word, readable := a.value(raw)
		if !readable {
			// An unreadable value is a guess about the vocabulary, not a
			// spelling of it: leave the key for the prescription.
			continue
		}
		if _, exists := style[a.canonical]; exists {
			return common.ValidationErrorf("%s.%s conflicts with %s; pass only %s", path, a.alias, a.canonical, a.canonical)
		}
		style[a.canonical] = word
		delete(style, a.alias)
	}
	// fore_color is deliberately NOT aliased: in openpyxl vocabulary fgColor
	// is the FILL color while a plain reading suggests the font color — a
	// silent pick could color the wrong thing. Prescribe both options.
	if _, has := style["fore_color"]; has {
		return common.ValidationErrorf("%s.fore_color is ambiguous — use font_color for text color or background_color for the cell fill", path)
	}
	// Boolean wrap habit: true unambiguously means wrap on, false means off.
	if b, isBool := style["word_wrap"].(bool); isBool {
		if b {
			style["word_wrap"] = "auto-wrap"
		} else {
			style["word_wrap"] = "overflow"
		}
	}
	// Scalar style fields carry a declared type in flag-defs. --styles and the
	// typed --cells payloads bypass the generic JSON-schema pass (their schema
	// describes the outer envelope, not each cell_styles object), so assert the
	// declared type here — otherwise {"font_weight": true} sails through
	// normalization and reaches the server as a boolean.
	// Both loops below can abort with an error, so they walk their vocabulary in
	// sorted order: map iteration would let the same bad payload report a
	// different field on every run.
	scalarTypes := cellStyleScalarTypes()
	for _, field := range sortedKeys(scalarTypes) {
		want := scalarTypes[field]
		raw, has := style[field]
		if !has || raw == nil {
			continue
		}
		if got := jsType(raw); got != want {
			// A quoted number under a numeric style field ("font_size":"16")
			// is the one type mismatch with a single reading — the digits are
			// right and only the quotes are wrong. 08-29..31 reflow: 30
			// --styles rejections, all font_size. Every other mismatch
			// (a boolean font_weight, a numeric background_color) still
			// fails: those are guesses about the vocabulary, not typing slips.
			if want == "number" {
				if n, ok := numericStyleValue(raw); ok {
					style[field] = n
					continue
				}
			}
			return common.ValidationErrorf("%s.%s must be a %s, got %s (%s)",
				path, field, want, got, formatJSONValue(raw))
		}
	}
	enumFields := cellStyleEnumFields()
	for _, field := range sortedKeys(enumFields) {
		enum := enumFields[field]
		raw, has := style[field]
		if !has {
			continue
		}
		val, isStr := raw.(string)
		if !isStr || val == "" || slices.Contains(enum, val) {
			continue
		}
		if canon := canonicalEnumValue(val, enum); canon != "" {
			style[field] = canon
			continue
		}
		msg := fmt.Sprintf("%s.%s value %q is invalid (allowed: %s)", path, field, val, strings.Join(enum, ", "))
		if match := closestEnumValue(val, enum); match != "" {
			msg += fmt.Sprintf("; did you mean %q?", match)
		}
		return common.ValidationErrorf("%s", msg)
	}
	return nil
}

// normalizeTypedCellsStyleAliases walks a typed --cells 2D array and applies
// normalizeCellStyleAliases to every cell's inline cell_styles object, so the
// alignment shorthands are accepted on +cells-set the same as on --styles.
// It also expands the border "all" shorthand and intercepts border_styles
// mis-nested inside cell_styles — both server-rejected shapes that eval
// traces show surviving CLI validation and costing a full network round
// trip. Structure is checked leniently to match the pass-through contract:
// any element that isn't the expected shape is skipped, not rejected.
func normalizeTypedCellsStyleAliases(cells []interface{}, path string) error {
	for r, rowRaw := range cells {
		row, ok := rowRaw.([]interface{})
		if !ok {
			continue
		}
		for c, cellRaw := range row {
			cell, ok := cellRaw.(map[string]interface{})
			if !ok {
				continue
			}
			// cells[][].style is the habitual spelling of cell_styles (recurring
			// server-side 900015206 in eval traces) — rewrite when unambiguous.
			if styleObj, isObj := cell["style"].(map[string]interface{}); isObj {
				if _, has := cell["cell_styles"]; has {
					return common.ValidationErrorf("%s[%d][%d].style conflicts with cell_styles; pass only cell_styles", path, r, c)
				}
				cell["cell_styles"] = styleObj
				delete(cell, "style")
			}
			// cells[][].type is not a cell field; the value type is whatever the
			// JSON value is. Reject with the fix instead of a server round trip.
			if _, has := cell["type"]; has {
				return common.ValidationErrorf("%s[%d][%d].type is not a cell field — the value type is inferred from the JSON value; control display format via cell_styles.number_format", path, r, c)
			}
			if bs, ok := cell["border_styles"].(map[string]interface{}); ok {
				expandBorderAllShorthand(bs)
			}
			st, ok := cell["cell_styles"].(map[string]interface{})
			if !ok {
				continue
			}
			if _, misNested := st["border_styles"]; misNested {
				return common.ValidationErrorf(
					"%s[%d][%d].cell_styles.border_styles is not valid — border_styles is a top-level cell field, a sibling of cell_styles; move it up one level",
					path, r, c)
			}
			if err := normalizeCellStyleAliases(st, fmt.Sprintf("%s[%d][%d].cell_styles", path, r, c)); err != nil {
				return err
			}
		}
	}
	return nil
}

// expandBorderAllShorthand rewrites the "all" side shorthand — habitual from
// Excel / openpyxl vocabulary, rejected by the backend — into the four
// explicit sides, in place. An explicitly set side wins over the shorthand.
// Applied on both the typed --cells path and the --styles path, so batch
// sub-ops get the same rewrite as standalone calls. Every side it leaves
// behind then goes through normalizeBorderSideVocab, which makes this the
// one funnel where border VALUES get canonicalized as well.
//
// "outer" is the same shorthand under the Lark OpenAPI's own name
// (OUTER_BORDER): per-side specs address the RANGE's edges, so its four sides
// are exactly the outer box. Its counterpart INNER_BORDER has no expression
// here at all and keeps the border prescription instead of a wrong guess.
func expandBorderAllShorthand(border map[string]interface{}) {
	if outer, ok := border["outer"]; ok {
		all, hasAll := border["all"]
		switch {
		case !hasAll:
			border["all"] = outer
			delete(border, "outer")
		case reflect.DeepEqual(all, outer):
			// A duplicate spelling of the same box; dropping it loses nothing.
			delete(border, "outer")
		default:
			// Two different boxes under two names for the same thing. Picking
			// one would silently apply half the caller's intent, so "outer"
			// stays and the invalid-side check downstream names the collision.
		}
	}
	if all, ok := border["all"]; ok {
		for _, side := range []string{"top", "bottom", "left", "right"} {
			if _, exists := border[side]; !exists {
				border[side] = all
			}
		}
		delete(border, "all")
	}
	for _, raw := range border {
		if side, ok := raw.(map[string]interface{}); ok {
			normalizeBorderSideVocab(side)
		}
	}
}

// borderWeightWord folds a thickness word onto the weight enum, returning
// "" for anything that is not one. The wire contract splits a border into
// style (line type) x weight (thickness) while openpyxl packs both into one
// — Side(border_style="thin") — so these words show up in BOTH slots and
// this one helper serves both.
//
// The 08-11 tally (596 traces) says they are the whole border-value problem:
// "thin" in the style slot 1795 hits / 39 tasks, "hair" in weight 476 / 19,
// "medium" in style 78 / 7. Everything else scored ZERO — openpyxl's other
// line styles (dashDot, mediumDashed) and other libraries' spellings
// (xlContinuous, CSS hidden, SOLID_THICK) stay rejected with the enum in the
// message, per the admission bar at the top of this file.
func borderWeightWord(s string) string {
	switch lower := strings.ToLower(s); lower {
	case "thin", "medium", "thick":
		return lower
	case "hair":
		// openpyxl's hairline; the contract has no grade thinner than thin
		return "thin"
	}
	return ""
}

// normalizeBorderSideVocab canonicalizes ONE border side spec in place:
// a thickness word in the style slot moves to weight (a "thin border" is a
// thin SOLID line), the weight slot folds casing and hair, and a numeric
// weight reads as a line width. Unknown values are left for the enum error.
func normalizeBorderSideVocab(side map[string]interface{}) {
	// "width" is the Google Sheets API name for weight (35 hits / 3 tasks).
	// Only when weight is free — a side carrying both is contradictory
	// input, left intact for the validator.
	if w, aliased := side["width"]; aliased {
		if _, taken := side["weight"]; !taken {
			side["weight"] = w
			delete(side, "width")
		}
	}
	// "type" is the line-kind slot in the Lark OpenAPI's own border vocabulary
	// (border_type: FULL_BORDER / …) and in openpyxl's Side(border_style=…)
	// read loosely; inside a per-side spec whose only kind slot is `style`,
	// it can mean nothing else. 08-29..31 reflow: 29 rejections across
	// +styles-put and +workbook-create answered "type is not a border
	// attribute", a message that names the vocabulary but not the mapping.
	// The value then goes through the style/weight sorting below, so
	// {"type":"thin"} lands as a thin solid line exactly like {"style":"thin"}.
	if t, aliased := side["type"]; aliased {
		if _, taken := side["style"]; !taken {
			side["style"] = t
			delete(side, "type")
		}
	}
	// A number is a line width in px/pt (xlsxwriter's set_border(1) and the
	// Google Sheets API agree at 1 and 2). 0 and negatives are not guessed
	// at: "no width" is a border the caller should spell style:"none".
	if n, isWidth := borderLineWidth(side["weight"]); isWidth {
		switch {
		case n >= 3:
			side["weight"] = "thick"
		case n >= 2:
			side["weight"] = "medium"
		case n > 0:
			side["weight"] = "thin"
		}
	}
	if w, isStr := side["weight"].(string); isStr {
		if canon := borderWeightWord(w); canon != "" {
			side["weight"] = canon
		}
	}
	// The style slot: move the thickness word over and default the line
	// type to solid. An explicit weight that contradicts the word keeps the
	// enum error path rather than picking a winner.
	if s, isStr := side["style"].(string); isStr {
		if canon := borderWeightWord(s); canon != "" {
			w, has := side["weight"]
			if !has {
				side["weight"] = canon
			}
			if !has || w == canon {
				side["style"] = "solid"
			}
		}
	}
}

// borderLineWidth reads a weight that arrived as a line width instead of a
// word — JSON's float64, or the digits-in-a-string form ("1") that models
// emit just as often. Deliberately not tableGetToFloat: that one answers
// "is this cell a number?" for column typing and must NOT accept a quoted
// number, whereas in the weight slot a quoted number is unambiguously a
// width.
//
// A non-finite result is NOT a width: ParseFloat accepts "Inf" / "Infinity" /
// "NaN", and an infinite line width folded onto "thick" would be a guess at
// input that means nothing. Those stay on the enum error path with the three
// accepted words in the message.
func borderLineWidth(v interface{}) (float64, bool) {
	var n float64
	switch t := v.(type) {
	case float64:
		n = t
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	if math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, false
	}
	return n, true
}

// normalizeBorderStylesFlagValue runs the border vocabulary rewrites on the
// parsed --border-styles value BEFORE schema validation (jsonFlagNormalizers
// seam in parseJSONFlag). Without it the enum check fires first and rejects
// the weight-word-in-style habit ({"style":"thin"}) that
// expandBorderAllShorthand exists to absorb — the acceptance layer was
// unreachable on this path (07-28 root-cause report #2, 173 occurrences).
// Non-object shapes pass through for the validator to prescribe.
func normalizeBorderStylesFlagValue(_ flagView, v interface{}) interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		expandBorderAllShorthand(m)
	}
	return v
}

// normalizeCellsFlagValue is the +cells-set --cells pre-validation pipeline:
// strip a {"cells": …} envelope, wrap a lone cell object into [[cell]], lift
// bare scalars in cell slots into {"value": …}, then run the border
// vocabulary rewrites on each cell's border_styles so weight words in the
// style slot normalize before the enum check — same reachability fix as
// normalizeBorderStylesFlagValue, for the typed-cells carrier (07-28
// root-cause report #10, 58 occurrences). Each helper documents why its own
// rewrite is unambiguous. Structure is checked leniently: anything that isn't
// the expected shape is left for the validator.
func normalizeCellsFlagValue(runtime flagView, v interface{}) interface{} {
	v = wrapLoneCellObject(unwrapCellsEnvelope(v))
	// A lone scalar is the one cell it names. The 2D shape is what the flag
	// documents, but "write this here" with a single value and a single
	// anchor has no second reading (09-04..07: 400 rejections whose payload
	// was a bare string or number).
	if lifted := scalarCellValue(v); lifted != nil {
		return []interface{}{[]interface{}{lifted}}
	}
	rows, ok := v.([]interface{})
	if !ok {
		return v
	}
	rows = liftFlatCellsRow(rows, strings.TrimSpace(runtime.Str("range")))
	v = rows
	for _, rowRaw := range rows {
		row, ok := rowRaw.([]interface{})
		if !ok {
			continue
		}
		for i, cellRaw := range row {
			cell, ok := cellRaw.(map[string]interface{})
			if !ok {
				if lifted := scalarCellValue(cellRaw); lifted != nil {
					row[i] = lifted
				}
				continue
			}
			foldCellLevelStyleVocabulary(cell)
			if bs, ok := cell["border_styles"].(map[string]interface{}); ok {
				expandBorderAllShorthand(bs)
			}
		}
	}
	padRaggedCellRows(rows)
	return v
}

// padRaggedCellRows squares off a payload whose rows stop at their last
// meaningful cell, padding the short ones with {} — the empty cell object
// that writes nothing and leaves what is there alone. The rewrite is the one
// the rejection already prescribed word for word ("pad short rows with {} to
// keep those cells unchanged"), so performing it changes no outcome except
// the round trip: a payload the caller has to retype byte for byte is a
// rejection that carries its own answer. 09-04..07: 3063 rejections on
// +cells-set alone. Same reading as the short-row fill --sheets takes.
//
// A payload whose rows are uniformly short is already a rectangle and stays
// untouched; the range it does or does not fill is fitCellsRange's question,
// not this one.
func padRaggedCellRows(rows []interface{}) {
	width := 0
	for _, rowRaw := range rows {
		row, isArray := rowRaw.([]interface{})
		if !isArray {
			return // not the 2D shape at all — leave the validator its error
		}
		width = max(width, len(row))
	}
	if width == 0 {
		return
	}
	for i, rowRaw := range rows {
		row, _ := rowRaw.([]interface{})
		for len(row) < width {
			row = append(row, map[string]interface{}{})
		}
		rows[i] = row
	}
}

// cellCarrierFields are the keys a cell object may hold on the wire: one
// content field plus the stackable carriers. Anything else that is style
// vocabulary belongs inside cell_styles (scalars) or border_styles (the
// border family), which is what foldCellLevelStyleVocabulary arranges.
var cellCarrierFields = map[string]bool{
	"value": true, "formula": true, "rich_text": true, "multiple_values": true,
	"cell_styles": true, "border_styles": true, "note": true, "data_validation": true,
}

// foldCellLevelStyleVocabulary moves style fields written directly on a cell
// into the carrier that holds them. A cell spelled
// {"value":"x","font_weight":"bold","border":{…}} passed every client check
// and reached the backend verbatim, which answered
// `[cells[0][0].border] unexpected property "border" is not defined` -- a
// message naming neither the carrier nor the fix. 08-29..31 reflow: 33
// +cells-set rejections came back from the server that way, on a payload the
// --styles path would have accepted, which is exactly the vocabulary-parity
// break this file's contract forbids.
//
// Only keys this domain already knows are moved. An unrecognized key is left
// where it is: it may be a field the tool contract gained since this build,
// and guessing at it is what the contract forbids.
func foldCellLevelStyleVocabulary(cell map[string]interface{}) {
	// The border family folds into border_styles, which IS a cell-level
	// carrier -- the same rewrite the --styles path performs. A conflict
	// (both spellings present) is left alone for the validator to report.
	if hasBorderFamilyKey(cell) {
		if err := foldBorderFamilyAliases(cell, "--cells"); err != nil {
			return
		}
	}
	scalars := cellStyleScalarTypes()
	for _, field := range sortedKeys(cell) {
		if cellCarrierFields[field] {
			continue
		}
		canonical := field
		if _, known := scalars[canonical]; !known {
			// Try the alias table's spellings (wrap_text, halign, …) so a
			// habitual name written at cell level lands the same way it does
			// inside cell_styles.
			for _, a := range cellStyleAliases {
				if a.alias == field {
					canonical = a.canonical
					break
				}
			}
		}
		if _, known := scalars[canonical]; !known {
			continue
		}
		styles, ok := cell["cell_styles"].(map[string]interface{})
		if !ok {
			if _, exists := cell["cell_styles"]; exists {
				return // a non-object cell_styles is the validator's to report
			}
			styles = map[string]interface{}{}
			cell["cell_styles"] = styles
		}
		if _, taken := styles[canonical]; taken {
			continue // cell_styles wins; the duplicate is reported downstream
		}
		styles[canonical] = cell[field]
		delete(cell, field)
	}
	// Canonicalize the values too, on the same pass. The typed --cells carrier
	// meets the generic JSON-schema check right after this normalizer, while
	// the --styles carrier skips it and reaches normalizeCellStyleAliases with
	// its rewrites intact -- so without this, a boolean word_wrap that
	// --styles accepts died here on "expected type string". The error is
	// dropped on purpose: this is the rewrite pass, and the same function runs
	// again later with the path context that makes a good message.
	if styles, ok := cell["cell_styles"].(map[string]interface{}); ok {
		_ = normalizeCellStyleAliases(styles, "--cells")
	}
}

// hasBorderFamilyKey reports whether a cell carries any border spelling other
// than the canonical border_styles carrier, i.e. whether the fold has work.
func hasBorderFamilyKey(cell map[string]interface{}) bool {
	for field := range cell {
		if field == "border_styles" {
			continue
		}
		if strings.Contains(strings.ToLower(field), "border") {
			return true
		}
	}
	return false
}

// unwrapWritesEnvelope turns the two object spellings of --writes into the
// array the flag takes: the {"writes":[…]} wrapper (the flag name repeated
// inside its own value, the same habit that produces a bare {"cells":…}), and
// a lone write object written without its list. 08-29..31 reflow: 54
// +cells-set rejections read `--writes: expected type "array", got "object"`,
// the largest diagnosed cluster on that command.
//
// A lone write is recognized by carrying `cells` or `values` -- the payload a
// write must have. An object with neither is left alone: it is not a write,
// and the type error is the right answer for it.
func unwrapWritesEnvelope(v interface{}) interface{} {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	if inner, wrapped := obj["writes"]; wrapped && len(obj) == 1 {
		if list, isList := inner.([]interface{}); isList {
			return list
		}
		if single, isObj := inner.(map[string]interface{}); isObj {
			return []interface{}{single}
		}
		return v
	}
	_, hasCells := obj["cells"]
	_, hasValues := obj["values"]
	if hasCells || hasValues {
		return []interface{}{obj}
	}
	return v
}

// normalizeWritesFlagValue runs the --cells rewrites on every --writes item
// before the writes array meets its schema. cellsSetWritesOps already gives
// each item the standalone pipeline through a per-item flag view, but that
// runs after requireJSONArray has validated the array, so an item spelling
// its payload "values" or wrapping it in a {"cells": …} envelope died on the
// array schema ("required property \"cells\" is missing") while the identical
// +batch-update sub-op was accepted. Same rewrites, one step earlier, so the
// two forms of the same write agree.
//
// values → cells only when "cells" is absent: two spellings carrying
// different payloads is a conflict for normalizeSubOpInputKeys to report, not
// one to silently resolve here.
func normalizeWritesFlagValue(runtime flagView, v interface{}) interface{} {
	v = unwrapWritesEnvelope(v)
	items, ok := v.([]interface{})
	if !ok {
		return v
	}
	for _, itemRaw := range items {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if _, taken := item["cells"]; !taken {
			if values, ok := item["values"]; ok {
				item["cells"] = values
				delete(item, "values")
			}
		}
		if cells, ok := item["cells"]; ok {
			item["cells"] = normalizeCellsFlagValue(newMapFlagViewForCommand("+cells-set", item), cells)
		}
	}
	return v
}

// borderStylesFromFlag parses --border-styles as a JSON object (top/bottom/
// left/right with style sub-objects), expanding the "all" side shorthand the
// same as the typed --cells and --styles paths so +cells-set-style /
// +cells-batch-set-style don't ship {"all":…} for the backend to reject.
// The expansion normally already ran inside parseJSONFlag (see
// normalizeBorderStylesFlagValue); the call here is an idempotent safety net
// for entry paths that bypass the normalizer table.
// Returns nil when the flag is empty.
func borderStylesFromFlag(runtime flagView) (map[string]interface{}, error) {
	if runtime.Str("border-styles") == "" {
		return nil, nil
	}
	v, err := parseJSONFlag(runtime, "border-styles")
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, sheetsValidationForFlag("border-styles", "--border-styles must be a JSON object")
	}
	expandBorderAllShorthand(m)
	return m, nil
}

// requireAnyStyleFlag ensures at least one style-defining flag (style or
// border) is set — otherwise the request would do nothing.
func requireAnyStyleFlag(runtime flagView) error {
	if len(buildCellStyleFromFlags(runtime)) > 0 {
		return nil
	}
	if runtime.Str("border-styles") != "" {
		return nil
	}
	return common.ValidationErrorf("at least one style flag is required (e.g. --background-color, --font-weight, --border-styles)").
		WithParams(
			sheetsInvalidParam("background-color", "required; specify at least one style flag"),
			sheetsInvalidParam("font-weight", "required; specify at least one style flag"),
			sheetsInvalidParam("border-styles", "required; specify at least one style flag"),
		)
}

// foldBorderFamilyAliases rewrites the habitual flattened border vocabulary
// (Excel / openpyxl conventions) into the canonical nested border_styles
// object, in place. 07-20 eval: the border family alone accounted for the
// largest --styles error cluster (borders / border / border_bottom /
// border_style / border_top_color / …), each burning a full payload retry.
// Accepted rewrites, all unambiguous:
//
//	borders / border   (object) → border_styles (side-keyed) or border_styles.all (attr-keyed)
//	border_top|bottom|left|right (object) → border_styles.<side>
//	border_style|color|weight  (scalar)  → border_styles.all.<attr>
//	border_<side>_<style|color|weight> (scalar) → border_styles.<side>.<attr>
//
// A border_style value from the WEIGHT vocabulary (thin/medium/thick — the
// habitual Excel word) sets weight and defaults style to solid: a "thin
// border" always means a thin solid line. Conflicts with an explicitly given
// border_styles error out instead of picking a side.
// Every walk over a set here goes through an ORDERED slice, never a map range:
// each branch below can abort with an error, so map iteration order would
// decide which of several bad fields gets reported and the same payload would
// produce different messages run to run (same reason parseWorkbookCreateFreezeOp
// sorts its keys).
func foldBorderFamilyAliases(in map[string]interface{}, path string) error {
	attrNames := []string{"color", "style", "weight"}
	sides := map[string]bool{"top": true, "bottom": true, "left": true, "right": true, "all": true}
	attrs := map[string]bool{"style": true, "color": true, "weight": true}

	ensureBorder := func() map[string]interface{} {
		bs, ok := in["border_styles"].(map[string]interface{})
		if !ok {
			bs = map[string]interface{}{}
			in["border_styles"] = bs
		}
		return bs
	}
	setSideAttr := func(side, attr string, v interface{}, from string) error {
		bs := ensureBorder()
		sideObj, ok := bs[side].(map[string]interface{})
		if !ok {
			if _, exists := bs[side]; exists {
				return common.ValidationErrorf("%s.%s conflicts with border_styles.%s; keep one form", path, from, side)
			}
			sideObj = map[string]interface{}{}
			bs[side] = sideObj
		}
		if _, exists := sideObj[attr]; exists {
			return common.ValidationErrorf("%s.%s conflicts with border_styles.%s.%s; keep one form", path, from, side, attr)
		}
		sideObj[attr] = v
		return nil
	}
	setSide := func(side string, v interface{}, from string) error {
		obj, ok := v.(map[string]interface{})
		if !ok {
			return common.ValidationErrorf("%s.%s must be an object like {\"style\":\"solid\",\"color\":\"#000000\"}", path, from)
		}
		// The line-kind slot is spelled `type` in the Lark OpenAPI's own
		// border vocabulary and in openpyxl's Side(border_style=…) read
		// loosely. This spec has exactly one kind slot, so the rename is the
		// only reading; normalizeBorderSideVocab then sorts a thickness word
		// out of the style slot as it does for any other spelling.
		normalizeBorderSideVocab(obj)
		for _, attr := range sortedKeys(obj) {
			if !attrs[attr] {
				return common.ValidationErrorf("%s.%s.%s is not a border attribute (want style/weight/color)", path, from, attr)
			}
			if err := setSideAttr(side, attr, obj[attr], from); err != nil {
				return err
			}
		}
		return nil
	}
	// setSideScalar reads a border written as ONE value rather than an
	// attribute object — border:"solid", border_all:"thin", border:2. Every
	// external border vocabulary has this form (xlsxwriter's {'border': 1},
	// CSS's shorthand, openpyxl's Side(border_style="thin")) and it has one
	// reading: the value names the line, the rest of the spec defaults.
	// A thickness word or a pixel count fills weight and leaves style solid,
	// exactly as the flattened border_style path does; anything else is a
	// line style and reaches the style enum, which names the allowed set when
	// the word came from a vocabulary this contract does not carry.
	// 09-04..07: 5903 rejections read "border must be an object", all of them
	// a caller who wrote the value the shorthand takes everywhere else.
	setSideScalar := func(side string, v interface{}, from string) error {
		if s, ok := v.(string); ok {
			if canon := borderWeightWord(s); canon != "" {
				if err := setSideAttr(side, "weight", canon, from); err != nil {
					return err
				}
				return setSideAttr(side, "style", "solid", from)
			}
			return setSideAttr(side, "style", v, from)
		}
		if _, isWidth := borderLineWidth(v); isWidth {
			// normalizeBorderSideVocab turns the number into a weight word
			// once the side object exists; setting it here keeps that one
			// conversion table in one place.
			if err := setSideAttr(side, "weight", v, from); err != nil {
				return err
			}
			return setSideAttr(side, "style", "solid", from)
		}
		return common.ValidationErrorf("%s.%s must be an object like {\"style\":\"solid\",\"color\":\"#000000\"}, a line style (\"solid\"), or a thickness (\"thin\", 2)", path, from)
	}
	// setSideLoose takes whichever of the two forms the caller used.
	setSideLoose := func(side string, v interface{}, from string) error {
		if _, isObj := v.(map[string]interface{}); isObj {
			return setSide(side, v, from)
		}
		return setSideScalar(side, v, from)
	}
	// A flattened border_style holding a thickness word fills both attributes
	// — same helper as the nested form (borderWeightWord), but split out here
	// so a contradicting border_styles still reports the conflict rather than
	// falling through to an enum error.
	setAllScalar := func(attr string, v interface{}, from string) error {
		if attr == "style" {
			if s, ok := v.(string); ok {
				if canon := borderWeightWord(s); canon != "" {
					if err := setSideAttr("all", "weight", canon, from); err != nil {
						return err
					}
					return setSideAttr("all", "style", "solid", from)
				}
			}
		}
		return setSideAttr("all", attr, v, from)
	}

	for _, key := range []string{"borders", "border"} {
		v, has := in[key]
		if !has {
			continue
		}
		obj, ok := v.(map[string]interface{})
		if !ok {
			// The one-value shorthand, not a malformed object.
			if err := setSideScalar("all", v, key); err != nil {
				return err
			}
			delete(in, key)
			continue
		}
		sideKeyed := false
		for k := range obj {
			if sides[k] {
				sideKeyed = true
				break
			}
		}
		if sideKeyed {
			for _, side := range sortedKeys(obj) {
				if !sides[side] {
					return common.ValidationErrorf("%s.%s.%s is not a valid side (want top/bottom/left/right/all)", path, key, side)
				}
				if err := setSideLoose(side, obj[side], key); err != nil {
					return err
				}
			}
		} else if err := setSide("all", v, key); err != nil {
			return err
		}
		delete(in, key)
	}
	// "all" rides in the same loop as the four sides: border_all is the same
	// flattened habit one key wider, and the side vocabulary already carries
	// it (09-04..07: 242 rejections).
	for _, side := range []string{"top", "bottom", "left", "right", "all"} {
		// Both word orders appear in the wild: border_bottom (07-20 eval) and
		// bottom_border (07-21), same for the flattened attribute triples.
		for _, key := range []string{"border_" + side, side + "_border"} {
			if v, has := in[key]; has {
				if err := setSideLoose(side, v, key); err != nil {
					return err
				}
				delete(in, key)
			}
		}
		for _, attr := range attrNames {
			for _, key := range []string{"border_" + side + "_" + attr, side + "_border_" + attr} {
				if v, has := in[key]; has {
					if err := setSideAttr(side, attr, v, key); err != nil {
						return err
					}
					delete(in, key)
				}
			}
		}
		for spelling, attr := range borderAttrAliases {
			for _, key := range []string{"border_" + side + "_" + spelling, side + "_border_" + spelling} {
				if v, has := in[key]; has {
					if err := setSideAttr(side, attr, v, key); err != nil {
						return err
					}
					delete(in, key)
				}
			}
		}
	}
	for _, attr := range attrNames {
		key := "border_" + attr
		if v, has := in[key]; has {
			if err := setAllScalar(attr, v, key); err != nil {
				return err
			}
			delete(in, key)
		}
	}
	for _, spelling := range sortedKeys(borderAttrAliases) {
		key := "border_" + spelling
		if v, has := in[key]; has {
			if err := setAllScalar(borderAttrAliases[spelling], v, key); err != nil {
				return err
			}
			delete(in, key)
		}
	}
	// border_type runs last so an explicitly spelled attribute wins: the
	// values that only pick SIDES say nothing about the line, and a caller
	// who wrote border_type:"FULL_BORDER" alongside border_style:"dashed"
	// means a dashed box, not a conflict.
	if v, has := in["border_type"]; has {
		if err := foldBorderTypeValue(v, path, setSideAttr, setSideLoose, func(side, attr string, val interface{}) {
			bs, ok := in["border_styles"].(map[string]interface{})
			if !ok {
				bs = map[string]interface{}{}
				in["border_styles"] = bs
			}
			sideObj, ok := bs[side].(map[string]interface{})
			if !ok {
				if _, exists := bs[side]; exists {
					return
				}
				sideObj = map[string]interface{}{}
				bs[side] = sideObj
			}
			if _, exists := sideObj[attr]; !exists {
				sideObj[attr] = val
			}
		}); err != nil {
			return err
		}
		delete(in, "border_type")
	}
	return nil
}

// borderAttrAliases carries the border attribute names other vocabularies use
// for a slot this contract spells differently. width is the Google Sheets API
// and CSS name for weight; normalizeBorderSideVocab already folds it inside a
// per-side object, and these entries close the flattened spellings
// (border_width, border_top_width) that never reach a side object at all.
// 09-04..07: 919 rejections on border.width and border_width.
var borderAttrAliases = map[string]string{"width": "weight"}

// borderTypeSideValues maps the Lark OpenAPI's own borderType vocabulary onto
// the sides a per-cell style spec addresses. set_cell_range applies each side
// to EVERY cell of the range (verified 09-10: border_styles.top over A1:C3
// leaves a top border on all nine cells, not on row 1 only), so FULL_BORDER
// and the single-side words are exact — "all" is precisely FULL_BORDER, and
// the earlier reading of these values as inexpressible was the mapping being
// read the wrong way round.
var borderTypeSideValues = map[string]string{
	"FULL_BORDER": "all", "ALL_BORDER": "all", "ALL_BORDERS": "all", "ALL": "all", "GRID": "all",
	"TOP_BORDER": "top", "BOTTOM_BORDER": "bottom", "LEFT_BORDER": "left", "RIGHT_BORDER": "right",
}

// borderTypeInteriorValues are the borderType words that address a subset of
// a range's edges — the interior lines, or the outline alone. A per-cell spec
// styles every cell the same way, so neither has an expression here and a
// silent nearest-fit would draw lines the caller did not ask for.
var borderTypeInteriorValues = map[string]string{
	"OUTER_BORDER":      `border_type:"OUTER_BORDER" outlines the range while leaving the cells inside it bare, which a per-cell style spec cannot express: style the four edge ranges instead ({"border_styles":{"top":{"style":"solid"}}} on the first row, "bottom" on the last, "left" on the first column, "right" on the last)`,
	"INNER_BORDER":      `border_type:"INNER_BORDER" draws only the lines between cells, which a per-cell style spec cannot express; border every cell with border_type:"FULL_BORDER"`,
	"HORIZONTAL_BORDER": `border_type:"HORIZONTAL_BORDER" draws only the lines between rows, which a per-cell style spec cannot express; border every cell with border_type:"FULL_BORDER", or style one row's range with {"border_styles":{"bottom":{"style":"solid"}}}`,
	"VERTICAL_BORDER":   `border_type:"VERTICAL_BORDER" draws only the lines between columns, which a per-cell style spec cannot express; border every cell with border_type:"FULL_BORDER", or style one column's range with {"border_styles":{"right":{"style":"solid"}}}`,
}

// foldBorderTypeValue folds border_type onto the border_styles it names,
// dispatching on the VALUE because the field carries two vocabularies at
// once: the Lark OpenAPI's side selector (FULL_BORDER / TOP_BORDER / …) and
// the CSS-ish line word every model reaches for (solid, dashed, thin, 2).
// Each value belongs to exactly one of them, so the pair is decidable even
// though the field name alone is not — which is what kept border_type
// rejected until now, at 46035 rejections over 09-04..07, the single largest
// style-field cluster.
//
// soft fills a side attribute only when nothing has claimed it, for the
// side-selector branch: those values state that a border exists, not what it
// looks like.
func foldBorderTypeValue(v interface{}, path string,
	setSideAttr func(side, attr string, v interface{}, from string) error,
	setSideLoose func(side string, v interface{}, from string) error,
	soft func(side, attr string, val interface{}),
) error {
	word, isStr := v.(string)
	if !isStr {
		return setSideLoose("all", v, "border_type")
	}
	key := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(word), "-", "_"))
	key = strings.ReplaceAll(key, " ", "_")
	if side, ok := borderTypeSideValues[key]; ok {
		soft(side, "style", "solid")
		return nil
	}
	if key == "NO_BORDER" || key == "NONE" {
		return setSideAttr("all", "style", "none", "border_type")
	}
	if rx, ok := borderTypeInteriorValues[key]; ok {
		return common.ValidationErrorf("%s.border_type: %s", path, rx)
	}
	return setSideLoose("all", v, "border_type")
}
