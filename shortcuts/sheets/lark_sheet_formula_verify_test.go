// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestFormulaVerify_DryRun pins the wire shape verify_formula sends for the
// common input combinations: no selector (workbook-wide scan), explicit
// sheet_ids, explicit ranges, and the optional max_locations_per_error
// field. The test exercises the One-OpenAPI body
// directly so the schema field names stay locked to the canonical
// tool-schemas.json verify_formula node.
func TestFormulaVerify_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantInput map[string]interface{}
	}{
		{
			name: "no selector — workbook-wide scan defaults",
			args: []string{"--url", testURL},
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name: "sheet_ids multi via repeat",
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--sheet-id", testSheetID2},
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_ids": []interface{}{testSheetID, testSheetID2},
			},
		},
		{
			name: "sheet_names multi via comma",
			args: []string{"--url", testURL, "--sheet-name", "Sheet1,Sheet2"},
			wantInput: map[string]interface{}{
				"excel_id":    testToken,
				"sheet_names": []interface{}{"Sheet1", "Sheet2"},
			},
		},
		{
			name: "ranges + max_locations",
			args: []string{
				"--url", testURL,
				"--range", "A1:Z200",
				"--range", "AA1:AZ100",
				"--max-locations", "5",
			},
			wantInput: map[string]interface{}{
				"excel_id":                testToken,
				"ranges":                  []interface{}{"A1:Z200", "AA1:AZ100"},
				"max_locations_per_error": float64(5),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, FormulaVerify, tt.args)
			got := decodeToolInput(t, body, "verify_formula")
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestFormulaVerify_DryRunInvokeReadPath confirms the request hits
// invoke_read (read scope) and not invoke_write — a scope mismatch here would
// surface as a 403 from the gateway.
func TestFormulaVerify_DryRunInvokeReadPath(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, FormulaVerify, []string{"--url", testURL})
	if len(calls) == 0 {
		t.Fatalf("dry-run produced no api calls")
	}
	call, _ := calls[0].(map[string]interface{})
	url, _ := call["url"].(string)
	if !strings.HasSuffix(url, "/tools/invoke_read") {
		t.Errorf("verify_formula must hit invoke_read; got url=%q", url)
	}
	if want := "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read"; url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
}

// TestFormulaVerify_RejectsBothSelectors locks the "at most one selector"
// rule on the two multi-value flags. Both empty is the documented
// workbook-wide scan path, so we only reject the both-supplied case.
func TestFormulaVerify_RejectsBothSelectors(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, FormulaVerify, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--sheet-name", "Sheet1",
		"--dry-run",
	})
	ve := requireValidation(t, err, "mutually exclusive")
	gotParams := map[string]bool{}
	for _, p := range ve.Params {
		gotParams[p.Name] = true
	}
	if !gotParams["--sheet-id"] || !gotParams["--sheet-name"] {
		t.Errorf("params = %#v, want both --sheet-id and --sheet-name flagged", ve.Params)
	}
}

// TestFormulaVerify_RejectsNonPositiveLimits guards against typos like
// `--max-locations 0`, which would otherwise be silently swallowed by the
// "explicit value but unset" comparison in the input builder.
func TestFormulaVerify_RejectsNonPositiveLimits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "max-locations=0",
			args: []string{"--url", testURL, "--max-locations", "0"},
			want: "--max-locations must be > 0",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, FormulaVerify, append(c.args, "--dry-run"))
			requireValidation(t, err, c.want)
		})
	}
}

// TestFormulaVerifyExitOnError_StatusMatrix locks the --exit-on-error
// contract: success/partial → no error; errors_found → typed validation
// error with SubtypeFailedPrecondition; missing or unknown status →
// typed internal error so a silent zero-exit can never happen.
func TestFormulaVerifyExitOnError_StatusMatrix(t *testing.T) {
	t.Parallel()

	t.Run("success returns no error", func(t *testing.T) {
		t.Parallel()
		if err := formulaVerifyExitOnError(map[string]interface{}{"status": "success"}); err != nil {
			t.Fatalf("success path returned err: %v", err)
		}
	})

	t.Run("partial returns no error", func(t *testing.T) {
		t.Parallel()
		if err := formulaVerifyExitOnError(map[string]interface{}{"status": "partial", "has_more": true}); err != nil {
			t.Fatalf("partial path returned err: %v", err)
		}
	})

	t.Run("errors_found yields failed_precondition with count", func(t *testing.T) {
		t.Parallel()
		err := formulaVerifyExitOnError(map[string]interface{}{
			"status":       "errors_found",
			"total_errors": float64(7),
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
		}
		if ve.Subtype != errs.SubtypeFailedPrecondition {
			t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeFailedPrecondition)
		}
		if !strings.Contains(ve.Message, "7 formula error") {
			t.Errorf("message %q must surface the error count", ve.Message)
		}
		if ve.Hint == "" {
			t.Errorf("hint must be set so AI agents know to re-run after fixes")
		}
	})

	t.Run("unknown status maps to internal/invalid_response", func(t *testing.T) {
		t.Parallel()
		err := formulaVerifyExitOnError(map[string]interface{}{"status": "weird"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed problem, got %T %v", err, err)
		}
		if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
			t.Errorf("category/subtype = %q/%q, want internal/invalid_response", p.Category, p.Subtype)
		}
	})

	t.Run("non-object output maps to internal/invalid_response", func(t *testing.T) {
		t.Parallel()
		err := formulaVerifyExitOnError("oops")
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed problem, got %T %v", err, err)
		}
		if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
			t.Errorf("category/subtype = %q/%q, want internal/invalid_response", p.Category, p.Subtype)
		}
	})
}
