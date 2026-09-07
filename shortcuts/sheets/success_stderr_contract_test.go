// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// The sheets success contract, and the exact reach of what is proven here:
//
//	a successful JSON run writes its whole answer to stdout and nothing to
//	stderr.
//
// Advisories the domain used to print on the success path (ignored sub-op
// locators, emulated semantics, deprecated spellings, the dropdown
// option-error steer, a corrected file extension) now travel in the payload.
// That matters because PowerShell's native-command handling and most agent
// harnesses treat non-empty stderr as failure: a warning printed there turned
// a working call into a reported error.
//
// The coverage below includes the two commands that delegate to the shared
// drive export / import cores, since those cores were cleaned up as part of
// this change. One path a sheets caller can still reach is NOT covered,
// because its output comes from a shared helper this change does not touch:
//
//   - any bot-identity create/import → common.AutoGrantCurrentUserDrivePermission
//     duplicates its permission_grant result on stderr when the grant is
//     skipped or fails
//
// Multipart progress now uses the shared TTY-only spinner, so captured sheets
// output stays silent without command-specific suppression flags.

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

// TestNoDirectStderrWritesInSheetsPackage is the anti-regression guard the
// audit asks for: sheets code must not write to ErrOut at all — there is no
// allowlist. Typed errors already reach stderr through the emitter, and
// everything else belongs in the result. If a future path genuinely needs a
// human-facing channel, it has to be an explicitly subscribed one, not a bare
// Fprintf here.
func TestNoDirectStderrWritesInSheetsPackage(t *testing.T) {
	t.Parallel()

	const dir = "."
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
			t.Errorf("%s:%d writes to stderr on a shortcut path: %s\n"+
				"put the information in the success payload (warnings / effective_operation / "+
				"deprecation) instead", path, i+1, line)
		}
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

// TestAnnotateSheetsResultShapes pins the three payload shapes callTool can
// hand back. The array case is the one that matters: an annotation must not be
// dropped just because the tool answered with something other than an object,
// so the tool's own answer survives under `result`. A nil result stays nil-free
// — inventing "result": null would name a result the tool never returned.
func TestAnnotateSheetsResultShapes(t *testing.T) {
	t.Parallel()

	object := annotateSheetsResult(map[string]interface{}{"success": true}, "warnings", []string{"w"})
	objectMap, _ := object.(map[string]interface{})
	if objectMap["success"] != true || objectMap["warnings"] == nil {
		t.Errorf("an object result must be annotated in place, got %#v", object)
	}
	if _, wrapped := objectMap["result"]; wrapped {
		t.Errorf("an object result must not be wrapped, got %#v", object)
	}

	array := annotateSheetsResult([]interface{}{1, 2}, "warnings", []string{"w"})
	arrayMap, _ := array.(map[string]interface{})
	inner, _ := arrayMap["result"].([]interface{})
	if len(inner) != 2 {
		t.Errorf("a non-object result must survive under result, got %#v", array)
	}
	if arrayMap["warnings"] == nil {
		t.Errorf("the annotation must survive alongside it, got %#v", array)
	}

	empty := annotateSheetsResult(nil, "warnings", []string{"w"})
	emptyMap, _ := empty.(map[string]interface{})
	if emptyMap["warnings"] == nil {
		t.Errorf("an annotation on an empty result must still be reported, got %#v", empty)
	}
	if _, invented := emptyMap["result"]; invented {
		t.Errorf("no result key may be invented when the tool returned nothing, got %#v", empty)
	}
}

// TestAppendSheetsWarningsMergesAndSkips pins that warnings accumulate instead
// of overwriting each other, and that an empty warning list leaves the payload
// byte-identical — the property that keeps clean calls' output shape unchanged.
func TestAppendSheetsWarningsMergesAndSkips(t *testing.T) {
	t.Parallel()

	merged := appendSheetsWarnings(map[string]interface{}{"warnings": []string{"first"}}, []string{"second"})
	mergedMap, _ := merged.(map[string]interface{})
	if got, _ := mergedMap["warnings"].([]string); len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("warnings should accumulate, got %#v", mergedMap["warnings"])
	}

	// A payload decoded from JSON carries []interface{}, not []string; that
	// branch merges too, and losing it would silently drop prior warnings.
	decoded := appendSheetsWarnings(map[string]interface{}{"warnings": []interface{}{"first"}}, []string{"second"})
	decodedMap, _ := decoded.(map[string]interface{})
	if got, _ := decodedMap["warnings"].([]interface{}); len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("decoded warnings should accumulate in order, got %#v", decodedMap["warnings"])
	}

	untouched := appendSheetsWarnings(map[string]interface{}{"success": true}, nil)
	untouchedMap, _ := untouched.(map[string]interface{})
	if _, present := untouchedMap["warnings"]; present {
		t.Errorf("no warnings means no warnings field, got %#v", untouched)
	}
}

// TestDimInsertReportsEmulatedAnchorInPayload pins the +dim-insert half of the
// effective_operation contract. --inherit-style before is emulated by anchoring
// one row/column earlier and inserting after it, so the request body carries a
// position the caller never typed; without this block in the result, a caller
// diffing the request against what they asked for reads it as an off-by-one.
func TestDimInsertReportsEmulatedAnchorInPayload(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCapturingStderr(t, DimInsert, []string{
		"--url", testURL, "--sheet-id", "sh1",
		"--position", "C", "--count", "1",
		"--inherit-style", "before",
	}, toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("a successful insert must leave stderr empty, got: %q", stderr)
	}

	effective, _ := decodeEnvelopeData(t, stdout)["effective_operation"].(map[string]interface{})
	if effective == nil {
		t.Fatalf("expected data.effective_operation for an emulated --inherit-style before: %s", stdout)
	}
	for field, want := range map[string]interface{}{
		"requested_position": "C",
		"anchor_position":    "B", // one column earlier: insert-after-B == insert-before-C
		"side":               "after",
		"inherit_style":      "before",
	} {
		if effective[field] != want {
			t.Errorf("effective_operation[%q] = %#v, want %#v", field, effective[field], want)
		}
	}

	// The non-emulated spelling must not gain the block: nothing was rewritten.
	stdout, _, err = runCapturingStderr(t, DimInsert, []string{
		"--url", testURL, "--sheet-id", "sh1",
		"--position", "C", "--count", "1",
		"--inherit-style", "after",
	}, toolOutputStub(testToken, "write", `{"success":true}`))
	if err != nil {
		t.Fatalf("execute failed: %v\nstdout=%s", err, stdout)
	}
	if _, present := decodeEnvelopeData(t, stdout)["effective_operation"]; present {
		t.Errorf("--inherit-style after anchors where the caller asked; no effective_operation expected: %s", stdout)
	}
}

// TestBatchUpdateKeepsAdvisoriesOnFailure pins the failure half of the warning
// contract. batch_update is fail-fast and does not roll back, so a partial
// failure is exactly when a caller needs to know which locators were ignored:
// it decides which target was really written and which sub-ops are safe to
// resend. The advisories used to be printed before the request, so they
// survived the failure; moving them to the success payload alone would have
// dropped them from the path that needs them most.
func TestBatchUpdateKeepsAdvisoriesOnFailure(t *testing.T) {
	t.Parallel()

	failing := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_write",
		Body: map[string]interface{}{
			"code": 1310213,
			"msg":  "batch_update: 1 succeeded, 1 failed",
		},
	}

	stdout, stderr, err := runCapturingStderr(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[
		  {"shortcut":"+cells-clear","input":{"sheet_name":"S1","range":"A1:B2","excel_id":"shtIGNORED"}},
		  {"shortcut":"+cells-clear","input":{"sheet_name":"S1","range":"C1:D2"}}
		]`,
		"--yes",
	}, failing)
	if err == nil {
		t.Fatalf("expected the tool failure to surface, got nil\nstdout=%s\nstderr=%s", stdout, stderr)
	}

	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed error, got %T: %v", err, err)
	}
	for _, want := range []string{"operations[0] (+cells-clear)", "excel_id", "safe retry set"} {
		if !strings.Contains(problem.Hint, want) {
			t.Errorf("failure hint should carry the ignored-locator advisory (%q), got: %q", want, problem.Hint)
		}
	}
	if problem.Message == "" || !strings.Contains(problem.Message, "failed") {
		t.Errorf("the tool's own failure must stay the message, got: %q", problem.Message)
	}
}
