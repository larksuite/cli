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

func TestBuildMailRuleReorderIDs_PartialFrontSegment(t *testing.T) {
	got, err := buildMailRuleReorderIDs([]string{"rule_a", "rule_c"}, []mailRuleOrderItem{
		{ID: "rule_a", Index: 0},
		{ID: "rule_b", Index: 1},
		{ID: "rule_c", Index: 2},
		{ID: "rule_d", Index: 3},
	})
	if err != nil {
		t.Fatalf("buildMailRuleReorderIDs() error = %v", err)
	}
	want := []string{"rule_a", "rule_c", "rule_b", "rule_d"}
	if !equalStringSlices(got, want) {
		t.Fatalf("buildMailRuleReorderIDs() = %v, want %v", got, want)
	}
}

func TestBuildMailRuleReorderIDs_FullInputCompatible(t *testing.T) {
	got, err := buildMailRuleReorderIDs([]string{"rule_b", "rule_a", "rule_c"}, []mailRuleOrderItem{
		{ID: "rule_a", Index: 0},
		{ID: "rule_b", Index: 1},
		{ID: "rule_c", Index: 2},
	})
	if err != nil {
		t.Fatalf("buildMailRuleReorderIDs() error = %v", err)
	}
	want := []string{"rule_b", "rule_a", "rule_c"}
	if !equalStringSlices(got, want) {
		t.Fatalf("buildMailRuleReorderIDs() = %v, want %v", got, want)
	}
}

func TestSortMailRulesBySequenceWhenComplete(t *testing.T) {
	seq3, seq1, seq2 := int64(3), int64(1), int64(2)
	rules := []mailRuleOrderItem{
		{ID: "rule_c", Sequence: &seq3, Index: 0},
		{ID: "rule_a", Sequence: &seq1, Index: 1},
		{ID: "rule_b", Sequence: &seq2, Index: 2},
	}
	sortMailRulesBySequenceWhenComplete(rules)
	got := []string{rules[0].ID, rules[1].ID, rules[2].ID}
	want := []string{"rule_a", "rule_b", "rule_c"}
	if !equalStringSlices(got, want) {
		t.Fatalf("sorted IDs = %v, want %v", got, want)
	}
}

func TestMailRuleReorderExecute_SubmitsCompleteRuleIDs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailRuleList(reg, []map[string]interface{}{
		{"id": "rule_a"},
		{"id": "rule_b"},
		{"id": "rule_c"},
		{"id": "rule_d"},
	})
	reorderStub := stubMailRuleReorder(reg, func(body []byte) bool {
		return bodyHasRuleIDs(t, body, []string{"rule_a", "rule_c", "rule_b", "rule_d"})
	}, map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "rule_a,rule_c",
	}, f, stdout)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(reorderStub.CapturedBody) == 0 {
		t.Fatal("expected reorder API to be called")
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	got := interfaceSliceToStrings(data["submitted_rule_ids"].([]interface{}))
	want := []string{"rule_a", "rule_c", "rule_b", "rule_d"}
	if !equalStringSlices(got, want) {
		t.Fatalf("submitted_rule_ids = %v, want %v", got, want)
	}
}

func TestMailRuleReorderExecute_UnknownIDDoesNotCallReorder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailRuleList(reg, []map[string]interface{}{
		{"id": "rule_a"},
		{"id": "rule_b"},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "rule_x",
	}, f, stdout)
	requireMailRuleValidation(t, err, "--rule-ids", "unknown rule ID")
	reg.Verify(t)
}

func TestMailRuleReorderExecute_ValidationErrorsDoNotCallAPIs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty", args: []string{"+rule-reorder"}, want: "at least one rule ID"},
		{name: "duplicate", args: []string{"+rule-reorder", "--rule-ids", "rule_a,rule_a"}, want: "duplicate"},
		{name: "blank element", args: []string{"+rule-reorder", "--rule-ids", "rule_a, "}, want: "empty rule ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailRuleReorder, tt.args, f, stdout)
			requireMailRuleValidation(t, err, "--rule-ids", tt.want)
		})
	}
}

func TestMailRuleReorderExecute_ListFailureDoesNotCallReorder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code":   190001,
			"msg":    "list failed",
			"log_id": "log-list",
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "rule_a",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected list failure")
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("error = %v, want list failure", err)
	}
	reg.Verify(t)
}

func TestMailRuleReorderExecute_ReorderFailureAddsSubmittedIDsHint(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailRuleList(reg, []map[string]interface{}{
		{"id": "rule_a"},
		{"id": "rule_b"},
		{"id": "rule_c"},
	})
	stubMailRuleReorder(reg, func(body []byte) bool {
		return bodyHasRuleIDs(t, body, []string{"rule_b", "rule_a", "rule_c"})
	}, map[string]interface{}{
		"code":   190002,
		"msg":    "rule set changed",
		"log_id": "log-reorder",
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "rule_b",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected reorder failure")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if !strings.Contains(problem.Hint, `submitted_rule_ids=["rule_b","rule_a","rule_c"]`) {
		t.Fatalf("hint = %q, want submitted_rule_ids context", problem.Hint)
	}
}

func stubMailRuleList(reg *httpmock.Registry, rules []map[string]interface{}) *httpmock.Stub {
	items := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		items = append(items, rule)
	}
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"items": items},
		},
	}
	reg.Register(stub)
	return stub
}

func stubMailRuleReorder(reg *httpmock.Registry, filter func([]byte) bool, body map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method:     "POST",
		URL:        "/user_mailboxes/me/rules/reorder",
		Body:       body,
		BodyFilter: filter,
	}
	reg.Register(stub)
	return stub
}

func bodyHasRuleIDs(t *testing.T, body []byte, want []string) bool {
	t.Helper()
	var gotBody map[string]interface{}
	if err := json.Unmarshal(body, &gotBody); err != nil {
		t.Fatalf("request body is not JSON: %v; body=%s", err, string(body))
	}
	rawIDs, ok := gotBody["rule_ids"].([]interface{})
	if !ok {
		t.Fatalf("request body rule_ids = %#v, want array", gotBody["rule_ids"])
	}
	got := interfaceSliceToStrings(rawIDs)
	return equalStringSlices(got, want)
}

func interfaceSliceToStrings(raw []interface{}) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(string))
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func requireMailRuleValidation(t *testing.T, err error, param, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error containing %q, got nil", contains)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != param {
		t.Fatalf("validation Param = %q, want %q", validationErr.Param, param)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("validation error = %v, want substring %q", err, contains)
	}
}
