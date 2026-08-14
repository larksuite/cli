// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"math"
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
// names the backend expects. Only the unambiguous alignment shorthands are
// aliased — they are the high-frequency miss; ambiguous guesses (e.g. "color",
// "bg_color", "text_align") are intentionally left out so a wrong guess still
// surfaces as an error rather than being silently reinterpreted.
var cellStyleAliases = []struct{ alias, canonical string }{
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

// styleFieldPrescriptions carries the exact fix for high-frequency
// unsupported cell_styles field names where the edit-distance suggester is
// actively misleading (07-28 root-cause report: font_bold drew "did you mean
// font_color?" and nested font drew "font_line" — an agent that follows
// either burns a second failed round trip). Keyed by lowercased field name;
// the text replaces the did-you-mean on the unsupported-field error. These
// stay prescriptions, not silent aliases: bold/text_align are on the
// deliberate no-alias list above.
var styleFieldPrescriptions = map[string]string{
	"bold":       `bold text is font_weight:"bold"`,
	"font_bold":  `bold text is font_weight:"bold"`,
	"italic":     `italic text is font_style:"italic"`,
	"underline":  `underline is font_line:"underline"`,
	"text_align": "horizontal text alignment is horizontal_alignment (left/center/right)",
	"font":       `cell_styles has no nested font object — use the flat font_* fields (font:{"bold":true,"size":18,"color":"#000"} becomes font_weight:"bold", font_size:18, font_color:"#000")`,
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
func expandBorderAllShorthand(border map[string]interface{}) {
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
func normalizeBorderStylesFlagValue(v interface{}) interface{} {
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
func normalizeCellsFlagValue(v interface{}) interface{} {
	v = wrapLoneCellObject(unwrapCellsEnvelope(v))
	rows, ok := v.([]interface{})
	if !ok {
		return v
	}
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
			if bs, ok := cell["border_styles"].(map[string]interface{}); ok {
				expandBorderAllShorthand(bs)
			}
		}
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
func normalizeWritesFlagValue(v interface{}) interface{} {
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
			item["cells"] = normalizeCellsFlagValue(cells)
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
			return common.ValidationErrorf("%s.%s must be an object — either side-keyed ({\"top\":{…},\"bottom\":{…}} / {\"all\":{…}}) or attribute-keyed ({\"style\":\"solid\",\"color\":\"#000\"} = all four sides)", path, key)
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
				if err := setSide(side, obj[side], key); err != nil {
					return err
				}
			}
		} else if err := setSide("all", v, key); err != nil {
			return err
		}
		delete(in, key)
	}
	for _, side := range []string{"top", "bottom", "left", "right"} {
		// Both word orders appear in the wild: border_bottom (07-20 eval) and
		// bottom_border (07-21), same for the flattened attribute triples.
		for _, key := range []string{"border_" + side, side + "_border"} {
			if v, has := in[key]; has {
				if err := setSide(side, v, key); err != nil {
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
	return nil
}
