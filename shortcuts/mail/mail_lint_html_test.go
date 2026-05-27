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
// envelope shape.
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
// `..` traversal are rejected by the path safety check.
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
