// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestCompleteRuleOrder(t *testing.T) {
	current := []mailRuleOrderEntry{{ID: "A"}, {ID: "B"}, {ID: "C"}, {ID: "D"}}
	tests := []struct {
		name    string
		input   []string
		current []mailRuleOrderEntry
		want    []string
		wantErr string
	}{
		{
			name:    "complete input keeps order",
			input:   []string{"D", "C", "B", "A"},
			current: current,
			want:    []string{"D", "C", "B", "A"},
		},
		{
			name:    "partial input prepends selected and appends rest",
			input:   []string{"C", "A"},
			current: current,
			want:    []string{"C", "A", "B", "D"},
		},
		{
			name:    "trim and deduplicate input",
			input:   []string{" B ", "B", "D"},
			current: current,
			want:    []string{"B", "D", "A", "C"},
		},
		{
			name:    "missing input id",
			input:   []string{"X"},
			current: current,
			wantErr: "rule ID not found: X",
		},
		{
			name:    "empty input",
			input:   []string{" ", ""},
			current: current,
			wantErr: "provide at least one rule ID",
		},
		{
			name:    "no rules",
			input:   []string{"A"},
			current: nil,
			wantErr: "current mailbox has no inbox rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := completeRuleOrder(tt.input, tt.current)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("completeRuleOrder() error = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("completeRuleOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMailRuleReorder_SubmitsCompletedOrder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rules": []map[string]interface{}{
					{"rule_id": "A"},
					{"rule_id": "B"},
					{"rule_id": "C"},
					{"rule_id": "D"},
				},
			},
		},
	})
	var submitted []string
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/rules/reorder",
		BodyFilter: func(body []byte) bool {
			var payload map[string][]string
			if err := json.Unmarshal(body, &payload); err != nil {
				return false
			}
			submitted = payload["rule_ids"]
			return strings.Join(submitted, ",") == "C,A,B,D"
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ok": true},
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "C",
		"--rule-ids", "A",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["submitted_count"] != float64(4) {
		t.Fatalf("submitted_count = %v, want 4", data["submitted_count"])
	}
	if strings.Join(submitted, ",") != "C,A,B,D" {
		t.Fatalf("submitted body = %v", submitted)
	}
}

func TestMailRuleReorder_DoesNotSubmitOnMissingID(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rules": []map[string]interface{}{{"rule_id": "A"}},
			},
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "missing",
	}, f, stdout)
	assertRuleReorderValidationError(t, err, "rule ID not found: missing")
	reg.Verify(t)
}

func TestMailRuleReorder_DoesNotListOnEmptyInput(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", " ",
	}, f, stdout)
	assertRuleReorderValidationError(t, err, "provide at least one rule ID")
	reg.Verify(t)
}

func TestMailRuleReorder_DoesNotSubmitWhenNoRules(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"rules": []map[string]interface{}{}},
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "A",
	}, f, stdout)
	assertRuleReorderValidationError(t, err, "current mailbox has no inbox rules")
	reg.Verify(t)
}

func assertRuleReorderValidationError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(validationErr.Error(), wantSubstr) {
		t.Fatalf("error = %v, want substring %q", validationErr, wantSubstr)
	}
}
