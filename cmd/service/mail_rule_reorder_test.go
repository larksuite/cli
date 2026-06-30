// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

func mailRuleServiceSpec() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "mail",
		"servicePath": "/open-apis/mail/v1",
	})
}

func mailRuleReorderMethod() meta.Method {
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

func newMailRuleReorderCommand(t *testing.T, f *cmdutil.Factory) *cobra.Command {
	t.Helper()
	return NewCmdServiceMethod(f, mailRuleServiceSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
}

func registerMailRuleListStub(reg *httpmock.Registry, body interface{}, onMatch func()) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body:   body,
	}
	if onMatch != nil {
		stub.OnMatch = func(_ *http.Request) { onMatch() }
	}
	reg.Register(stub)
	return stub
}

func registerMailRuleReorderStub(reg *httpmock.Registry, onMatch func([]byte)) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"success": true},
		},
	}
	if onMatch != nil {
		stub.OnMatch = func(req *http.Request) {
			payload, _ := io.ReadAll(req.Body)
			onMatch(payload)
		}
	}
	reg.Register(stub)
	return stub
}

func mailRuleListResponse(ids ...string) map[string]interface{} {
	items := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]interface{}{"rule_id": id})
	}
	return map[string]interface{}{
		"code": 0,
		"msg":  "ok",
		"data": map[string]interface{}{"items": items},
	}
}

func runMailRuleReorder(t *testing.T, data string, regFn func(*httpmock.Registry, *[]string, *[]string)) ([]string, []string, error) {
	t.Helper()
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	var calls []string
	var posted []string
	if regFn != nil {
		regFn(reg, &calls, &posted)
	}
	cmd := newMailRuleReorderCommand(t, f)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", data})
	return calls, posted, cmd.Execute()
}

func registerSuccessfulMailRuleFlow(reg *httpmock.Registry, calls *[]string, posted *[]string, listIDs ...string) {
	registerMailRuleListStub(reg, mailRuleListResponse(listIDs...), func() {
		*calls = append(*calls, "GET")
	})
	registerMailRuleReorderStub(reg, func(payload []byte) {
		*calls = append(*calls, "POST")
		var body map[string][]string
		_ = json.Unmarshal(payload, &body)
		*posted = append((*posted)[:0], body[mailRuleIDsParam]...)
	})
}

func TestMailRuleReorder_CompletesPartialRuleIDs(t *testing.T) {
	calls, posted, err := runMailRuleReorder(t, `{"rule_ids":["3"]}`, func(reg *httpmock.Registry, calls, posted *[]string) {
		registerSuccessfulMailRuleFlow(reg, calls, posted, "1", "2", "3")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"GET", "POST"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if want := []string{"3", "1", "2"}; !reflect.DeepEqual(posted, want) {
		t.Fatalf("posted rule_ids = %v, want %v", posted, want)
	}
}

func TestMailRuleReorder_FullRuleIDsUnchanged(t *testing.T) {
	_, posted, err := runMailRuleReorder(t, `{"rule_ids":["1","2","3"]}`, func(reg *httpmock.Registry, calls, posted *[]string) {
		registerSuccessfulMailRuleFlow(reg, calls, posted, "1", "2", "3")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"1", "2", "3"}; !reflect.DeepEqual(posted, want) {
		t.Fatalf("posted rule_ids = %v, want %v", posted, want)
	}
}

func TestMailRuleReorder_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "empty array", data: `{"rule_ids":[]}`, want: "must be non-empty"},
		{name: "non string", data: `{"rule_ids":["1",2]}`, want: "must be a string"},
		{name: "empty string", data: `{"rule_ids":[""]}`, want: "must be non-empty"},
		{name: "duplicate", data: `{"rule_ids":["1","1"]}`, want: "duplicate rule id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runMailRuleReorder(t, tt.data, nil)
			assertMailRuleValidationError(t, err, tt.want)
		})
	}
}

func TestMailRuleReorder_UnknownExplicitID(t *testing.T) {
	calls, _, err := runMailRuleReorder(t, `{"rule_ids":["9"]}`, func(reg *httpmock.Registry, calls, posted *[]string) {
		registerMailRuleListStub(reg, mailRuleListResponse("1", "2"), func() {
			*calls = append(*calls, "GET")
		})
	})
	assertMailRuleValidationError(t, err, "unknown rule id: 9")
	if want := []string{"GET"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestMailRuleReorder_ListAPIFailureStopsBeforePost(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 230027,
			"msg":  "user not authorized",
		},
	})
	cmd := newMailRuleReorderCommand(t, f)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", `{"rule_ids":["1"]}`})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected list API failure")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
}

func TestMailRuleReorder_DryRunShowsTwoStepFlow(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu, UserOpenId: "ou_test",
	})
	cmd := newMailRuleReorderCommand(t, f)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["3"]}`,
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"method": "GET"`,
		`"/open-apis/mail/v1/user_mailboxes/me/rules"`,
		`"method": "POST"`,
		`"/open-apis/mail/v1/user_mailboxes/me/rules/reorder"`,
		mailRuleDryRunRemainingID,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func assertMailRuleValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Category != errs.CategoryValidation || validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %s/%s, want validation/invalid_argument", validationErr.Category, validationErr.Subtype)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}
