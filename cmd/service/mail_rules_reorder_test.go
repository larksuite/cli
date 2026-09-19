// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
)

func mailRulesSpec() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "mail",
		"servicePath": "/open-apis/mail/v1",
	})
}

func mailRulesReorderMethod() meta.Method {
	return meta.FromMap(map[string]interface{}{
		"path":       "user_mailboxes/{user_mailbox_id}/rules/reorder",
		"httpMethod": "POST",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{"type": "string", "location": "path", "required": true},
		},
		"requestBody": map[string]interface{}{
			"rule_ids": map[string]interface{}{"type": "array", "required": true},
		},
		"accessTokens": []interface{}{"user"},
	})
}

func mailRulesReorderCommand(f *cmdutil.Factory) *cobraCommandWrapper {
	cmd := NewCmdServiceMethod(f, mailRulesSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	return &cobraCommandWrapper{cmd: cmd}
}

type cobraCommandWrapper struct {
	cmd interface {
		SetArgs([]string)
		Execute() error
	}
}

func (w *cobraCommandWrapper) run(args ...string) error {
	w.cmd.SetArgs(args)
	return w.cmd.Execute()
}

func TestCompleteRuleIDs(t *testing.T) {
	tests := []struct {
		name    string
		user    []string
		current []string
		want    []string
		wantErr string
	}{
		{
			name:    "partial input prepends user order and appends remaining",
			user:    []string{"C", "A"},
			current: []string{"A", "B", "C", "D"},
			want:    []string{"C", "A", "B", "D"},
		},
		{
			name:    "complete input keeps original behavior",
			user:    []string{"A", "B", "C"},
			current: []string{"A", "B", "C"},
			want:    []string{"A", "B", "C"},
		},
		{
			name:    "remaining rules keep server relative order",
			user:    []string{"D"},
			current: []string{"A", "B", "C", "D", "E"},
			want:    []string{"D", "A", "B", "C", "E"},
		},
		{
			name:    "unknown rule id fails",
			user:    []string{"A", "X"},
			current: []string{"A", "B", "C"},
			wantErr: "unknown rule_ids",
		},
		{
			name:    "empty completed list fails",
			user:    []string{},
			current: []string{},
			wantErr: "completed rule_ids must not be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := completeRuleIDs(tt.user, tt.current)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("completeRuleIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUserRuleIDsFromReorderBodyValidation(t *testing.T) {
	tests := []struct {
		name      string
		body      interface{}
		wantErr   string
		wantParam string
	}{
		{name: "missing body", body: nil, wantErr: "--data must be a JSON object", wantParam: "--data"},
		{name: "missing rule ids", body: map[string]interface{}{}, wantErr: "--data.rule_ids is required", wantParam: "rule_ids"},
		{name: "empty rule ids", body: map[string]interface{}{"rule_ids": []interface{}{}}, wantErr: "must not be empty", wantParam: "rule_ids"},
		{name: "non string rule id", body: map[string]interface{}{"rule_ids": []interface{}{1}}, wantErr: "must be a string", wantParam: "rule_ids"},
		{name: "blank rule id", body: map[string]interface{}{"rule_ids": []interface{}{"rule_a", " "}}, wantErr: "must not be blank", wantParam: "rule_ids"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := userRuleIDsFromReorderBody(tt.body)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
			}
			assertValidationProblem(t, err, tt.wantParam)
		})
	}
}

func TestServiceMethod_MailRulesReorderDryRunShowsListThenReorder(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := mailRulesReorderCommand(f)

	err := cmd.run(
		"--as", "user",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule_c","rule_a"]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", got["dry_run"])
	}
	data := got["data"].(map[string]interface{})
	apis := data["api"].([]interface{})
	if len(apis) != 2 {
		t.Fatalf("api calls = %d, want 2: %#v", len(apis), apis)
	}
	first := apis[0].(map[string]interface{})
	second := apis[1].(map[string]interface{})
	if first["method"] != "GET" || first["url"] != "/open-apis/mail/v1/user_mailboxes/me/rules" {
		t.Fatalf("first call = %#v", first)
	}
	if second["method"] != "POST" || second["url"] != "/open-apis/mail/v1/user_mailboxes/me/rules/reorder" {
		t.Fatalf("second call = %#v", second)
	}
	body := second["body"].(map[string]interface{})
	ruleIDs := body["rule_ids"].([]interface{})
	want := []string{"rule_c", "rule_a", "<remaining_rule_ids_in_current_order>"}
	if len(ruleIDs) != len(want) {
		t.Fatalf("dry-run rule_ids len = %d, want %d: %#v", len(ruleIDs), len(want), ruleIDs)
	}
	for i, wantID := range want {
		if ruleIDs[i] != wantID {
			t.Fatalf("dry-run rule_ids[%d] = %v, want %s; all=%#v", i, ruleIDs[i], wantID, ruleIDs)
		}
	}
}

func TestServiceMethod_MailRulesReorderCompletesRuleIDsBeforePost(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
	order := []string{}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		OnMatch: func(*http.Request) {
			order = append(order, "GET")
		},
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"rule_id": "rule_a"},
					map[string]interface{}{"rule_id": "rule_b"},
					map[string]interface{}{"rule_id": "rule_c"},
					map[string]interface{}{"rule_id": "rule_d"},
				},
			},
		},
	})
	postStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		OnMatch: func(*http.Request) {
			order = append(order, "POST")
		},
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}},
	}
	reg.Register(postStub)
	cmd := mailRulesReorderCommand(f)

	err := cmd.run(
		"--as", "user",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule_c","rule_a"]}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(order, ",") != "GET,POST" {
		t.Fatalf("call order = %v, want GET,POST", order)
	}
	var captured map[string]interface{}
	if err := json.Unmarshal(postStub.CapturedBody, &captured); err != nil {
		t.Fatalf("POST body invalid JSON: %v\n%s", err, string(postStub.CapturedBody))
	}
	got := captured["rule_ids"].([]interface{})
	want := []string{"rule_c", "rule_a", "rule_b", "rule_d"}
	if len(got) != len(want) {
		t.Fatalf("rule_ids len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i, wantID := range want {
		if got[i] != wantID {
			t.Fatalf("rule_ids[%d] = %v, want %s; all=%#v", i, got[i], wantID, got)
		}
	}
	if !strings.Contains(stdout.String(), `"ok":true`) && !strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("expected success output, got: %s", stdout.String())
	}
}

func TestServiceMethod_MailRulesReorderPaginatesBeforePost(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"rule_id": "rule_a"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "page_token=next",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"rule_id": "rule_b"}},
			},
		},
	})
	postStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	reg.Register(postStub)
	cmd := mailRulesReorderCommand(f)

	err := cmd.run(
		"--as", "user",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule_b"]}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var captured map[string]interface{}
	if err := json.Unmarshal(postStub.CapturedBody, &captured); err != nil {
		t.Fatalf("POST body invalid JSON: %v", err)
	}
	got := captured["rule_ids"].([]interface{})
	if strings.Join([]string{got[0].(string), got[1].(string)}, ",") != "rule_b,rule_a" {
		t.Fatalf("rule_ids = %#v, want [rule_b rule_a]", got)
	}
}

func TestServiceMethod_MailRulesReorderPageAllDoesNotBypassCompletion(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"rule_id": "rule_a"},
					map[string]interface{}{"rule_id": "rule_b"},
				},
			},
		},
	})
	postStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	reg.Register(postStub)
	cmd := mailRulesReorderCommand(f)

	err := cmd.run(
		"--as", "user",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule_b"]}`,
		"--page-all",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var captured map[string]interface{}
	if err := json.Unmarshal(postStub.CapturedBody, &captured); err != nil {
		t.Fatalf("POST body invalid JSON: %v", err)
	}
	got := captured["rule_ids"].([]interface{})
	if strings.Join([]string{got[0].(string), got[1].(string)}, ",") != "rule_b,rule_a" {
		t.Fatalf("rule_ids = %#v, want [rule_b rule_a]", got)
	}
}

func TestServiceMethod_MailRulesReorderValidationFailuresDoNotPost(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		stubGet bool
		wantErr string
	}{
		{name: "empty input", data: `{"rule_ids":[]}`, wantErr: "must not be empty"},
		{name: "duplicate input", data: `{"rule_ids":["rule_a","rule_a"]}`, wantErr: "duplicate rule_ids"},
		{name: "unknown input", data: `{"rule_ids":["rule_x"]}`, stubGet: true, wantErr: "unknown rule_ids"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, testConfig)
			if tt.stubGet {
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
					Body: map[string]interface{}{
						"code": 0, "msg": "ok",
						"data": map[string]interface{}{
							"items": []interface{}{map[string]interface{}{"rule_id": "rule_a"}},
						},
					},
				})
			}
			postStub := &httpmock.Stub{
				Method:   "POST",
				URL:      "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
				Optional: true,
				OnMatch: func(*http.Request) {
					t.Fatal("reorder POST must not be called on validation failure")
				},
				Body: map[string]interface{}{"code": 0, "msg": "ok"},
			}
			reg.Register(postStub)
			cmd := mailRulesReorderCommand(f)

			err := cmd.run(
				"--as", "user",
				"--params", `{"user_mailbox_id":"me"}`,
				"--data", tt.data,
			)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if validationErr.Category != errs.CategoryValidation || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "rule_ids" {
				t.Fatalf("validation metadata = (%s, %s, %q), want (%s, %s, %q)",
					validationErr.Category, validationErr.Subtype, validationErr.Param,
					errs.CategoryValidation, errs.SubtypeInvalidArgument, "rule_ids")
			}
		})
	}
}

func TestServiceMethod_MailRulesReorderListFailureDoesNotPost(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 230027,
			"msg":  "user not authorized",
		},
	})
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Optional: true,
		OnMatch: func(*http.Request) {
			t.Fatal("reorder POST must not be called when list fails")
		},
		Body: map[string]interface{}{"code": 0, "msg": "ok"},
	})
	cmd := mailRulesReorderCommand(f)

	err := cmd.run(
		"--as", "user",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule_a"]}`,
	)
	if err == nil {
		t.Fatal("expected list API error")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
}

func TestServiceMethod_MailRulesReorderDryRunDoesNotCallAPI(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/mail/v1/user_mailboxes/me/rules",
		Optional: true,
		OnMatch: func(*http.Request) {
			t.Fatal("dry-run must not call list API")
		},
		Body: map[string]interface{}{"code": 0},
	})
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Optional: true,
		OnMatch: func(*http.Request) {
			t.Fatal("dry-run must not call reorder API")
		},
		Body: map[string]interface{}{"code": 0},
	})
	cmd := mailRulesReorderCommand(f)

	err := cmd.run(
		"--as", "user",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule_a"]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchAllMailRuleIDsRejectsMalformedPagination(t *testing.T) {
	ac, _, _, reg := newServicePaginateTestHarness(t)
	ac.ErrOut = io.Discard
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"rule_id": "rule_a"}},
				"has_more": true,
			},
		},
	})

	_, err := fetchAllMailRuleIDs(context.Background(), ac, client.RawApiRequest{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		As:     core.AsBot,
	}, ac.CheckResponse)
	if err == nil || !strings.Contains(err.Error(), "has_more=true") {
		t.Fatalf("error = %v, want malformed pagination error", err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
}

func TestFetchAllMailRuleIDsRejectsRepeatedPageToken(t *testing.T) {
	ac, _, _, reg := newServicePaginateTestHarness(t)
	ac.ErrOut = io.Discard
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"rule_id": "rule_a"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "page_token=next",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"rule_id": "rule_b"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})

	_, err := fetchAllMailRuleIDs(context.Background(), ac, client.RawApiRequest{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		As:     core.AsBot,
	}, ac.CheckResponse)
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("error = %v, want repeated page token error", err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
}

func assertValidationProblem(t *testing.T, err error, wantParam string) {
	t.Helper()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	if validationErr.Category != errs.CategoryValidation || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != wantParam {
		t.Fatalf("validation metadata = (%s, %s, %q), want (%s, %s, %q)",
			validationErr.Category, validationErr.Subtype, validationErr.Param,
			errs.CategoryValidation, errs.SubtypeInvalidArgument, wantParam)
	}
}
