// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailSenderShortcuts_MetadataScopesAndRegistration(t *testing.T) {
	if MailAllowSendersList.Command != "+allow-senders-list" {
		t.Fatalf("allow list command = %q", MailAllowSendersList.Command)
	}
	if MailBlockedSendersRemove.Command != "+blocked-senders-remove" {
		t.Fatalf("blocked remove command = %q", MailBlockedSendersRemove.Command)
	}
	for _, sc := range []struct {
		name      string
		risk      string
		authTypes []string
		scopes    []string
		wantScope string
		wantRisk  string
	}{
		{"allow list", MailAllowSendersList.Risk, MailAllowSendersList.AuthTypes, MailAllowSendersList.Scopes, "mail:user_mailbox.message:readonly", "read"},
		{"blocked list", MailBlockedSendersList.Risk, MailBlockedSendersList.AuthTypes, MailBlockedSendersList.Scopes, "mail:user_mailbox.message:readonly", "read"},
		{"allow add", MailAllowSendersAdd.Risk, MailAllowSendersAdd.AuthTypes, MailAllowSendersAdd.Scopes, "mail:user_mailbox.message:modify", "write"},
		{"allow remove", MailAllowSendersRemove.Risk, MailAllowSendersRemove.AuthTypes, MailAllowSendersRemove.Scopes, "mail:user_mailbox.message:modify", "write"},
		{"blocked add", MailBlockedSendersAdd.Risk, MailBlockedSendersAdd.AuthTypes, MailBlockedSendersAdd.Scopes, "mail:user_mailbox.message:modify", "write"},
		{"blocked remove", MailBlockedSendersRemove.Risk, MailBlockedSendersRemove.AuthTypes, MailBlockedSendersRemove.Scopes, "mail:user_mailbox.message:modify", "write"},
	} {
		t.Run(sc.name, func(t *testing.T) {
			if sc.risk != sc.wantRisk {
				t.Fatalf("Risk = %q, want %q", sc.risk, sc.wantRisk)
			}
			if len(sc.authTypes) != 1 || sc.authTypes[0] != "user" {
				t.Fatalf("AuthTypes = %v, want [user]", sc.authTypes)
			}
			if len(sc.scopes) != 1 || sc.scopes[0] != sc.wantScope {
				t.Fatalf("Scopes = %v, want [%s]", sc.scopes, sc.wantScope)
			}
		})
	}
	registered := map[string]bool{}
	for _, sc := range Shortcuts() {
		registered[sc.Command] = true
	}
	for _, command := range []string{"+allow-senders-list", "+allow-senders-add", "+allow-senders-remove", "+blocked-senders-list", "+blocked-senders-add", "+blocked-senders-remove"} {
		if !registered[command] {
			t.Fatalf("shortcut %s is not registered", command)
		}
	}
}

func TestMailSenderList_PaginationAndKeywordUseUserMailboxPath(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/team@example.com/allow_senders?keyword=market&page_size=50&page_token=tok_1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":    []map[string]interface{}{{"sender": "marketing@example.com", "create_time": "1710000000"}},
			"has_more": true, "page_token": "tok_2",
		}},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailAllowSendersList, []string{
		"+allow-senders-list",
		"--mailbox", "team@example.com",
		"--keyword", "market",
		"--page-size", "50",
		"--page-token", "tok_1",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["list"] != "allow" || data["mailbox_id"] != "team@example.com" {
		t.Fatalf("unexpected output data: %#v", data)
	}
	items := data["items"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["sender"] != "marketing@example.com" {
		t.Fatalf("items = %#v", items)
	}
}

func TestMailSenderAdd_NormalizesAndReportsPartialFailures(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_create",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"failed_items": []map[string]interface{}{{"sender": "bad-format", "reason_code": "INVALID"}},
		}},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailBlockedSendersAdd, []string{
		"+blocked-senders-add",
		"--sender", "Alice@Example.COM,example.org",
		"--sender", "alice@example.com",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	items := body["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items = %#v, want two deduped entries", items)
	}
	first := items[0].(map[string]interface{})
	if first["sender"] != "alice@example.com" || first["sender_type"].(float64) != mailSenderTypeAddress {
		t.Fatalf("first item = %#v", first)
	}
	second := items[1].(map[string]interface{})
	if second["sender"] != "example.org" || second["sender_type"].(float64) != mailSenderTypeDomain {
		t.Fatalf("second item = %#v", second)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	failed := data["failed_items"].([]interface{})
	if len(failed) != 1 || failed[0].(map[string]interface{})["reason_code"] != "INVALID" {
		t.Fatalf("failed_items = %#v", failed)
	}
}

func TestMailSenderRemove_PreservesMixedCaseSenderValues(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/allow_senders/batch_remove",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"deleted_count": 2}},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailAllowSendersRemove, []string{
		"+allow-senders-remove",
		"--sender", "Alice@Example.COM,example.org",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	senders := body["senders"].([]interface{})
	if len(senders) != 2 || senders[0] != "Alice@Example.COM" || senders[1] != "example.org" {
		t.Fatalf("senders = %#v, want original mixed-case value preserved", senders)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["deleted_count"].(float64) != 2 {
		t.Fatalf("deleted_count = %#v", data["deleted_count"])
	}
}

func TestMailSenderList_ReadonlyScopePreflight(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	token, err := auth.GetStoredToken("test-app", "ou_testuser")
	if err != nil {
		t.Fatalf("GetStoredToken() error = %v", err)
	}
	token.Scope = "mail:user_mailbox.message:readonly"
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/blocked_senders?page_size=20",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}, "has_more": false}},
	})

	err = runMountedMailShortcut(t, MailBlockedSendersList, []string{"+blocked-senders-list", "--format", "json"}, f, stdout)
	if err != nil {
		t.Fatalf("list with readonly scope should pass: %v", err)
	}
}

func TestMailSenderAdd_ModifyScopePreflight(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	token, err := auth.GetStoredToken("test-app", "ou_testuser")
	if err != nil {
		t.Fatalf("GetStoredToken() error = %v", err)
	}
	token.Scope = "mail:user_mailbox.message:readonly"
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	err = runMountedMailShortcut(t, MailAllowSendersAdd, []string{"+allow-senders-add", "--sender", "a@example.com"}, f, stdout)
	if err == nil {
		t.Fatal("expected scope preflight error, got nil")
	}
	if !strings.Contains(err.Error(), "mail:user_mailbox.message:modify") {
		t.Fatalf("error = %v, want missing modify scope", err)
	}
}

func TestMailSenderValidationAndAPIErrorsAreTyped(t *testing.T) {
	_, _, _, _ = mailShortcutTestFactory(t)
	if _, err := normalizeMailSenderAddItems(nil); err == nil {
		t.Fatal("expected missing sender validation error")
	} else {
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("missing sender error = %T, want *errs.ValidationError", err)
		}
		if validationErr.Param != "--sender" {
			t.Fatalf("param = %q, want --sender", validationErr.Param)
		}
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_create",
		Body:   map[string]interface{}{"code": 99991663, "msg": "Forbidden"},
	})
	err := runMountedMailShortcut(t, MailBlockedSendersAdd, []string{"+blocked-senders-add", "--sender", "a@example.com"}, f, stdout)
	if err == nil {
		t.Fatal("expected API permission error, got nil")
	}
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("expected structured problem error, got %T: %v", err, err)
	}
}
