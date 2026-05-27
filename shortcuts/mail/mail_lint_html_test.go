// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// =====================================================================
// +lint-html Shortcut tests — public stdout envelope contract checks.
//
// These exercise the full cobra Mount → Execute pipeline (parse args →
// Validate → Execute → OutFormat) so they catch any regression in flag
// declaration, mutual-exclusion validation, path safety, and the JSON
// envelope shape (spec §4.2 + S2 contract «Stdout envelope contract»).
// =====================================================================

// TestMailLintHTML_RequiresExactlyOneOfBodyOrFile verifies the mutual-
// exclusion + at-least-one-of constraint surfaces ErrValidation.
func TestMailLintHTML_RequiresExactlyOneOfBodyOrFile(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	t.Run("neither flag", func(t *testing.T) {
		err := runMountedMailShortcut(t, MailLintHTML, []string{"+lint-html"}, f, stdout)
		if err == nil {
			t.Fatal("expected error when neither flag is set")
		}
		if !strings.Contains(err.Error(), "exactly one of --body or --body-file") {
			t.Errorf("wrong error: %v", err)
		}
	})

	t.Run("both flags", func(t *testing.T) {
		err := runMountedMailShortcut(t, MailLintHTML, []string{
			"+lint-html",
			"--body", "<p>x</p>",
			"--body-file", "fake.html",
		}, f, stdout)
		if err == nil {
			t.Fatal("expected error when both flags set")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("wrong error: %v", err)
		}
	})
}

// TestMailLintHTML_BodyFilePathSafetyRejected verifies absolute paths /
// `..` traversal are rejected (KB Pitfall 4 + S2 contract «Public input
// surface inventory»).
func TestMailLintHTML_BodyFilePathSafetyRejected(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	chdirTemp(t)

	t.Run("absolute path", func(t *testing.T) {
		err := runMountedMailShortcut(t, MailLintHTML, []string{
			"+lint-html",
			"--body-file", "/etc/passwd",
		}, f, stdout)
		if err == nil {
			t.Fatal("expected validation error for absolute path")
		}
	})

	t.Run("dotdot traversal", func(t *testing.T) {
		err := runMountedMailShortcut(t, MailLintHTML, []string{
			"+lint-html",
			"--body-file", "../../../etc/passwd",
		}, f, stdout)
		if err == nil {
			t.Fatal("expected validation error for traversal")
		}
	})
}

// TestMailLintHTML_BodyFileReadsCwdSubpath verifies a legitimate cwd-subtree
// path loads HTML correctly.
func TestMailLintHTML_BodyFileReadsCwdSubpath(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	chdirTemp(t)
	if err := os.WriteFile("input.html", []byte(`<p>safe</p><script>1</script>`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body-file", "input.html",
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	errors, _ := data["errors"].([]interface{})
	if len(errors) != 1 {
		t.Errorf("expected 1 error finding (script), got %d: %+v", len(errors), errors)
	}
	cleaned, _ := data["cleaned_html"].(string)
	if strings.Contains(cleaned, "<script") {
		t.Errorf("cleaned_html should not contain <script>, got %q", cleaned)
	}
}

// TestMailLintHTML_DefaultEnvelopeShape verifies the default envelope only
// contains cleaned_html — warnings[] / errors[] are token-frugally suppressed
// unless --show-lint-details is passed.
func TestMailLintHTML_DefaultEnvelopeShape(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<p>safe content</p>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if _, ok := data["cleaned_html"]; !ok {
		t.Error("cleaned_html key missing from envelope (default --auto-fix=true)")
	}
	if _, ok := data["warnings"]; ok {
		t.Error("warnings[] must be hidden in default mode (use --show-lint-details to surface)")
	}
	if _, ok := data["errors"]; ok {
		t.Error("errors[] must be hidden in default mode (use --show-lint-details to surface)")
	}
}

// TestMailLintHTML_ShowLintDetailsExposesArrays verifies --show-lint-details
// surfaces the full warnings[] / errors[] arrays alongside cleaned_html.
func TestMailLintHTML_ShowLintDetailsExposesArrays(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<p>safe content</p>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if _, ok := data["warnings"]; !ok {
		t.Error("warnings[] missing in --show-lint-details mode")
	}
	if _, ok := data["errors"]; !ok {
		t.Error("errors[] missing in --show-lint-details mode")
	}
}

// TestMailLintHTML_AutoFixFalseOmitsCleanedHTML verifies spec §4.2 row
// "--auto-fix=false → cleaned_html absent". Pairs --auto-fix=false with
// --show-lint-details so both finding arrays remain visible.
func TestMailLintHTML_AutoFixFalseOmitsCleanedHTML(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<p>x</p>`,
		"--auto-fix=false",
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if _, ok := data["cleaned_html"]; ok {
		t.Errorf("cleaned_html should be absent when --auto-fix=false")
	}
	if _, ok := data["warnings"]; !ok {
		t.Error("warnings should still be present under --show-lint-details")
	}
	if _, ok := data["errors"]; !ok {
		t.Error("errors should still be present under --show-lint-details")
	}
}

// TestMailLintHTML_StrictExitsNonZero verifies --strict bumps the exit code.
func TestMailLintHTML_StrictExitsNonZero(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<font color="red">x</font>`,
		"--strict",
		"--show-lint-details",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected non-zero exit under --strict with a warning, got nil")
	}
	// Envelope is still emitted (Execute writes to stdout before returning).
	data := decodeShortcutEnvelopeData(t, stdout)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Error("--strict should promote warnings to errors")
	}
}

// TestMailLintHTML_StrictPassesOnCleanInput verifies --strict exits 0 when
// no findings are surfaced.
func TestMailLintHTML_StrictPassesOnCleanInput(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<p>safe content</p>`,
		"--strict",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected success on clean input under strict, got: %v", err)
	}
}

// TestMailLintHTML_PlainTextBodyShortCircuits verifies plain-text input
// produces empty arrays (lib short-circuit path) when --show-lint-details is
// set; without the flag, the arrays are omitted entirely.
func TestMailLintHTML_PlainTextBodyShortCircuits(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", "just plain text, no markup",
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	w, _ := data["warnings"].([]interface{})
	e, _ := data["errors"].([]interface{})
	if len(w) != 0 || len(e) != 0 {
		t.Errorf("plain text should produce no findings, got w=%v e=%v", w, e)
	}
}

// TestMailLintHTML_FindingShape verifies each finding entry has the
// contract-required keys (rule_id / severity / tag_or_attr / excerpt / hint).
func TestMailLintHTML_FindingShape(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<p>x</p><script>alert(1)</script>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("expected at least 1 error finding")
	}
	first, _ := errors[0].(map[string]interface{})
	for _, key := range []string{"rule_id", "severity", "tag_or_attr", "excerpt", "hint"} {
		if _, ok := first[key]; !ok {
			t.Errorf("finding missing required key %q: %+v", key, first)
		}
	}
	if first["severity"] != "error" {
		t.Errorf("severity = %v, want error", first["severity"])
	}
	if !strings.HasPrefix(first["rule_id"].(string), "TAG_") &&
		!strings.HasPrefix(first["rule_id"].(string), "ATTR_") &&
		!strings.HasPrefix(first["rule_id"].(string), "STYLE_") {
		t.Errorf("rule_id must be UPPER_SNAKE_CASE prefix, got %v", first["rule_id"])
	}
}

// TestMailLintHTML_DryRun verifies dry-run mode doesn't execute lint and
// surfaces the read-only / no-network annotation.
func TestMailLintHTML_DryRun(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<p>x</p>`,
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Dry-run output is JSON containing "mode":"local-lint-only".
	if !strings.Contains(stdout.String(), "local-lint-only") {
		t.Errorf("expected dry-run mode marker, stdout=%s", stdout.String())
	}
}

// TestMailLintHTML_BlockedTagAndWarningAccumulate verifies the report
// surfaces both warning + error findings simultaneously.
func TestMailLintHTML_BlockedTagAndWarningAccumulate(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	body := `<font color="red">warn-tag</font><script>err-tag</script>` +
		`<a href="javascript:0">err-url</a>`
	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", body,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	w, _ := data["warnings"].([]interface{})
	e, _ := data["errors"].([]interface{})
	if len(w) < 1 {
		t.Errorf("expected ≥ 1 warning, got %d", len(w))
	}
	if len(e) < 2 {
		t.Errorf("expected ≥ 2 errors (script + js URL), got %d", len(e))
	}
}

// TestMailLintHTML_FindingsAreJSONSerialisable confirms the cleaned envelope
// can round-trip through json (no nil / function values leak in).
func TestMailLintHTML_FindingsAreJSONSerialisable(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailLintHTML, []string{
		"+lint-html",
		"--body", `<font color="red">x</font>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Re-encode the data back to JSON to confirm it's serialisable.
	data := decodeShortcutEnvelopeData(t, stdout)
	if _, err := json.Marshal(data); err != nil {
		t.Errorf("envelope not JSON-serialisable: %v", err)
	}
}
