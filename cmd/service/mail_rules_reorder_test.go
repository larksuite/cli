// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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
		"risk":       cmdutil.RiskWrite,
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type":     "string",
				"location": "path",
				"required": true,
			},
		},
		"requestBody": map[string]interface{}{
			"rule_ids": map[string]interface{}{"type": "array", "required": true},
		},
	})
}

func newMailRulesReorderCommand(t *testing.T) (*cmdutil.Factory, *httpmock.Registry, *cobraCommandShim) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules", AppSecret: "test-secret-mail-rules", Brand: core.BrandFeishu,
	})
	cmd := NewCmdServiceMethod(f, mailRulesSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	return f, reg, &cobraCommandShim{cmd: cmd, stdout: stdout}
}

type cobraCommandShim struct {
	cmd interface {
		SetArgs([]string)
		Execute() error
	}
	stdout interface {
		Bytes() []byte
		String() string
	}
}

func TestCompleteMailRuleReorderIDs(t *testing.T) {
	tests := []struct {
		name      string
		current   []string
		requested []string
		want      []string
		wantErr   string
		wantParam string
	}{
		{
			name:      "complete input unchanged",
			current:   []string{"A", "B", "C"},
			requested: []string{"C", "A", "B"},
			want:      []string{"C", "A", "B"},
		},
		{
			name:      "single partial input is prepended",
			current:   []string{"A", "B", "C", "D"},
			requested: []string{"C"},
			want:      []string{"C", "A", "B", "D"},
		},
		{
			name:      "multiple partial input keeps requested order",
			current:   []string{"A", "B", "C", "D"},
			requested: []string{"C", "A"},
			want:      []string{"C", "A", "B", "D"},
		},
		{
			name:      "duplicate requested ID",
			current:   []string{"A", "B", "C"},
			requested: []string{"B", "B"},
			wantErr:   "duplicate rule ID",
			wantParam: "rule_ids",
		},
		{
			name:      "unknown requested ID",
			current:   []string{"A", "B", "C"},
			requested: []string{"C", "X"},
			wantErr:   "does not exist",
			wantParam: "rule_ids",
		},
		{
			name:      "empty current list",
			current:   nil,
			requested: []string{"A"},
			wantErr:   "returned no rule IDs",
		},
		{
			name:      "duplicate current list",
			current:   []string{"A", "A"},
			requested: []string{"A"},
			wantErr:   "duplicate rule ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := completeMailRuleReorderIDs(tt.current, tt.requested)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.wantParam != "" {
					requireValidationParam(t, err, tt.wantParam)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("completed IDs = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMailRulesReorderFetchesListAndSubmitsCompletedIDs(t *testing.T) {
	_, reg, shim := newMailRulesReorderCommand(t)
	var calls []string
	var reorderBody map[string]interface{}
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		OnMatch: func(*http.Request) {
			calls = append(calls, "list")
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "A"},
					map[string]interface{}{"id": "B"},
					map[string]interface{}{"id": "C"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		OnMatch: func(req *http.Request) {
			calls = append(calls, "reorder")
			if err := json.NewDecoder(req.Body).Decode(&reorderBody); err != nil {
				t.Fatalf("decode reorder body: %v", err)
			}
		},
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	shim.cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["C"]}`,
	})
	if err := shim.cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"list", "reorder"}) {
		t.Fatalf("calls = %#v, want list then reorder", calls)
	}
	got := stringifyInterfaceSlice(reorderBody["rule_ids"])
	if want := []string{"C", "A", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reorder rule_ids = %#v, want %#v", got, want)
	}
	if !strings.Contains(shim.stdout.String(), `"ok": true`) {
		t.Fatalf("stdout missing success envelope: %s", shim.stdout.String())
	}
}

func TestMailRulesReorderFetchesAllListPages(t *testing.T) {
	_, reg, shim := newMailRulesReorderCommand(t)
	var listTokens []string
	var reorderBody map[string]interface{}
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		OnMatch: func(req *http.Request) {
			listTokens = append(listTokens, req.URL.Query().Get("page_token"))
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "A"}},
				"has_more":   true,
				"page_token": "next-1",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		OnMatch: func(req *http.Request) {
			listTokens = append(listTokens, req.URL.Query().Get("page_token"))
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "B"},
					map[string]interface{}{"id": "C"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		OnMatch: func(req *http.Request) {
			if err := json.NewDecoder(req.Body).Decode(&reorderBody); err != nil {
				t.Fatalf("decode reorder body: %v", err)
			}
		},
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	shim.cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me","page_token":"stale"}`,
		"--data", `{"rule_ids":["B"]}`,
	})
	if err := shim.cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := []string{"", "next-1"}; !reflect.DeepEqual(listTokens, want) {
		t.Fatalf("list page tokens = %#v, want %#v", listTokens, want)
	}
	if got, want := stringifyInterfaceSlice(reorderBody["rule_ids"]), []string{"B", "A", "C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reorder rule_ids = %#v, want %#v", got, want)
	}
}

func TestMailRulesReorderRejectsRepeatedPageToken(t *testing.T) {
	_, reg, shim := newMailRulesReorderCommand(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "A"}},
				"has_more":   true,
				"page_token": "next-1",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "B"}},
				"has_more":   true,
				"page_token": "next-1",
			},
		},
	})

	shim.cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A"]}`,
	})
	err := shim.cmd.Execute()
	if err == nil {
		t.Fatal("expected repeated page token error")
	}
	requireMailRulesProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
}

func TestMailRulesReorderListFailureDoesNotCallReorder(t *testing.T) {
	_, reg, shim := newMailRulesReorderCommand(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body:   map[string]interface{}{"code": 999, "msg": "list failed"},
	})

	shim.cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A"]}`,
	})
	if err := shim.cmd.Execute(); err == nil {
		t.Fatal("expected list failure")
	} else if strings.Contains(err.Error(), "no stub") {
		t.Fatalf("reorder was called after list failure: %v", err)
	}
}

func TestMailRulesReorderValidationFailureDoesNotCallReorder(t *testing.T) {
	_, reg, shim := newMailRulesReorderCommand(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "A"},
					map[string]interface{}{"id": "B"},
				},
			},
		},
	})

	shim.cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A","A"]}`,
	})
	err := shim.cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if strings.Contains(err.Error(), "no stub") {
		t.Fatalf("reorder was called after validation failure: %v", err)
	}
	requireValidationParam(t, err, "rule_ids")
}

func TestMailRulesReorderLocalBodyValidationDoesNotCallListOrReorder(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "missing rule_ids", data: `{}`},
		{name: "empty rule_ids", data: `{"rule_ids":[]}`},
		{name: "empty ID", data: `{"rule_ids":[""]}`},
		{name: "non-string ID", data: `{"rule_ids":[123]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, shim := newMailRulesReorderCommand(t)
			shim.cmd.SetArgs([]string{
				"--as", "bot",
				"--params", `{"user_mailbox_id":"me"}`,
				"--data", tt.data,
			})
			err := shim.cmd.Execute()
			if err == nil {
				t.Fatal("expected local validation error")
			}
			if strings.Contains(err.Error(), "no stub") {
				t.Fatalf("list or reorder was called after local validation failure: %v", err)
			}
			requireValidationParam(t, err, "rule_ids")
		})
	}
}

func TestMailRulesReorderBackendFailureIncludesRetryHint(t *testing.T) {
	_, reg, shim := newMailRulesReorderCommand(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"id": "A"}},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body:   map[string]interface{}{"code": 999, "msg": "rule set changed"},
	})

	shim.cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A"]}`,
	})
	err := shim.cmd.Execute()
	if err == nil {
		t.Fatal("expected reorder API error")
	}
	if problem, ok := errs.ProblemOf(err); !ok || !strings.Contains(problem.Hint, "rules list") {
		t.Fatalf("error hint = %#v, want rules list retry hint", problem)
	}
}

func stringifyInterfaceSlice(value interface{}) []string {
	items, _ := value.([]interface{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.(string))
	}
	return out
}

func requireValidationParam(t *testing.T, err error, wantParam string) {
	t.Helper()
	requireMailRulesProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError: %v", err, err)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("validation param = %q, want %q", validationErr.Param, wantParam)
	}
}

func requireMailRulesProblem(t *testing.T, err error, wantCategory errs.Category, wantSubtype errs.Subtype) *errs.Problem {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf(%T) ok = false; err = %v", err, err)
	}
	if problem.Category != wantCategory {
		t.Fatalf("problem category = %q, want %q", problem.Category, wantCategory)
	}
	if problem.Subtype != wantSubtype {
		t.Fatalf("problem subtype = %q, want %q", problem.Subtype, wantSubtype)
	}
	return problem
}
