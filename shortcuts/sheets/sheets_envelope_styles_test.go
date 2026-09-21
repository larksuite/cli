// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// A caller who holds one document puts the styles in it next to the sheets.
// The sheets parser decodes into a struct that declares only `sheets`, so that
// section used to vanish without a word: ok:true, data written, formatting
// silently absent. These pin that it is read, and that a payload naming two
// different style lists is refused rather than resolved by guess.

const envelopeWithStyles = `{"sheets":[{"name":"S1","columns":["c"],"data":[["x"]]}],` +
	`"styles":[{"name":"S1","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`

func TestSheetsEnvelopeStyles_AreAppliedWithoutTheStylesFlag(t *testing.T) {
	t.Parallel()
	out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"),
		[]string{"--title", "T", "--sheets", envelopeWithStyles, "--dry-run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "cell_styles") {
		t.Errorf("the styles section next to --sheets.sheets was dropped:\n%s", out)
	}
}

func TestSheetsEnvelopeStyles_AgreeingFlagIsAccepted(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"),
		[]string{"--title", "T", "--sheets", envelopeWithStyles,
			"--styles", `[{"name":"S1","cell_styles":[{"range":"A1","font_weight":"bold"}]}]`, "--dry-run"})
	if err != nil {
		t.Fatalf("the redundant spelling of the same styles was refused: %v", err)
	}
}

func TestSheetsEnvelopeStyles_DisagreeingFlagIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"),
		[]string{"--title", "T", "--sheets", envelopeWithStyles,
			"--styles", `[{"name":"S1","cell_styles":[{"range":"A1","font_weight":"normal"}]}]`, "--dry-run"})
	if err == nil {
		t.Fatal("two different style lists in one call were silently resolved")
	}
	if !strings.Contains(err.Error(), "different styles") {
		t.Errorf("error does not name the disagreement: %v", err)
	}
}

// The caller never typed --styles, so no message about this payload may say
// they did.
func TestSheetsEnvelopeStyles_ProblemsPointAtTheSheetsFlag(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"),
		[]string{"--title", "T", "--dry-run", "--sheets",
			`{"sheets":[{"name":"S1","columns":["c"],"data":[["x"]]}],` +
				`"styles":[{"name":"WRONG","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`})
	if err == nil {
		t.Fatal("a styles item naming no sub-sheet was accepted")
	}
	if !strings.Contains(err.Error(), "--sheets.styles[0]") {
		t.Errorf("message must point at --sheets.styles[0], the path the caller wrote: %v", err)
	}
	if strings.Contains(err.Error(), "--styles.styles") {
		t.Errorf("message blames a --styles the caller never typed: %v", err)
	}
}

// A bare [...] sub-sheet list has no room for a sibling; it must stay a
// plain sheets payload rather than acquire an empty styles section.
func TestSheetsEnvelopeStyles_BareListCarriesNoSection(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`[{"name":"S1"}]`,
		`{"sheets":[{"name":"S1"}]}`,
		`{"sheets":[{"name":"S1"}],"styles":null}`,
		`not json at all`,
	} {
		view := newMapFlagViewForCommand("+workbook-create", map[string]interface{}{"sheets": raw})
		if _, ok := sheetsEnvelopeStyles(view); ok {
			t.Errorf("%s reported a styles section", raw)
		}
	}
}
