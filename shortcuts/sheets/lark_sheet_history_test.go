// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestHistoryShortcuts_DryRun asserts each history shortcut targets the right
// facade-agg tool, routes through the correct read/write invoke endpoint, and
// builds the expected tool input (excel_id always; history_version_id for the
// revert pair).
func TestHistoryShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	const versionID = "histVER123"
	const txnID = "txn-abc-123"

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantPath  string // invoke_read | invoke_write suffix
		wantInput map[string]interface{}
	}{
		{
			name:     "+history-list via --url",
			sc:       HistoryList,
			args:     []string{"--url", testURL},
			toolName: "history_list",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name:     "+history-list via --spreadsheet-token",
			sc:       HistoryList,
			args:     []string{"--spreadsheet-token", testToken},
			toolName: "history_list",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name:     "+history-list paginates with --end-version",
			sc:       HistoryList,
			args:     []string{"--url", testURL, "--end-version", "12345"},
			toolName: "history_list",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id":    testToken,
				"end_version": float64(12345), // post-JSON-unmarshal numeric type
			},
		},
		{
			name:     "+history-revert routes to invoke_write with version id",
			sc:       HistoryRevert,
			args:     []string{"--url", testURL, "--history-version-id", versionID},
			toolName: "history_revert",
			wantPath: "invoke_write",
			wantInput: map[string]interface{}{
				"excel_id":           testToken,
				"history_version_id": versionID,
			},
		},
		{
			name:     "+history-revert-status routes to invoke_read with transaction id",
			sc:       HistoryRevertStatus,
			args:     []string{"--url", testURL, "--transaction-id", txnID},
			toolName: "history_revert_status",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"transaction_id": txnID,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			callURL := dryRunFirstCallURL(t, tt.sc, tt.args)
			if !containsSuffix(callURL, tt.wantPath) {
				t.Errorf("invoke url = %q, want suffix %q", callURL, tt.wantPath)
			}
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestHistoryRevert_MissingRequiredFlag asserts each shortcut rejects a
// missing required selector before any request is sent, with two distinct
// gates by design:
//
//   - +history-revert: --history-version-id is cobra-required (Required=true
//     in the flag def → MarkFlagRequired). cobra refuses the call before
//     Validate runs with a plain "required flag(s)" error; the cmd dispatcher
//     classifies it as a typed *errs.ValidationError (invalid_argument, exit 2).
//     The test rig invokes the shortcut via cmd.Execute and observes the raw
//     cobra error directly (no dispatcher wrap), so we assert the cobra text
//     contract instead of the typed envelope.
//
//   - +history-revert-status: --transaction-id is cobra-optional;
//     requiredness is enforced inside Validate so we still get a typed,
//     flag-tagged *errs.ValidationError with Param="--transaction-id".
func TestHistoryRevert_MissingRequiredFlag(t *testing.T) {
	t.Parallel()

	t.Run(HistoryRevert.Command, func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, HistoryRevert, []string{"--url", testURL})
		if err == nil {
			t.Fatalf("%s: expected error for missing --history-version-id", HistoryRevert.Command)
		}
		msg := err.Error()
		if !strings.Contains(msg, "required flag(s)") || !strings.Contains(msg, "history-version-id") {
			t.Fatalf("%s: cobra error = %q, want substrings 'required flag(s)' and 'history-version-id'", HistoryRevert.Command, msg)
		}
	})

	t.Run(HistoryRevertStatus.Command, func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, HistoryRevertStatus, []string{"--url", testURL})
		if err == nil {
			t.Fatalf("%s: expected error for missing --transaction-id", HistoryRevertStatus.Command)
		}
		msg := err.Error()
		if !strings.Contains(msg, "required flag(s)") || !strings.Contains(msg, "transaction-id") {
			t.Fatalf("%s: cobra error = %q, want substrings 'required flag(s)' and 'transaction-id'", HistoryRevertStatus.Command, msg)
		}
	})
}

func TestHistoryRevert_HighRiskWriteRequiresYes(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, HistoryRevert, []string{
		"--url", testURL,
		"--history-version-id", "histVER123",
	})
	requireProblem(t, err, errs.CategoryConfirmation, errs.SubtypeConfirmationRequired, "")
}

// dryRunFirstCallURL runs the shortcut in --dry-run and returns the first
// api call's url, so tests can assert read vs. write endpoint routing.
func dryRunFirstCallURL(t *testing.T, sc common.Shortcut, args []string) string {
	t.Helper()
	out, err := runShortcut(t, sc, append(args, "--dry-run"))
	if err != nil {
		t.Fatalf("dry-run failed: %v\noutput=%s", err, out)
	}
	dryRun := decodeDryRunRaw(t, out)
	calls, ok := dryRunAPIEntries(dryRun)
	if !ok || len(calls) == 0 {
		t.Fatalf("dry-run api array empty or wrong shape: %#v", dryRun)
	}
	call, _ := calls[0].(map[string]interface{})
	url, _ := call["url"].(string)
	return url
}

func containsSuffix(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
