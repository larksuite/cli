// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestUnknownFlagFromParseError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		name string
		ok   bool
	}{
		{"unknown flag: --cols", "cols", true},
		{"unknown flag: --with-styles", "with-styles", true},
		{"unknown shorthand flag: 'z' in -z", "", false},
		{"flag needs an argument: --find", "", false},
		{`invalid argument "x" for "--count"`, "", false},
	}
	for _, c := range cases {
		name, ok := unknownFlagFromParseError(errors.New(c.in))
		if name != c.name || ok != c.ok {
			t.Errorf("unknownFlagFromParseError(%q) = (%q,%v), want (%q,%v)", c.in, name, ok, c.name, c.ok)
		}
	}
}

// TestSheetsFlagErrorFunc_SemanticGuessListsValidFlags pins the sheets
// override of the root unknown-flag error: --cols is a semantic guess for
// --range that edit distance can't rank, so the hint must inline the full
// valid-flag list instead of deferring to a --help round trip.
func TestSheetsFlagErrorFunc_SemanticGuessListsValidFlags(t *testing.T) {
	t.Parallel()
	c := &cobra.Command{Use: "demo"}
	c.Flags().String("range", "", "")
	c.Flags().Int("width", 0, "")

	err := sheetsFlagErrorFunc(c, errors.New("unknown flag: --cols"))
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if verr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want invalid_argument", verr.Subtype)
	}
	if len(verr.Params) != 1 || verr.Params[0].Name != "--cols" {
		t.Errorf("Params = %v, want one entry named --cols", verr.Params)
	}
	if strings.Contains(verr.Hint, "--help") {
		t.Errorf("hint should not defer to --help when flags fit inline, got %q", verr.Hint)
	}
	for _, want := range []string{"--range", "--width"} {
		if !strings.Contains(verr.Hint, want) {
			t.Errorf("hint should inline valid flag %s, got %q", want, verr.Hint)
		}
	}
}

// TestSheetsFlagErrorFunc_TypoKeepsSuggestion pins that the root behavior
// (did-you-mean suggestion, machine-readable Suggestions) is preserved by
// the sheets override, with the valid-flag list appended.
func TestSheetsFlagErrorFunc_TypoKeepsSuggestion(t *testing.T) {
	t.Parallel()
	c := &cobra.Command{Use: "demo"}
	c.Flags().String("range", "", "")
	c.Flags().Bool("dry-run", false, "")

	err := sheetsFlagErrorFunc(c, errors.New("unknown flag: --rang"))
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	found := false
	for _, s := range verr.Params[0].Suggestions {
		if s == "--range" {
			found = true
		}
	}
	if !found {
		t.Errorf("Suggestions should include --range, got %v", verr.Params[0].Suggestions)
	}
	for _, want := range []string{"did you mean", "--range", "--dry-run"} {
		if !strings.Contains(verr.Hint, want) {
			t.Errorf("hint should contain %q, got %q", want, verr.Hint)
		}
	}
}

// TestSheetsFlagErrorFunc_BatchUpdateSheetLocator pins the targeted fix: a
// top-level --sheet-id / --sheet-name on +batch-update points the caller at
// the per-op locator contract instead of offering a misleading fuzzy guess.
func TestSheetsFlagErrorFunc_BatchUpdateSheetLocator(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"sheet-id", "sheet-name", "sheet_id", "sheet_name"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := &cobra.Command{Use: "+batch-update"}
			c.Flags().String("operations", "", "")
			err := sheetsFlagErrorFunc(c, errors.New("unknown flag: --"+name))
			var verr *errs.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected *errs.ValidationError, got %T", err)
			}
			if !strings.Contains(verr.Message, "put sheet_id/sheet_name inside each operation's input") {
				t.Errorf("message should name the per-op locator contract, got %q", verr.Message)
			}
			if strings.Contains(verr.Hint, "did you mean") {
				t.Errorf("must not offer a fuzzy guess here, got hint %q", verr.Hint)
			}
			if len(verr.Params) != 1 || verr.Params[0].Name != "--"+name {
				t.Errorf("Params should carry the offending flag, got %v", verr.Params)
			}
			if len(verr.Params[0].Suggestions) != 0 {
				t.Errorf("no suggestions expected, got %v", verr.Params[0].Suggestions)
			}
		})
	}
}

// TestSheetsFlagErrorFunc_BatchUpdateOtherUnknownStillSuggests confirms the
// special case is scoped to the two sheet-locator flags: any other unknown
// flag on +batch-update keeps the normal did-you-mean behaviour.
func TestSheetsFlagErrorFunc_BatchUpdateOtherUnknownStillSuggests(t *testing.T) {
	t.Parallel()
	c := &cobra.Command{Use: "+batch-update"}
	c.Flags().String("operations", "", "")
	err := sheetsFlagErrorFunc(c, errors.New("unknown flag: --operation"))
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if strings.Contains(verr.Message, "no top-level sheet locator") {
		t.Errorf("non-locator unknown flag must not hit the special case, got %q", verr.Message)
	}
}

func TestSheetsFlagErrorFunc_OtherErrorStaysGeneric(t *testing.T) {
	t.Parallel()
	c := &cobra.Command{Use: "demo"}
	err := sheetsFlagErrorFunc(c, errors.New("flag needs an argument: --find"))
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if verr.Param != "" || len(verr.Params) != 0 {
		t.Errorf("Param=%q Params=%v, want both empty for generic flag error", verr.Param, verr.Params)
	}
	if strings.Contains(verr.Hint, "did you mean") {
		t.Errorf("generic flag error must not produce a did-you-mean hint, got %q", verr.Hint)
	}
}

func TestInlineFlagList_TruncatesPastLimit(t *testing.T) {
	t.Parallel()
	if got := inlineFlagList(nil); got != "" {
		t.Errorf("inlineFlagList(nil) = %q, want empty", got)
	}
	names := make([]string, inlineFlagListLimit+5)
	for i := range names {
		names[i] = fmt.Sprintf("flag-%02d", i)
	}
	got := inlineFlagList(names)
	if !strings.Contains(got, "5 more") || !strings.Contains(got, "--help") {
		t.Errorf("truncated list should count the overflow and defer to --help, got %q", got)
	}
	if strings.Contains(got, names[inlineFlagListLimit]) {
		t.Errorf("list should stop at the limit, got %q", got)
	}
}

func TestCanonicalEnumValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		val  string
		enum []string
		want string
	}{
		{"SUM", []string{"sum", "count"}, "sum"},                  // casing
		{"center", []string{"top", "middle", "bottom"}, "middle"}, // alias: CSS vertical center
		{"middle", []string{"left", "center", "right"}, "center"}, // alias: horizontal middle
		{"overwite", []string{"append", "overwrite"}, ""},         // typo is NOT canonical
		{"delete", []string{"append", "overwrite"}, ""},           // nothing close
	}
	for _, c := range cases {
		if got := canonicalEnumValue(c.val, c.enum); got != c.want {
			t.Errorf("canonicalEnumValue(%q, %v) = %q, want %q", c.val, c.enum, got, c.want)
		}
	}
}

func TestClosestEnumValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		val  string
		enum []string
		want string
	}{
		{"SUM", []string{"sum", "count"}, "sum"},                   // casing
		{"center", []string{"top", "middle", "bottom"}, "middle"},  // alias
		{"overwite", []string{"append", "overwrite"}, "overwrite"}, // edit distance
		{"delete", []string{"append", "overwrite"}, ""},            // nothing close
	}
	for _, c := range cases {
		if got := closestEnumValue(c.val, c.enum); got != c.want {
			t.Errorf("closestEnumValue(%q, %v) = %q, want %q", c.val, c.enum, got, c.want)
		}
	}
}

// TestChainEnumNormalization_UnitContract pins the PreRunE stage in
// isolation: canonical vocabulary is auto-applied, typos error with a
// suggestion (never applied), the framework PreRunE keeps running first,
// and --print-schema skips enum gating entirely.
func TestChainEnumNormalization_UnitContract(t *testing.T) {
	t.Parallel()
	newCmd := func() (*cobra.Command, *bool) {
		cmd := &cobra.Command{Use: "+cells-set-style"}
		cmd.Flags().String("vertical-alignment", "", "")
		cmd.Flags().Bool("print-schema", false, "")
		prevCalled := false
		cmd.PreRunE = func(*cobra.Command, []string) error {
			prevCalled = true
			return nil
		}
		chainEnumNormalization(cmd)
		return cmd, &prevCalled
	}

	// Alias auto-applied, framework PreRunE preserved.
	cmd, prevCalled := newCmd()
	cmd.Flags().Set("vertical-alignment", "center")
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("center should normalize and pass, got: %v", err)
	}
	if got, _ := cmd.Flags().GetString("vertical-alignment"); got != "middle" {
		t.Errorf("vertical-alignment = %q, want rewritten to %q", got, "middle")
	}
	if !*prevCalled {
		t.Error("framework PreRunE must keep running first")
	}

	// Typo: error with suggestion, value untouched.
	cmd, _ = newCmd()
	cmd.Flags().Set("vertical-alignment", "botom")
	err := cmd.PreRunE(cmd, nil)
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("typo should fail with *errs.ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(verr.Hint, `"bottom"`) {
		t.Errorf("hint should suggest bottom for the typo, got %q", verr.Hint)
	}
	if got, _ := cmd.Flags().GetString("vertical-alignment"); got != "botom" {
		t.Errorf("typo must not be rewritten, got %q", got)
	}

	// --print-schema skips enum gating (pure local introspection).
	cmd, _ = newCmd()
	cmd.Flags().Set("vertical-alignment", "not-a-value")
	cmd.Flags().Set("print-schema", "true")
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Errorf("--print-schema must skip enum gating, got: %v", err)
	}
}

// shortcutFromRegistry returns the fully wired shortcut (PostMount
// ergonomics included) as Shortcuts() exposes it to the framework.
func shortcutFromRegistry(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, sc := range Shortcuts() {
		if sc.Command == command {
			return sc
		}
	}
	t.Fatalf("shortcut %q not found in Shortcuts()", command)
	return common.Shortcut{}
}

// TestShortcuts_FlagErgonomicsMounted verifies the ergonomics ride every
// mounted sheets command end-to-end: enum vocabulary normalizes on a real
// invocation, and unknown flags answer with the inlined valid-flag list.
func TestShortcuts_FlagErgonomicsMounted(t *testing.T) {
	t.Parallel()

	t.Run("enum alias normalizes through a real run", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cells-set-style")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1:A1",
			"--vertical-alignment", "center",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("center should normalize to middle and pass, got: %v", err)
		}
		if !strings.Contains(stdout, "middle") || strings.Contains(stdout, "center") {
			t.Errorf("dry-run body should carry the normalized value, got %q", stdout)
		}
	})

	t.Run("enum typo errors with suggestion", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cells-set-style")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1:A1",
			"--vertical-alignment", "botom",
			"--dry-run",
		})
		ve := requireValidation(t, err, `invalid value "botom" for --vertical-alignment`)
		if !strings.Contains(ve.Hint, `"bottom"`) {
			t.Errorf("hint should suggest bottom, got %q", ve.Hint)
		}
	})

	t.Run("unknown flag inlines valid flags", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cols-resize")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--col-size", "A:D",
		})
		ve := requireValidation(t, err, `unknown flag "--col-size"`)
		for _, want := range []string{"valid flags:", "--range", "--width", "--widths"} {
			if !strings.Contains(ve.Hint, want) {
				t.Errorf("hint should contain %q, got %q", want, ve.Hint)
			}
		}
	})
}

// TestShortcuts_IntuitiveFlagAliases verifies the silent-alias tier: a
// habitual name with identical value semantics parses as the real flag on a
// mounted command, costing zero round trips (eval: --cols, --file, --name,
// --source/--target each burned an unknown-flag failure plus a --help call).
func TestShortcuts_IntuitiveFlagAliases(t *testing.T) {
	t.Parallel()

	t.Run("cols-resize --cols parses as --range", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cols-resize")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--cols", "A:D",
			"--width", "100",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--cols should alias to --range and pass, got: %v", err)
		}
		if !strings.Contains(stdout, "A:D") {
			t.Errorf("dry-run body should carry the aliased range, got %q", stdout)
		}
	})

	t.Run("sheet-create --name parses as --title", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+sheet-create")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--name", "汇总",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--name should alias to --title and pass, got: %v", err)
		}
		if !strings.Contains(stdout, "汇总") {
			t.Errorf("dry-run body should carry the aliased title, got %q", stdout)
		}
	})

	t.Run("sheet-rename --new-name parses as --title", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+sheet-rename")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--new-name", "授权需求清单",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--new-name should alias to --title and pass, got: %v", err)
		}
		if !strings.Contains(stdout, "授权需求清单") {
			t.Errorf("dry-run body should carry the aliased title, got %q", stdout)
		}
	})

	t.Run("range-fill --source/--target parse as ranges", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+range-fill")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--source", "B2",
			"--target", "B3:B10",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--source/--target should alias to the -range flags, got: %v", err)
		}
		for _, want := range []string{"B2", "B3:B10"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("dry-run body should carry %q, got %q", want, stdout)
			}
		}
	})

	t.Run("csv-put --file parses as --csv", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+csv-put")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--start-cell", "A1",
			"--file", "a,b\n1,2",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--file with CSV text should alias to --csv and pass, got: %v", err)
		}
		if !strings.Contains(stdout, "a,b") {
			t.Errorf("dry-run body should carry the CSV text, got %q", stdout)
		}
	})

	t.Run("cols-resize --size parses as --width", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cols-resize")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A:C",
			"--size", "120",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--size should alias to --width (styles-protocol vocabulary), got: %v", err)
		}
		if !strings.Contains(stdout, "120") {
			t.Errorf("dry-run body should carry the pixel width 120, got %q", stdout)
		}
	})

	t.Run("rows-resize --size parses as --height", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+rows-resize")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "1:3",
			"--size", "36",
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--size should alias to --height (styles-protocol vocabulary), got: %v", err)
		}
	})

	t.Run("cells-set --values parses as --cells", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cells-set")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "C1",
			// The gspread payload verbatim: a plain values matrix, not cell
			// objects. It rides through unchanged because the alias fixes the
			// name and normalizeCellsFlagValue lifts the bare scalar — the two
			// halves have to hold together for the silent tier to be correct.
			"--values", `[["工作内容"]]`,
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("--values should alias to --cells and pass, got: %v", err)
		}
		input := decodeToolInput(t, decodeDryRunFirstCall(t, stdout), "set_cell_range")
		cells, _ := json.Marshal(input["cells"])
		if string(cells) != `[[{"value":"工作内容"}]]` {
			t.Errorf("cells = %s, want the lifted [[{\"value\":\"工作内容\"}]]", cells)
		}
	})

	t.Run("alias never shadows a registered flag", func(t *testing.T) {
		t.Parallel()
		c := &cobra.Command{Use: "+csv-put"}
		c.Flags().String("csv", "", "")
		c.Flags().String("file", "", "") // hypothetical real flag wins
		chainFlagAliases(c)
		if err := c.ParseFlags([]string{"--file", "x"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got, _ := c.Flags().GetString("file"); got != "x" {
			t.Errorf("registered --file should keep its own value, got %q", got)
		}
		if got, _ := c.Flags().GetString("csv"); got != "" {
			t.Errorf("--csv must stay empty when --file is a real flag, got %q", got)
		}
	})
}

// TestShortcuts_IntuitiveFlagHints verifies the prescription tier: habitual
// names whose fix is not a rename answer with the exact correct form, so the
// retry needs no --help round trip (eval: +sheet-copy burned 3/3 post-error
// --help calls, +dim-insert kept failing even after reading help).
func TestShortcuts_IntuitiveFlagHints(t *testing.T) {
	t.Parallel()
	cases := []struct {
		command  string
		args     []string
		wrong    string
		wantHint []string
		// rejectHint pins what a prescription must NOT name — used where the
		// obvious wording would steer into a deprecated flag.
		rejectHint []string
	}{
		{
			command:  "+dim-insert",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--dimension", "row"},
			wrong:    "--dimension",
			wantHint: []string{"--position", "--count"},
		},
		{
			command: "+dim-freeze",
			args:    []string{"--url", testURL, "--sheet-name", "s", "--frozen-rows", "2"},
			wrong:   "--frozen-rows",
			// Must prescribe the CURRENT spelling: --dimension/--count is
			// retired and hidden from --help, so a hint naming it would point at
			// a flag missing from the same error's valid-flags list.
			wantHint:   []string{"--rows N"},
			rejectHint: []string{"--dimension", "--count"},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--bold", "true"},
			wrong:    "--bold",
			wantHint: []string{"--font-weight bold"},
		},
		{
			command:  "+sheet-copy",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--new-sheet-name", "副本"},
			wrong:    "--new-sheet-name",
			wantHint: []string{"--title", "source sheet"},
		},
		{
			command:  "+table-put",
			args:     []string{"--url", testURL, "--sheets", "{}", "--start-cell", "B2"},
			wrong:    "--start-cell",
			wantHint: []string{`"start_cell"`, "+csv-put"},
		},
		{
			command:    "+dim-freeze",
			args:       []string{"--url", testURL, "--sheet-name", "s", "--frozen-row-count", "1"},
			wrong:      "--frozen-row-count",
			wantHint:   []string{"--rows N"},
			rejectHint: []string{"--dimension", "--count"},
		},
		{
			// The parse error reports the flag as typed: the underscore
			// spelling must hit the same curated entry as the hyphenated one.
			command:    "+dim-freeze",
			args:       []string{"--url", testURL, "--sheet-name", "s", "--frozen_rows", "2"},
			wrong:      "--frozen_rows",
			wantHint:   []string{"--rows N"},
			rejectHint: []string{"--dimension", "--count"},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--font-bold", "true"},
			wrong:    "--font-bold",
			wantHint: []string{"--font-weight bold"},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--bg-color", "#FFF"},
			wrong:    "--bg-color",
			wantHint: []string{"--background-color"},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--wrap-strategy", "overflow"},
			wrong:    "--wrap-strategy",
			wantHint: []string{"--word-wrap"},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--border-all", "thin"},
			wrong:    "--border-all",
			wantHint: []string{"--border-styles", `"all"`},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--border-top", "thin"},
			wrong:    "--border-top",
			wantHint: []string{"--border-styles", `"top"`},
		},
		{
			command:  "+cells-set-style",
			args:     []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--border-color", "#000"},
			wrong:    "--border-color",
			wantHint: []string{"--border-styles", "color"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.command+" "+tc.wrong, func(t *testing.T) {
			t.Parallel()
			sc := shortcutFromRegistry(t, tc.command)
			_, _, err := runShortcutCapturingErr(t, sc, tc.args)
			ve := requireValidation(t, err, "unknown flag \""+tc.wrong+"\"")
			for _, want := range tc.wantHint {
				if !strings.Contains(ve.Hint, want) {
					t.Errorf("hint should contain %q, got %q", want, ve.Hint)
				}
			}
			// The valid-flags list is appended to the same Hint, so only the
			// prescription itself is checked for banned wording.
			prescription, _, _ := strings.Cut(ve.Hint, "; valid flags:")
			for _, banned := range tc.rejectHint {
				if strings.Contains(prescription, banned) {
					t.Errorf("prescription must not steer to %q, got %q", banned, prescription)
				}
			}
			// A curated prescription must not ship contradicting edit-distance
			// candidates (--font-bold used to carry --font-color/--font-line/
			// --font-size in params while the fix is --font-weight).
			for _, p := range ve.Params {
				if len(p.Suggestions) > 0 {
					t.Errorf("curated prescription should drop edit-distance suggestions, got %v", p.Suggestions)
				}
			}
		})
	}
}
