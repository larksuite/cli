// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
		"path":       "user_mailboxes/{user_mailbox_id}/rules/reorder",
		"httpMethod": "POST",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
		},
		"requestBody": map[string]interface{}{
			"rule_ids": map[string]interface{}{"type": "array", "required": true},
		},
	})
}

func TestMailRulesReorderCompletesPartialRuleIDs(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-a"},
		map[string]interface{}{"rule_id": "rule-b"},
		map[string]interface{}{"rule_id": "rule-c"},
		map[string]interface{}{"rule_id": "rule-d"},
	}))
	reorderStub := mailRulesReorderStubBody(map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}})
	reg.Register(reorderStub)

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-c","rule-a"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	reg.Verify(t)
	got := capturedRuleIDs(t, reorderStub)
	want := []string{"rule-c", "rule-a", "rule-b", "rule-d"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rule_ids = %v, want %v", got, want)
	}
}

func TestMailRulesReorderCompletesEmptyRuleIDs(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-empty", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-a"},
		map[string]interface{}{"rule_id": "rule-b"},
	}))
	reorderStub := mailRulesReorderStubBody(map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}})
	reg.Register(reorderStub)

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":[]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	reg.Verify(t)
	got := capturedRuleIDs(t, reorderStub)
	want := []string{"rule-a", "rule-b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rule_ids = %v, want %v", got, want)
	}
}

func TestMailRulesReorderCompletesUniquePrefix(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-prefix", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-alpha"},
		map[string]interface{}{"rule_id": "rule-beta"},
		map[string]interface{}{"rule_id": "rule-gamma"},
	}))
	reorderStub := mailRulesReorderStubBody(map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}})
	reg.Register(reorderStub)

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-g"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	reg.Verify(t)
	got := capturedRuleIDs(t, reorderStub)
	want := []string{"rule-gamma", "rule-alpha", "rule-beta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rule_ids = %v, want %v", got, want)
	}
}

func TestMailRulesReorderCompletesRuleIDsFromLaterPage(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-paginated", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	registerMailRulesListPageStub(t, reg, "", "next-1", []interface{}{
		map[string]interface{}{"rule_id": "rule-a"},
		map[string]interface{}{"rule_id": "rule-b"},
	})
	registerMailRulesListPageStub(t, reg, "next-1", "", []interface{}{
		map[string]interface{}{"rule_id": "rule-c"},
		map[string]interface{}{"rule_id": "rule-d"},
	})
	reorderStub := mailRulesReorderStubBody(map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}})
	reg.Register(reorderStub)

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-c"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	reg.Verify(t)
	got := capturedRuleIDs(t, reorderStub)
	want := []string{"rule-c", "rule-a", "rule-b", "rule-d"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rule_ids = %v, want %v", got, want)
	}
}

func TestMailRulesReorderKeepsCompleteRuleIDs(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-complete", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-a"},
		map[string]interface{}{"rule_id": "rule-b"},
		map[string]interface{}{"rule_id": "rule-c"},
	}))
	reorderStub := mailRulesReorderStubBody(map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"ok": true}})
	reg.Register(reorderStub)

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-a","rule-b","rule-c"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	reg.Verify(t)
	got := capturedRuleIDs(t, reorderStub)
	want := []string{"rule-a", "rule-b", "rule-c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rule_ids = %v, want %v", got, want)
	}
}

func TestMailRulesReorderDuplicateIDsDoesNotCallHTTP(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-duplicate", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-a","rule-a"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected duplicate ID validation error")
	}
	requireMailRulesProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T, want *errs.ValidationError: %v", err, err)
	}
	if validation.Param != "rule_ids" || !strings.Contains(validation.Message, "rule-a") {
		t.Fatalf("validation = %+v, want duplicate rule-a on rule_ids", validation)
	}
	reg.Verify(t)
}

func TestMailRulesReorderUnknownIDsDoesNotCallReorder(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-unknown", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-a"},
		map[string]interface{}{"rule_id": "rule-b"},
	}))
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-x","rule-a"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown ID validation error")
	}
	requireMailRulesProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T, want *errs.ValidationError: %v", err, err)
	}
	if validation.Param != "rule_ids" || !strings.Contains(validation.Message, "rule-x") {
		t.Fatalf("validation = %+v, want unknown rule-x on rule_ids", validation)
	}
	reg.Verify(t)
}

func TestMailRulesReorderAmbiguousPrefixDoesNotCallReorder(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-ambiguous", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-alpha"},
		map[string]interface{}{"rule_id": "rule-archive"},
	}))
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-a"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous prefix validation error")
	}
	requireMailRulesProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T, want *errs.ValidationError: %v", err, err)
	}
	if validation.Param != "rule_ids" || !strings.Contains(validation.Message, "rule-a") {
		t.Fatalf("validation = %+v, want ambiguous rule-a on rule_ids", validation)
	}
	reg.Verify(t)
}

func TestMailRulesReorderListFailureDoesNotCallReorder(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-list-failure", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body:   map[string]interface{}{"code": 999999, "msg": "list unavailable"},
	})
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-a"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected list failure")
	}
	p := requireMailRulesProblem(t, err, errs.CategoryAPI, errs.SubtypeUnknown)
	if p.Hint == "" || !strings.Contains(p.Hint, "reorder was not called") {
		t.Fatalf("hint = %q, want reorder not called guidance", p.Hint)
	}
	reg.Verify(t)
}

func TestMailRulesReorderBackendFailureIncludesRetryHint(t *testing.T) {
	f, reg := mailRulesReorderFactory(t, &core.CliConfig{
		AppID: "test-mail-rules-reorder-backend-failure", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	reg.Register(mailRulesListStub([]interface{}{
		map[string]interface{}{"rule_id": "rule-a"},
		map[string]interface{}{"rule_id": "rule-b"},
	}))
	reg.Register(mailRulesReorderStubBody(map[string]interface{}{"code": 999998, "msg": "rule set changed"}))

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRulesReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["rule-b"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected reorder backend failure")
	}
	p := requireMailRulesProblem(t, err, errs.CategoryAPI, errs.SubtypeUnknown)
	if p.Hint == "" || !strings.Contains(p.Hint, "list again") {
		t.Fatalf("hint = %q, want list-again retry guidance", p.Hint)
	}
	reg.Verify(t)
}

func TestMailRulesReorderPreservesRuleIDWhitespace(t *testing.T) {
	got, err := stringList([]interface{}{" rule-a "})
	if err != nil {
		t.Fatalf("stringList() error = %v", err)
	}
	if len(got) != 1 || got[0] != " rule-a " {
		t.Fatalf("stringList() = %#v, want exact supplied ID", got)
	}
}

func mailRulesReorderFactory(t *testing.T, config *core.CliConfig) (*cmdutil.Factory, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, config)
	return f, reg
}

func requireMailRulesProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype) *errs.Problem {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != category || p.Subtype != subtype {
		t.Fatalf("problem = %s/%s, want %s/%s", p.Category, p.Subtype, category, subtype)
	}
	return p
}

func mailRulesListStub(items []interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":    items,
				"has_more": false,
			},
		},
	}
}

func registerMailRulesListPageStub(t *testing.T, reg *httpmock.Registry, wantPageToken, nextPageToken string, items []interface{}) {
	t.Helper()
	data := map[string]interface{}{
		"items":    items,
		"has_more": nextPageToken != "",
	}
	if nextPageToken != "" {
		data["page_token"] = nextPageToken
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("page_token"); got != wantPageToken {
				t.Errorf("page_token = %q, want %q", got, wantPageToken)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": data,
		},
	})
}

func mailRulesReorderStubBody(body map[string]interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body:   body,
	}
}

func capturedRuleIDs(t *testing.T, stub *httpmock.Stub) []string {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("captured body is not JSON: %v\n%s", err, string(stub.CapturedBody))
	}
	raw, ok := body["rule_ids"].([]interface{})
	if !ok {
		t.Fatalf("rule_ids = %#v, want array", body["rule_ids"])
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(string))
	}
	return out
}
