// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

func TestUndo_DryRun(t *testing.T) {
	t.Parallel()
	if Undo.Risk != "high-risk-write" {
		t.Fatalf("Undo.Risk = %q, want high-risk-write", Undo.Risk)
	}

	args := []string{"--url", testURL, "--as", "user"}
	callURL := dryRunFirstCallURL(t, Undo, args)
	if !containsSuffix(callURL, "invoke_write") {
		t.Errorf("invoke url = %q, want invoke_write", callURL)
	}

	body := parseDryRunBody(t, Undo, args)
	got := decodeToolInput(t, body, "undo_last")
	assertInputEquals(t, got, map[string]interface{}{
		"excel_id": testToken,
		"count":    float64(1),
	})
}

func TestExecute_Undo(t *testing.T) {
	t.Parallel()

	stub := toolOutputStub(testToken, "write", `{"undone":3,"op_id":"op-3","top_doc_revision":30,"new_revision":31,"op_ids":["op-3","op-2","op-1"],"results":[{"op_id":"op-3","top_doc_revision":30,"new_revision":31},{"op_id":"op-2","top_doc_revision":20,"new_revision":32},{"op_id":"op-1","top_doc_revision":10,"new_revision":33}]}`)
	out, err := runShortcutWithStubs(t, Undo, []string{"--url", testURL, "--count", "3", "--as", "user", "--yes"}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}

	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "undo_last")
	assertInputEquals(t, input, map[string]interface{}{
		"excel_id": testToken,
		"count":    float64(3),
	})

	data := decodeEnvelopeData(t, out)
	if data["undone"].(float64) != 3 {
		t.Fatalf("unexpected output data: %#v", data)
	}
	results := data["results"].([]interface{})
	first := results[0].(map[string]interface{})
	if data["op_id"] != first["op_id"] ||
		data["top_doc_revision"] != first["top_doc_revision"] ||
		data["new_revision"] != first["new_revision"] {
		t.Fatalf("legacy fields are not aligned with first result: %#v", data)
	}
}

func TestUndo_HighRiskWriteRequiresYes(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runShortcutCapturingErr(t, Undo, []string{
		"--url", testURL,
		"--as", "user",
	})
	if err == nil {
		t.Fatalf("expected confirmation_required without --yes; stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestUndo_ValidateCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		count   string
		wantSub string
	}{
		{name: "zero", count: "0", wantSub: "--count must be between 1 and 20"},
		{name: "negative", count: "-1", wantSub: "--count must be between 1 and 20"},
		{name: "too large", count: "21", wantSub: "--count must be between 1 and 20"},
		{name: "non integer", count: "1.5", wantSub: "invalid argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := runShortcutWithStubs(t, Undo, []string{"--url", testURL, "--count", tt.count, "--as", "user"})
			if err == nil {
				t.Fatal("expected count validation error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestExecute_UndoReasonOutputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputJSON string
		wantReason string
		wantUndone float64
	}{
		{
			name:       "partial success because requested count exceeds stack",
			outputJSON: `{"undone":2,"reason":"undo_stack_empty","op_ids":["op-2","op-1"],"results":[{"op_id":"op-2"},{"op_id":"op-1"}],"warning_message":"Requested 3 undo step(s), but only 2 active undo entries were available."}`,
			wantReason: "undo_stack_empty",
			wantUndone: 2,
		},
		{
			name:       "raw undo changeset missing or expired",
			outputJSON: `{"undone":0,"reason":"undo_entry_missing","warning_message":"The undo stack entry points to raw undo changeset data that is missing or expired. Use history_list/history_revert to roll the spreadsheet back via history."}`,
			wantReason: "undo_entry_missing",
			wantUndone: 0,
		},
		{
			name:       "unsupported object undo",
			outputJSON: `{"undone":0,"reason":"unsupported_object_undo","warning_message":"The top undo entry contains changes this undo implementation cannot replay."}`,
			wantReason: "unsupported_object_undo",
			wantUndone: 0,
		},
		{
			name:       "apply failed",
			outputJSON: `{"undone":0,"reason":"undo_apply_failed","warning_message":"Failed to apply the top undo entry."}`,
			wantReason: "undo_apply_failed",
			wantUndone: 0,
		},
		{
			name:       "marker persist failed after apply",
			outputJSON: `{"undone":1,"reason":"undo_state_persist_failed","op_ids":["op-1"],"results":[{"op_id":"op-1","reason":"undo_state_persist_failed","warning_message":"Undo was applied, but the marker was not persisted."}]}`,
			wantReason: "undo_state_persist_failed",
			wantUndone: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stub := toolOutputStub(testToken, "write", tt.outputJSON)
			out, err := runShortcutWithStubs(t, Undo, []string{"--url", testURL, "--as", "user", "--yes"}, stub)
			if err != nil {
				t.Fatalf("execute failed: %v\nout=%s", err, out)
			}
			data := decodeEnvelopeData(t, out)
			if data["undone"].(float64) != tt.wantUndone || data["reason"] != tt.wantReason {
				t.Fatalf("unexpected output data: %#v", data)
			}
			if tt.wantReason == "undo_entry_missing" && !strings.Contains(data["warning_message"].(string), "history_list/history_revert") {
				t.Fatalf("missing history fallback hint: %#v", data)
			}
		})
	}
}
