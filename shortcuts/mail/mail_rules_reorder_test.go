// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestCompleteMailRuleOrder(t *testing.T) {
	cases := []struct {
		name      string
		requested []string
		current   []string
		want      []string
	}{
		{"complete input stays ordered", []string{"c", "a", "b"}, []string{"a", "b", "c"}, []string{"c", "a", "b"}},
		{"partial input appends missing", []string{"b"}, []string{"a", "b", "c"}, []string{"b", "a", "c"}},
		{"unordered subset keeps explicit order", []string{"c", "a"}, []string{"a", "b", "c", "d"}, []string{"c", "a", "b", "d"}},
		{"duplicates keep first occurrence", []string{"b", "b", "a"}, []string{"a", "b", "c"}, []string{"b", "a", "c"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := completeMailRuleOrder(tt.requested, tt.current)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestCompleteMailRuleOrderRejectsUnknownID(t *testing.T) {
	_, err := completeMailRuleOrder([]string{"missing"}, []string{"a"})
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--rule-ids" {
		t.Fatalf("expected rule-ids validation error, got %v", err)
	}
}

func TestMailRulesReorderExecutesCompleteOrder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	list := &httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/rules", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"rule_id": "a"}, map[string]interface{}{"rule_id": "b"}, map[string]interface{}{"rule_id": "c"}}, "has_more": false}}}
	reorder := &httpmock.Stub{Method: "POST", URL: "/user_mailboxes/me/rules/reorder", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}}, BodyFilter: func(body []byte) bool {
		var got map[string][]string
		return json.Unmarshal(body, &got) == nil && len(got["rule_ids"]) == 3 && got["rule_ids"][0] == "c" && got["rule_ids"][1] == "a" && got["rule_ids"][2] == "b"
	}}
	reg.Register(list)
	reg.Register(reorder)
	if err := runMountedMailShortcut(t, MailRulesReorder, []string{"+rules-reorder", "--rule-ids", "c,a,c"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	reg.Verify(t)
	data := decodeShortcutEnvelopeData(t, stdout)
	if got := data["rule_ids"].([]interface{}); len(got) != 3 || got[0] != "c" {
		t.Fatalf("output rule_ids = %#v", data["rule_ids"])
	}
}

func TestMailRulesReorderRejectsUnknownWithoutWrite(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	list := &httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/rules", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"rule_id": "a"}}, "has_more": false}}}
	reg.Register(list)
	err := runMountedMailShortcut(t, MailRulesReorder, []string{"+rules-reorder", "--rule-ids", "missing"}, f, stdout)
	if err == nil {
		t.Fatal("expected unknown-ID error")
	}
	reg.Verify(t)
}

func TestMailRulesReorderListFailureDoesNotWrite(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	list := &httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/rules", Status: 500, Body: map[string]interface{}{"code": 99991663, "msg": "list failed"}}
	reg.Register(list)
	if err := runMountedMailShortcut(t, MailRulesReorder, []string{"+rules-reorder", "--rule-ids", "a"}, f, stdout); err == nil {
		t.Fatal("expected list failure")
	}
	reg.Verify(t)
}

func TestMailRulesReorderReturnsWriteFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	list := &httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/rules", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"rule_id": "a"}}, "has_more": false}}}
	reorder := &httpmock.Stub{Method: "POST", URL: "/user_mailboxes/me/rules/reorder", Status: 500, Body: map[string]interface{}{"code": 99991664, "msg": "rules changed"}}
	reg.Register(list)
	reg.Register(reorder)
	if err := runMountedMailShortcut(t, MailRulesReorder, []string{"+rules-reorder", "--rule-ids", "a"}, f, stdout); err == nil {
		t.Fatal("expected reorder failure")
	}
	reg.Verify(t)
}

func TestMailRulesReorderRejectsEmptyInputWithoutRead(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailRulesReorder, []string{"+rules-reorder"}, f, stdout)
	if err == nil {
		t.Fatal("expected empty-input error")
	}
	reg.Verify(t)
}

func TestMailRulesReorderPaginatesBeforeWriting(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	first := &httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/rules?page_size=100", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"rule_id": "a"}}, "has_more": true, "page_token": "next"}}}
	second := &httpmock.Stub{Method: "GET", URL: "page_token=next", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"rule_id": "b"}}, "has_more": false}}}
	reorder := &httpmock.Stub{Method: "POST", URL: "/user_mailboxes/me/rules/reorder", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}}, BodyFilter: func(body []byte) bool { return string(body) == `{"rule_ids":["b","a"]}` }}
	reg.Register(first)
	reg.Register(second)
	reg.Register(reorder)
	if err := runMountedMailShortcut(t, MailRulesReorder, []string{"+rules-reorder", "--rule-ids", "b"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	reg.Verify(t)
}
