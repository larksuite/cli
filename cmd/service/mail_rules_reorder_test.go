// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
)

func mailSpec() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "mail",
		"servicePath": "/open-apis/mail/v1",
	})
}

func mailRulesReorderMethod() meta.Method {
	return meta.FromMap(map[string]interface{}{
		"path":       "user_mailboxes/{user_mailbox_id}/rules",
		"httpMethod": "PATCH",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{"type": "string", "location": "path", "required": true},
		},
		"requestBody": map[string]interface{}{
			"rule_ids": map[string]interface{}{"type": "list", "required": true},
		},
	})
}

func mailRulesReorderSubpathMethod() meta.Method {
	return meta.FromMap(map[string]interface{}{
		"path":       "user_mailboxes/{user_mailbox_id}/rules/reorder",
		"httpMethod": "PATCH",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{"type": "string", "location": "path", "required": true},
		},
		"requestBody": map[string]interface{}{
			"rule_ids": map[string]interface{}{"type": "list", "required": true},
		},
	})
}

func newMailRulesReorderCommand(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry, *cobraCommandShim) {
	t.Helper()
	f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	return f, stdout, reg, &cobraCommandShim{setArgs: cmd.SetArgs, execute: cmd.Execute}
}

type cobraCommandShim struct {
	setArgs func([]string)
	execute func() error
}

func registerMailRulesListPage(reg *httpmock.Registry, mailboxID string, body map[string]interface{}, onMatch func(*http.Request)) {
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/mail/v1/user_mailboxes/" + mailboxID + "/rules",
		OnMatch: onMatch,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": body,
		},
	})
}

func registerMailRulesReorder(reg *httpmock.Registry, mailboxID string, onMatch func(*http.Request, map[string]interface{}), body map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/mail/v1/user_mailboxes/" + mailboxID + "/rules",
		OnMatch: func(req *http.Request) {
			var got map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
				panic(err)
			}
			onMatch(req, got)
		},
		Body: body,
	})
}

func TestMailRulesReorder_CompletesPartialIDsBeforeReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	var listMailbox, reorderMailbox string
	var gotRuleIDs []string
	registerMailRulesListPage(reg, "shared@example.com", map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"rule_id": "r1"},
			map[string]interface{}{"rule_id": "r2"},
			map[string]interface{}{"rule_id": "r3"},
		},
		"has_more": false,
	}, func(req *http.Request) {
		listMailbox = req.URL.Path
	})
	registerMailRulesReorder(reg, "shared@example.com", func(req *http.Request, got map[string]interface{}) {
		reorderMailbox = req.URL.Path
		gotRuleIDs = interfaceSliceToStrings(t, got["rule_ids"])
	}, map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}})

	cmd.setArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"shared@example.com"}`,
		"--data", `{"rule_ids":["r2"]}`,
	})
	if err := cmd.execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotRuleIDs, []string{"r2", "r1", "r3"}) {
		t.Fatalf("rule_ids = %#v, want [r2 r1 r3]", gotRuleIDs)
	}
	if listMailbox != reorderMailbox {
		t.Fatalf("list path = %q, reorder path = %q; want same mailbox context", listMailbox, reorderMailbox)
	}
}

func TestMailRulesReorder_FullIDsRemainInRequestedOrder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	var gotRuleIDs []string
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"rule_id": "r1"},
			map[string]interface{}{"rule_id": "r2"},
			map[string]interface{}{"rule_id": "r3"},
		},
		"has_more": false,
	}, nil)
	registerMailRulesReorder(reg, "me", func(req *http.Request, got map[string]interface{}) {
		gotRuleIDs = interfaceSliceToStrings(t, got["rule_ids"])
	}, map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}})

	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r3","r1","r2"]}`})
	if err := cmd.execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotRuleIDs, []string{"r3", "r1", "r2"}) {
		t.Fatalf("rule_ids = %#v, want [r3 r1 r2]", gotRuleIDs)
	}
}

func TestMailRulesReorder_DryRunDoesNotFetchRules(t *testing.T) {
	_, stdout, _, cmd := newMailRulesReorderCommand(t)
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r2"]}`, "--dry-run"})
	if err := cmd.execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var dryRun map[string]any
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(stdout.String(), "=== Dry Run ===\n")))
	if err := decoder.Decode(&dryRun); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	calls, ok := dryRunAPICalls(dryRun)
	if !ok || len(calls) != 1 {
		t.Fatalf("dry-run api = %#v, want one call", dryRun)
	}
	call, ok := calls[0].(map[string]any)
	if !ok {
		t.Fatalf("dry-run api[0] = %#v, want object", calls[0])
	}
	body, ok := call["body"].(map[string]any)
	if !ok {
		t.Fatalf("dry-run body = %#v, want object", call["body"])
	}
	if gotIDs := interfaceSliceToStrings(t, body["rule_ids"]); !reflect.DeepEqual(gotIDs, []string{"r2"}) {
		t.Fatalf("dry-run rule_ids = %#v, want [r2]", gotIDs)
	}
}

func dryRunAPICalls(dryRun map[string]any) ([]any, bool) {
	if calls, ok := dryRun["api"].([]any); ok {
		return calls, true
	}
	data, ok := dryRun["data"].(map[string]any)
	if !ok {
		return nil, false
	}
	calls, ok := data["api"].([]any)
	return calls, ok
}

func TestMailRulesReorder_ValidationErrorsDoNotCallAPIs(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantMsg string
		param   string
	}{
		{name: "non object body", data: `["r1"]`, wantMsg: "requires a JSON object body", param: "--data"},
		{name: "empty", data: `{"rule_ids":[]}`, wantMsg: "at least one"},
		{name: "not array", data: `{"rule_ids":"r1"}`, wantMsg: "rule_ids must be an array"},
		{name: "item not string", data: `{"rule_ids":["r1",1]}`, wantMsg: "rule_ids[1] must be a string"},
		{name: "duplicate", data: `{"rule_ids":["r1","r1"]}`, wantMsg: "duplicate rule_id: r1"},
		{name: "empty string", data: `{"rule_ids":[""]}`, wantMsg: "empty string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, cmd := newMailRulesReorderCommand(t)
			cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", tt.data})
			err := cmd.execute()
			wantParam := tt.param
			if wantParam == "" {
				wantParam = "rule_ids"
			}
			assertServiceValidationErrorWithParam(t, err, tt.wantMsg, wantParam)
		})
	}
}

func TestMailRulesReorder_UnknownIDDoesNotCallReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"rule_id": "known"}},
	}, nil)
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["missing"]}`})

	err := cmd.execute()
	assertServiceValidationError(t, err, "unknown rule_id: missing")
}

func TestMailRulesReorder_EmptyRuleListReturnsValidationError(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"has_more": false,
	}, nil)
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	assertServiceValidationError(t, err, "unknown rule_id: r1")
}

func TestMailRulesReorder_ListFailureDoesNotCallReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "list failed",
		},
	})
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	assertServiceAPIError(t, err, 999, "list failed")
}

func TestMailRulesReorder_ListLaterPageFailureDoesNotCallReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":      []interface{}{map[string]interface{}{"rule_id": "r1"}},
		"has_more":   true,
		"page_token": "next-1",
	}, nil)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules?page_token=next-1",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "second page failed",
		},
	})
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	assertServiceAPIError(t, err, 999, "second page failed")
}

func TestMailRulesReorder_ListMissingNextPageTokenDoesNotCallReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":    []interface{}{map[string]interface{}{"rule_id": "r1"}},
		"has_more": true,
	}, nil)
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected internal error, got %T: %v", err, err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
	if !strings.Contains(err.Error(), "missing next page token") {
		t.Fatalf("internal error = %q, want missing next page token", err.Error())
	}
}

func TestMailRulesReorder_ListRepeatedPageTokenDoesNotCallReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":      []interface{}{map[string]interface{}{"rule_id": "r1"}},
		"has_more":   true,
		"page_token": "next-1",
	}, nil)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":      []interface{}{map[string]interface{}{"rule_id": "r2"}},
		"has_more":   true,
		"page_token": "next-1",
	}, nil)
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected internal error, got %T: %v", err, err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
	if !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("internal error = %q, want repeated page token", err.Error())
	}
}

func TestMailRulesReorder_ListPageTokenCycleDoesNotCallReorder(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":      []interface{}{map[string]interface{}{"rule_id": "r1"}},
		"has_more":   true,
		"page_token": "next-1",
	}, nil)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules?page_token=next-1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"rule_id": "r2"}},
				"has_more":   true,
				"page_token": "next-2",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules?page_token=next-2",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"rule_id": "r3"}},
				"has_more":   true,
				"page_token": "next-1",
			},
		},
	})
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected internal error, got %T: %v", err, err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
	if !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("internal error = %q, want repeated page token", err.Error())
	}
}

func TestMailRulesReorder_ReorderFailureIsSurfaced(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"rule_id": "r1"}},
	}, nil)
	registerMailRulesReorder(reg, "me", func(req *http.Request, got map[string]interface{}) {}, map[string]interface{}{
		"code": 998,
		"msg":  "reorder failed",
	})
	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})

	err := cmd.execute()
	assertServiceAPIError(t, err, 998, "reorder failed")
}

func TestMailRulesReorder_ListPaginationFetchesAllRules(t *testing.T) {
	_, _, reg, cmd := newMailRulesReorderCommand(t)
	var tokens []string
	var gotRuleIDs []string
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":      []interface{}{map[string]interface{}{"rule_id": "r1"}},
		"has_more":   true,
		"page_token": "next-1",
	}, func(req *http.Request) {
		tokens = append(tokens, req.URL.Query().Get("page_token"))
	})
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items":    []interface{}{map[string]interface{}{"rule_id": "r2"}},
		"has_more": false,
	}, func(req *http.Request) {
		tokens = append(tokens, req.URL.Query().Get("page_token"))
	})
	registerMailRulesReorder(reg, "me", func(req *http.Request, got map[string]interface{}) {
		gotRuleIDs = interfaceSliceToStrings(t, got["rule_ids"])
	}, map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}})

	cmd.setArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r2"]}`})
	if err := cmd.execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotRuleIDs, []string{"r2", "r1"}) {
		t.Fatalf("rule_ids = %#v, want [r2 r1]", gotRuleIDs)
	}
	if !reflect.DeepEqual(tokens, []string{"", "next-1"}) {
		t.Fatalf("page tokens = %v, want [ next-1]", tokens)
	}
}

func TestMailRulesReorder_ListUsesRulesBaseWhenReorderHasSubpath(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderSubpathMethod(), "reorder", "user_mailbox.rules", nil)
	var listed, reordered bool
	registerMailRulesListPage(reg, "me", map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"rule_id": "r1"}},
	}, func(req *http.Request) {
		listed = true
	})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		OnMatch: func(req *http.Request) {
			reordered = true
		},
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	cmd.SetArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["r1"]}`})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !listed || !reordered {
		t.Fatalf("listed=%v reordered=%v, want both true", listed, reordered)
	}
}

func assertServiceValidationError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	assertServiceValidationErrorWithParam(t, err, wantSubstr, "rule_ids")
}

func assertServiceValidationErrorWithParam(t *testing.T, err error, wantSubstr, wantParam string) {
	t.Helper()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, 0)
	if validationErr.Param != wantParam {
		t.Fatalf("validation error param = %q, want %q", validationErr.Param, wantParam)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("validation error = %q, want substring %q", err.Error(), wantSubstr)
	}
}

func assertServiceAPIError(t *testing.T, err error, wantCode int, wantSubstr string) {
	t.Helper()
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected API error, got %T: %v", err, err)
	}
	requireProblem(t, err, errs.CategoryAPI, errs.SubtypeUnknown, wantCode)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("api error = %q, want substring %q", err.Error(), wantSubstr)
	}
}

func interfaceSliceToStrings(t *testing.T, v interface{}) []string {
	t.Helper()
	items, ok := v.([]interface{})
	if !ok {
		t.Fatalf("rule_ids = %#v, want []interface{}", v)
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("rule_ids[%d] = %#v, want string", i, item)
		}
		out = append(out, s)
	}
	return out
}
