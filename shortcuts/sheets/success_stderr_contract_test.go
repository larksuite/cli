// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// The sheets success contract, and the exact reach of what is proven here:
//
//	code OWNED by the sheets packages writes its whole answer to stdout and
//	nothing to stderr on a successful run.
//
// Advisories the domain used to print on the success path (ignored sub-op
// locators, emulated semantics, deprecated spellings, the dropdown
// option-error steer) now travel in the payload. That matters because
// PowerShell's native-command handling and most agent harnesses treat
// non-empty stderr as failure: a warning printed there turned a working call
// into a reported error.
//
// NOT proven here, and deliberately still noisy — these shortcuts reach shared
// implementations outside the sheets packages, whose cleanup belongs to the
// domain that owns them:
//
//   - +workbook-export      → drive.RunExport narrates task creation, each
//     poll, and completion
//   - +workbook-import      → drive.RunImport narrates upload, task creation
//     and completion; the sheets-owned extension-correction note is the one
//     allowlisted write below
//   - +media-upload (>20MB) → common.UploadDriveMediaMultipartTyped narrates
//     the chunk plan and every block
//
// Do not read TestSheetsSuccessPathsLeaveStderrEmpty as a domain-wide
// guarantee: it covers the paths this change actually fixed. When the shared
// paths are cleaned up, add those three commands here and delete the
// allowlist in TestNoDirectStderrWritesInSheetsPackages.

// runCapturingStderr runs a shortcut against stubs and returns stdout, stderr
// and the error, so a test can assert on the reporting channel and not just
// the payload.
func runCapturingStderr(t *testing.T, sc common.Shortcut, args []string, stubs ...*httpmock.Stub) (stdoutStr, stderrStr string, err error) {
	t.Helper()
	parent, stdout, stderr, reg := newTestRig(t, sc)
	for _, s := range stubs {
		reg.Register(s)
	}
	parent.SetArgs(append([]string{sc.Command}, args...))
	err = parent.Execute()
	return stdout.String(), stderr.String(), err
}

func TestSheetsSuccessPathsLeaveStderrEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sc   common.Shortcut
		args []string
	}{
		{
			// Used to print "note: --dimension/--count is superseded by …".
			name: "dim-freeze via the deprecated --dimension/--count pair",
			sc:   DimFreeze,
			args: []string{"--url", testURL, "--sheet-id", "sh1", "--dimension", "row", "--count", "1"},
		},
		{
			// Used to print the option-error warning from Validate.
			name: "dropdown-set with an oversized highlighted source range",
			sc:   DropdownSet,
			args: []string{"--url", testURL, "--sheet-id", "sh1", "--range", "A1:A10", "--source-range", "Sheet1!B1:B3000"},
		},
		{
			// Used to print "+cells-batch-set-style is superseded by …".
			name: "deprecated cells-batch-set-style",
			sc:   CellsBatchSetStyle,
			args: []string{"--url", testURL, "--ranges", `["Sheet1!A1:B2"]`, "--font-weight", "bold"},
		},
		{
			// Used to print one ignored-locator line per offending sub-op.
			name: "batch-update with ignored sub-op locators",
			sc:   BatchUpdate,
			args: []string{
				"--url", testURL,
				"--operations", `[{"shortcut":"+cells-clear","input":{"sheet_name":"S1","range":"A1:B2","excel_id":"shtIGNORED"}}]`,
				"--yes",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runCapturingStderr(t, tc.sc, tc.args,
				toolOutputStub(testToken, "write", `{"success":true}`))
			if err != nil {
				t.Fatalf("execute failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if stderr != "" {
				t.Errorf("a successful run must leave stderr empty, got: %q", stderr)
			}
			if !strings.Contains(stdout, `"ok": true`) {
				t.Errorf("expected a success envelope on stdout, got: %s", stdout)
			}
		})
	}
}

// allowedSheetsStderrWrite is the single tolerated direct stderr write in the
// sheets packages, matched exactly rather than by file: +workbook-import's
// extension-correction note has nowhere else to go, because drive.RunImport
// owns the whole output envelope and has no slot for a caller-supplied
// correction. Giving it one is a change to the shared drive import core, which
// this sheets-scoped pass deliberately leaves alone.
//
// Keyed by file and matched on the exact statement, so any OTHER write in the
// same file is still caught.
var allowedSheetsStderrWrite = struct {
	file string
	line string
}{
	file: "lark_sheet_workbook.go",
	line: "fmt.Fprintln(runtime.IO().ErrOut, note)",
}

// TestNoDirectStderrWritesInSheetsPackages is the anti-regression guard the
// audit asks for: sheets code must not write to ErrOut, with exactly one
// documented exception. Typed errors already reach stderr through the emitter,
// and everything else belongs in the result. If a future path genuinely needs
// a human-facing channel, it has to be an explicitly subscribed one, not a
// bare Fprintf here.
//
// The exception is asserted to still exist, not merely permitted: when the
// shared import core can carry input corrections and that write goes away,
// this test fails and forces the allowlist (and the contract comment at the
// top of this file) to be updated with it.
func TestNoDirectStderrWritesInSheetsPackages(t *testing.T) {
	t.Parallel()

	allowedHits := 0
	for _, dir := range []string{".", "backward"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for i, raw := range strings.Split(string(body), "\n") {
				line := strings.TrimSpace(raw)
				if !strings.Contains(line, "ErrOut") || strings.HasPrefix(line, "//") {
					continue
				}
				if name == allowedSheetsStderrWrite.file && line == allowedSheetsStderrWrite.line {
					allowedHits++
					continue
				}
				t.Errorf("%s:%d writes to stderr on a shortcut path: %s\n"+
					"put the information in the success payload (warnings / effective_operation / "+
					"deprecation) instead", path, i+1, line)
			}
		}
	}

	if allowedHits != 1 {
		t.Errorf("expected exactly 1 allowlisted stderr write (%s in %s), found %d — "+
			"if it was removed, delete the allowlist and extend the contract comment at the top of this file",
			allowedSheetsStderrWrite.line, allowedSheetsStderrWrite.file, allowedHits)
	}
}

// TestDimFreezeReportsEffectiveStateAndDeprecation pins where the legacy
// --dimension/--count steer went: the result's own `deprecation` key, while
// the state the call actually leaves behind — freezing replaces the whole
// state rather than adding to it — is reported as effective_operation.
func TestDimFreezeReportsEffectiveStateAndDeprecation(t *testing.T) {
	t.Parallel()

	stdout, _, err := runCapturingStderr(t, DimFreeze,
		[]string{"--url", testURL, "--sheet-id", "sh1", "--dimension", "row", "--count", "1"},
		toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s", err, stdout)
	}

	data := decodeEnvelopeData(t, stdout)
	if deprecation, _ := data["deprecation"].(string); !strings.Contains(deprecation, "--rows/--cols") {
		t.Errorf("deprecation should steer to --rows/--cols, got %q", data["deprecation"])
	}
	effective, _ := data["effective_operation"].(map[string]interface{})
	if effective == nil {
		t.Fatalf("expected data.effective_operation, got %#v", data)
	}
	if effective["frozen_rows"] != float64(1) || effective["frozen_cols"] != float64(0) {
		t.Errorf("effective_operation should report the whole resulting state, got %#v", effective)
	}
}

// TestCellsBatchSetStyleReportsDeprecation pins that the superseded command
// steers callers in-band, under a key of its own rather than mixed into the
// warnings that describe the write itself.
func TestCellsBatchSetStyleReportsDeprecation(t *testing.T) {
	t.Parallel()

	stdout, _, err := runCapturingStderr(t, CellsBatchSetStyle,
		[]string{"--url", testURL, "--ranges", `["Sheet1!A1:B2"]`, "--font-weight", "bold"},
		toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s", err, stdout)
	}

	data := decodeEnvelopeData(t, stdout)
	if deprecation, _ := data["deprecation"].(string); !strings.Contains(deprecation, "+styles-put") {
		t.Errorf("deprecation should steer to +styles-put, got %q", data["deprecation"])
	}
	if _, mixed := data["warnings"]; mixed {
		t.Errorf("a deprecation steer is not a warning about the write: %#v", data)
	}
}

// TestDropdownSetSurfacesOptionErrorWarningInPayload pins that the
// highlight-vs-source-size steer survived the move off stderr: it is the one
// signal telling a caller the dropdown they just installed is in the server's
// option-error state.
func TestDropdownSetSurfacesOptionErrorWarningInPayload(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCapturingStderr(t, DropdownSet,
		[]string{"--url", testURL, "--sheet-id", "sh1", "--range", "A1:A10", "--source-range", "Sheet1!B1:B3000"},
		toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	data := decodeEnvelopeData(t, stdout)
	warnings, _ := data["warnings"].([]interface{})
	if len(warnings) != 1 {
		t.Fatalf("expected one warning in the payload, got %#v", data["warnings"])
	}
	if warning, _ := warnings[0].(string); !strings.Contains(warning, "option-error") {
		t.Errorf("warning should name the option-error state, got %q", warnings[0])
	}

	// A request under the cap keeps its previous payload shape exactly.
	stdout, _, err = runCapturingStderr(t, DropdownSet,
		[]string{"--url", testURL, "--sheet-id", "sh1", "--range", "A1:A10", "--source-range", "Sheet1!B1:B10"},
		toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s", err, stdout)
	}
	if _, present := decodeEnvelopeData(t, stdout)["warnings"]; present {
		t.Errorf("a request within limits must not gain a warnings field: %s", stdout)
	}
}

// TestBatchUpdateSurfacesIgnoredLocatorsInPayload pins that the ignored-locator
// notes — which decide whether a caller can safely retry a sub-op — reach the
// result instead of stderr.
func TestBatchUpdateSurfacesIgnoredLocatorsInPayload(t *testing.T) {
	t.Parallel()

	stdout, _, err := runCapturingStderr(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+cells-clear","input":{"sheet_name":"S1","range":"A1:B2","excel_id":"shtIGNORED"}}]`,
		"--yes",
	}, toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s", err, stdout)
	}
	data := decodeEnvelopeData(t, stdout)
	warnings, _ := data["warnings"].([]interface{})
	if len(warnings) != 1 {
		t.Fatalf("expected the ignored-locator note in the payload, got %#v", data)
	}
	if warning, _ := warnings[0].(string); !strings.Contains(warning, "excel_id") {
		t.Errorf("warning should name the ignored key, got %q", warnings[0])
	}
}

// TestKnownGap_SharedCoresStillWriteStderr makes the scope boundary executable
// instead of leaving it as prose. +workbook-export and +workbook-import
// delegate to drive.RunExport / drive.RunImport, which narrate task creation,
// polling and completion; that code is shared with the drive domain and is not
// this change's to touch.
//
// The assertion is deliberately inverted: it pins that the noise is STILL
// there. When the shared cores stop writing, this test fails and points at the
// contract test that should then cover these two commands — so the gap cannot
// quietly outlive its fix, and nobody can read the suite as claiming the whole
// domain is already silent.
//
// (+media-upload over 20MB has the same shape through
// common.UploadDriveMediaMultipartTyped, but pinning it would mean staging a
// >20MB fixture on every run, so it is documented at the top of this file
// rather than asserted.)
func TestKnownGap_SharedCoresStillWriteStderr(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCapturingStderr(t, WorkbookExport,
		[]string{"--url", testURL, "--file-extension", "xlsx", "--as", "user"},
		&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/export_tasks",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"ticket": "tk_export"},
			},
		},
		&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/drive/v1/export_tasks/tk_export",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"result": map[string]interface{}{
					"job_status": float64(0),
					"file_token": "ftk_xlsx",
					"file_name":  "report.xlsx",
					"file_size":  float64(2048),
				}},
			},
		})
	if err != nil {
		t.Fatalf("export execute failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	for _, want := range []string{"Created export task", "Export task completed"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected the shared drive export core to still print %q on stderr, got: %q\n"+
				"if the shared core was cleaned up, delete this test and add +workbook-export to "+
				"TestSheetsSuccessPathsLeaveStderrEmpty and to the contract comment at the top of this file",
				want, stderr)
		}
	}
}
