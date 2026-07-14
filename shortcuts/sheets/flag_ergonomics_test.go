// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
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
			"--cols", "A:D",
		})
		ve := requireValidation(t, err, `unknown flag "--cols"`)
		for _, want := range []string{"valid flags:", "--range", "--width", "--widths"} {
			if !strings.Contains(ve.Hint, want) {
				t.Errorf("hint should contain %q, got %q", want, ve.Hint)
			}
		}
	})
}
