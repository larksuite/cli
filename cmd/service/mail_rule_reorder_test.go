// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
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
	})
}

func newMailRuleReorderCommand(f *cmdutil.Factory) *cobra.Command {
	return NewCmdServiceMethod(f, mailRuleServiceSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
}

func mailRuleListStub(ids ...string) *httpmock.Stub {
	items := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]interface{}{"id": id})
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"items": items},
		},
	}
}

func mailRuleReorderStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"ok": true},
		},
	}
}

func executeMailRuleReorder(t *testing.T, data string, dryRun bool, stubs ...*httpmock.Stub) (*bytes.Buffer, *httpmock.Stub, error) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	for _, stub := range stubs {
		reg.Register(stub)
	}
	cmd := newMailRuleReorderCommand(f)
	args := []string{"--as", "bot", "--params", `{"user_mailbox_id":"me"}`, "--data", data}
	if dryRun {
		args = append(args, "--dry-run")
	}
	cmd.SetArgs(args)
	var last *httpmock.Stub
	if len(stubs) > 0 {
		last = stubs[len(stubs)-1]
	}
	return stdout, last, cmd.Execute()
}

func capturedRuleIDs(t *testing.T, stub *httpmock.Stub) []string {
	t.Helper()
	var body struct {
		RuleIDs []string `json:"rule_ids"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode reorder body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	return body.RuleIDs
}

func dryRunRuleIDs(t *testing.T, stdout string) []string {
	t.Helper()
	const prefix = "=== Dry Run ===\n"
	if len(stdout) <= len(prefix) || stdout[:len(prefix)] != prefix {
		t.Fatalf("unexpected dry-run output:\n%s", stdout)
	}
	var out struct {
		API []struct {
			Body struct {
				RuleIDs []string `json:"rule_ids"`
			} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(stdout[len(prefix):]), &out); err != nil {
		t.Fatalf("decode dry-run JSON: %v\nraw=%s", err, stdout)
	}
	if len(out.API) != 1 {
		t.Fatalf("dry-run api call count = %d, want 1", len(out.API))
	}
	return out.API[0].Body.RuleIDs
}

func requireValidationError(t *testing.T, err error, wantMessage, wantParam string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Category != errs.CategoryValidation {
		t.Fatalf("validation category = %q, want %q", validationErr.Category, errs.CategoryValidation)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("validation subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Message != wantMessage {
		t.Fatalf("validation message = %q, want %q", validationErr.Message, wantMessage)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("validation param = %q, want %q", validationErr.Param, wantParam)
	}
}

func TestMailRuleReorderCompletesPartialRuleIDs(t *testing.T) {
	list := mailRuleListStub("r1", "r2", "r3", "r4")
	reorder := mailRuleReorderStub()
	_, _, err := executeMailRuleReorder(t, `{"rule_ids":["r3","r1"]}`, false, list, reorder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := capturedRuleIDs(t, reorder), []string{"r3", "r1", "r2", "r4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rule_ids = %#v, want %#v", got, want)
	}
}

func TestMailRuleReorderKeepsCompleteRuleIDs(t *testing.T) {
	list := mailRuleListStub("r1", "r2", "r3")
	reorder := mailRuleReorderStub()
	_, _, err := executeMailRuleReorder(t, `{"rule_ids":["r3","r2","r1"]}`, false, list, reorder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := capturedRuleIDs(t, reorder), []string{"r3", "r2", "r1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rule_ids = %#v, want %#v", got, want)
	}
}

func TestMailRuleReorderUnknownIDDoesNotCallReorder(t *testing.T) {
	list := mailRuleListStub("r1", "r2")
	_, _, err := executeMailRuleReorder(t, `{"rule_ids":["r3"]}`, false, list)
	requireValidationError(t, err, `--data.rule_ids contains unknown rule_id "r3"`, "--data.rule_ids")
}

func TestMailRuleReorderDuplicateIDDoesNotCallListOrReorder(t *testing.T) {
	_, _, err := executeMailRuleReorder(t, `{"rule_ids":["r1","r1"]}`, false)
	requireValidationError(t, err, `--data.rule_ids contains duplicate rule_id "r1"`, "--data.rule_ids")
}

func TestMailRuleReorderDryRunListsAndPrintsCompletedBody(t *testing.T) {
	list := mailRuleListStub("r1", "r2", "r3")
	stdout, _, err := executeMailRuleReorder(t, `{"rule_ids":["r3"]}`, true, list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.CapturedBodies) != 1 {
		t.Fatalf("list call count = %d, want 1", len(list.CapturedBodies))
	}
	if got, want := dryRunRuleIDs(t, stdout.String()), []string{"r3", "r1", "r2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dry-run rule_ids = %#v, want %#v", got, want)
	}
}
