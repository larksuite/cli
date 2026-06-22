// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestReorderRuleIDs(t *testing.T) {
	tests := []struct {
		name       string
		current    []string
		given      []string
		appendMode bool
		want       []string
	}{
		{name: "front fill", current: []string{"A", "B", "C", "D"}, given: []string{"C", "A"}, want: []string{"C", "A", "B", "D"}},
		{name: "append fill", current: []string{"A", "B", "C", "D"}, given: []string{"D", "B"}, appendMode: true, want: []string{"A", "C", "D", "B"}},
		{name: "full same order", current: []string{"A", "B", "C"}, given: []string{"A", "B", "C"}, want: []string{"A", "B", "C"}},
		{name: "full new order", current: []string{"A", "B", "C"}, given: []string{"C", "A", "B"}, want: []string{"C", "A", "B"}},
		{name: "single front", current: []string{"A", "B", "C"}, given: []string{"B"}, want: []string{"B", "A", "C"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderRuleIDs(tt.current, tt.given, tt.appendMode)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("reorderRuleIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstUnknownRuleID(t *testing.T) {
	if got := firstUnknownRuleID([]string{"A", "X"}, []string{"A", "B"}); got != "X" {
		t.Fatalf("firstUnknownRuleID() = %q, want X", got)
	}
}

func TestDiffRuleMoves(t *testing.T) {
	got := diffRuleMoves([]string{"A", "B", "C", "D"}, []string{"C", "A", "B", "D"})
	want := []mailRuleMove{{ID: "C", From: 2, To: 0}, {ID: "A", From: 0, To: 1}, {ID: "B", From: 1, To: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diffRuleMoves() = %#v, want %#v", got, want)
	}
}

func TestMailRuleReorderFrontFill(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", []mailRuleSummary{
		{ID: "A", Name: "rule A"},
		{ID: "B", Name: "rule B"},
		{ID: "C", Name: "rule C"},
		{ID: "D", Name: "rule D"},
	})
	reorderStub := registerRuleReorder(reg, "me", 200)

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "C,A",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	assertStringSlice(t, data["after"], []string{"C", "A", "B", "D"})
	if data["reordered"] != true {
		t.Fatalf("reordered = %v, want true", data["reordered"])
	}
	assertRuleIDsBody(t, reorderStub.CapturedBody, []string{"C", "A", "B", "D"})
}

func TestMailRuleReorderAppendFill(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", []mailRuleSummary{{ID: "A"}, {ID: "B"}, {ID: "C"}, {ID: "D"}})
	reorderStub := registerRuleReorder(reg, "me", 200)

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "D,B", "--append",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	assertStringSlice(t, data["after"], []string{"A", "C", "D", "B"})
	assertRuleIDsBody(t, reorderStub.CapturedBody, []string{"A", "C", "D", "B"})
}

func TestMailRuleReorderFullOrderInput(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", []mailRuleSummary{{ID: "A"}, {ID: "B"}, {ID: "C"}})
	reorderStub := registerRuleReorder(reg, "me", 200)

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "C,A,B",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	assertStringSlice(t, data["after"], []string{"C", "A", "B"})
	assertRuleIDsBody(t, reorderStub.CapturedBody, []string{"C", "A", "B"})
}

func TestMailRuleReorderUnknownID(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", []mailRuleSummary{{ID: "A"}, {ID: "B"}})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "A,X",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
	}
	if msg := err.Error(); !strings.Contains(msg, "not found") || !strings.Contains(msg, "A, B") {
		t.Fatalf("error = %v, want valid ID hint", err)
	}
}

func TestMailRuleReorderEmptyAndDuplicateRuleIDs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing flag", args: []string{"+reorder-rules"}, want: "--rule-ids is required"},
		{name: "empty parsed", args: []string{"+reorder-rules", "--rule-ids", " , "}, want: "required"},
		{name: "duplicate", args: []string{"+reorder-rules", "--rule-ids", "A,B,A"}, want: "duplicate rule id: A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailRuleReorder, tt.args, f, stdout)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %[1]v, want *errs.ValidationError", err)
			}
			if validationErr.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
			}
			if validationErr.Param != "--rule-ids" {
				t.Fatalf("param = %q, want --rule-ids", validationErr.Param)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf(%T) ok = false, want true", err)
			}
			if problem.Category != errs.CategoryValidation {
				t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "required flag") || strings.Contains(err.Error(), "Available Commands:") {
				t.Fatalf("error = %v, want structured validation without cobra usage", err)
			}
		})
	}
}

func TestMailRuleReorderDryRunAvoidsPOST(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", []mailRuleSummary{{ID: "A"}, {ID: "B"}, {ID: "C"}})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "C", "--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal dry-run stdout: %v; stdout=%s", err, stdout.String())
	}
	if out["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", out["dry_run"])
	}
	assertStringSlice(t, out["after"], []string{"C", "A", "B"})
}

func TestMailRuleReorderListFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "list failed",
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "A",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "list rules") {
		t.Fatalf("error = %v, want list context", err)
	}
}

func TestMailRuleReorderReorderFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", []mailRuleSummary{{ID: "A"}, {ID: "B"}})
	registerRuleReorder(reg, "me", 400)

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "B",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); !strings.Contains(msg, "reorder rules") {
		t.Fatalf("error = %v, want reorder context", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(problem.Hint, "+reorder-rules") {
		t.Fatalf("hint = %q, want +reorder-rules", problem.Hint)
	}
}

func TestMailRuleReorderEmptyRuleSetNoop(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerRuleList(t, reg, "me", nil)

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+reorder-rules", "--rule-ids", "A",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["reordered"] != false || data["reason"] != "no rules" {
		t.Fatalf("data = %#v, want no-op", data)
	}
	assertStringSlice(t, data["after"], []string{})
}

func registerRuleList(t *testing.T, reg *httpmock.Registry, mailbox string, rules []mailRuleSummary) {
	t.Helper()
	items := make([]map[string]interface{}, 0, len(rules))
	for _, rule := range rules {
		items = append(items, map[string]interface{}{"id": rule.ID, "name": rule.Name})
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/" + mailbox + "/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": items},
		},
	})
}

func registerRuleReorder(reg *httpmock.Registry, mailbox string, status int) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "open-apis/mail/v1/user_mailboxes/" + mailbox + "/rules/reorder",
		Status: status,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	if status >= 400 {
		stub.Body = map[string]interface{}{"code": 999, "msg": "reorder failed"}
	}
	reg.Register(stub)
	return stub
}

func assertRuleIDsBody(t *testing.T, body []byte, want []string) {
	t.Helper()
	var got struct {
		RuleIDs []string `json:"rule_ids"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", string(body), err)
	}
	if !reflect.DeepEqual(got.RuleIDs, want) {
		t.Fatalf("rule_ids = %v, want %v", got.RuleIDs, want)
	}
}

func assertStringSlice(t *testing.T, value interface{}, want []string) {
	t.Helper()
	gotIface, ok := value.([]interface{})
	if !ok {
		if len(want) == 0 && value == nil {
			return
		}
		t.Fatalf("value = %T(%#v), want []interface{}", value, value)
	}
	got := make([]string, 0, len(gotIface))
	for _, item := range gotIface {
		got = append(got, item.(string))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slice = %v, want %v", got, want)
	}
}
