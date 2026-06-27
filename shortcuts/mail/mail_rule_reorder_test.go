// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestParseRuleIDsInput(t *testing.T) {
	got, err := parseRuleIDsInput("3, 1  9")
	if err != nil {
		t.Fatalf("parseRuleIDsInput returned error: %v", err)
	}
	want := []string{"3", "1", "9"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parseRuleIDsInput = %v, want %v", got, want)
	}
}

func TestParseRuleIDsInputRejectsDuplicate(t *testing.T) {
	_, err := parseRuleIDsInput("3,1,3")
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate validation error, got %v", err)
	}
}

func TestBuildCompletedRuleOrder(t *testing.T) {
	current := []mailboxRule{
		{ID: "1", Name: "first"},
		{ID: "2", Name: "second"},
		{ID: "3", Name: "third"},
		{ID: "4", Name: "fourth"},
	}
	ids, reordered, err := buildCompletedRuleOrder([]string{"3", "1"}, current)
	if err != nil {
		t.Fatalf("buildCompletedRuleOrder returned error: %v", err)
	}
	if got, want := strings.Join(ids, ","), "3,1,2,4"; got != want {
		t.Fatalf("completed ids = %s, want %s", got, want)
	}
	if got, want := reordered[0].ID+","+reordered[1].ID+","+reordered[2].ID+","+reordered[3].ID, "3,1,2,4"; got != want {
		t.Fatalf("reordered ids = %s, want %s", got, want)
	}
}

func TestMailRuleReorderDryRunListsAndReorders(t *testing.T) {
	runtime := runtimeForMailRuleReorderDryRun(t, map[string]string{
		"mailbox":  "me",
		"rule-ids": "3,1",
	})
	apis := dryRunAPIsForMailRuleReorderTest(t, MailRuleReorder.DryRun(context.Background(), runtime))
	if len(apis) != 2 {
		t.Fatalf("expected 2 API calls in dry-run, got %d", len(apis))
	}
	if apis[0].Method != "GET" || apis[0].URL != mailboxPath("me", "rules") {
		t.Fatalf("first dry-run API = %+v, want GET %s", apis[0], mailboxPath("me", "rules"))
	}
	if apis[1].Method != "POST" || apis[1].URL != mailboxPath("me", "rules", "reorder") {
		t.Fatalf("second dry-run API = %+v, want POST %s", apis[1], mailboxPath("me", "rules", "reorder"))
	}
}

func TestMailRuleReorderExecuteCompletesMissingIDs(t *testing.T) {
	f, stdout, _, reg := mailRuleReorderTestFactory(t)
	registerMailboxRulesListStub(reg, "me", []map[string]interface{}{
		{"id": float64(1), "name": "rule-1", "is_enable": true},
		{"id": float64(2), "name": "rule-2", "is_enable": true},
		{"id": float64(3), "name": "rule-3", "is_enable": false},
		{"id": float64(4), "name": "rule-4", "is_enable": true},
	})
	reorderStub := registerMailboxRulesReorderStub(reg, "me")

	if err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--rule-ids", "3,1"}, f, stdout); err != nil {
		t.Fatalf("runMountedMailShortcut returned error: %v", err)
	}

	var body struct {
		RuleIDs []string `json:"rule_ids"`
	}
	if err := json.Unmarshal(reorderStub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal reorder body: %v", err)
	}
	if got, want := strings.Join(body.RuleIDs, ","), "3,1,2,4"; got != want {
		t.Fatalf("reorder body rule_ids = %s, want %s", got, want)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if got, want := data["mailbox"], "me"; got != want {
		t.Fatalf("mailbox = %v, want %v", got, want)
	}
	if got := data["dry_run"]; got != false {
		t.Fatalf("dry_run = %v, want false", got)
	}
	after, ok := data["after"].([]interface{})
	if !ok || len(after) != 4 {
		t.Fatalf("after = %#v, want 4 entries", data["after"])
	}
}

func TestMailRuleReorderExecuteRejectsUnknownRule(t *testing.T) {
	f, stdout, _, reg := mailRuleReorderTestFactory(t)
	registerMailboxRulesListStub(reg, "me", []map[string]interface{}{
		{"id": "1", "name": "rule-1"},
		{"id": "2", "name": "rule-2"},
	})
	err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--rule-ids", "7,1"}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found validation error, got %v", err)
	}
}

type ruleReorderDryRunPayload struct {
	API []struct {
		Method string                 `json:"method"`
		URL    string                 `json:"url"`
		Body   map[string]interface{} `json:"body"`
	} `json:"api"`
}

func runtimeForMailRuleReorderDryRun(t *testing.T, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, fl := range MailRuleReorder.Flags {
		switch fl.Type {
		case "bool":
			cmd.Flags().Bool(fl.Name, fl.Default == "true", "")
		case "int":
			cmd.Flags().Int(fl.Name, 0, "")
		default:
			cmd.Flags().String(fl.Name, fl.Default, "")
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range values {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag --%s failed: %v", k, err)
		}
	}
	return &common.RuntimeContext{
		Cmd:    cmd,
		Config: &core.CliConfig{AppID: "cli_test_app"},
	}
}

func dryRunAPIsForMailRuleReorderTest(t *testing.T, dry *common.DryRunAPI) []struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Body   map[string]interface{} `json:"body"`
} {
	t.Helper()
	var payload ruleReorderDryRunPayload
	b, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal dry-run failed: %v\njson=%s", err, string(b))
	}
	return payload.API
}

func mailRuleReorderTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	return f, stdout, stderr, reg
}

func registerMailboxRulesListStub(reg *httpmock.Registry, mailbox string, items []map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailbox, "rules"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": items,
			},
		},
	})
}

func registerMailboxRulesReorderStub(reg *httpmock.Registry, mailbox string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    mailboxPath(mailbox, "rules", "reorder"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)
	return stub
}
