// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func assertValidationError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Fatalf("exit code = %d, want ExitValidation", exitErr.Code)
	}
	if wantSubstr != "" && !strings.Contains(exitErr.Error(), wantSubstr) {
		t.Fatalf("error = %q, want substring %q", exitErr.Error(), wantSubstr)
	}
}

func TestParseSenderAddressList(t *testing.T) {
	got, err := parseSenderAddressList([]string{" Alice@Example.COM, bob@example.com ", "alice@example.com"}, true)
	if err != nil {
		t.Fatalf("parseSenderAddressList() error = %v", err)
	}
	if strings.Join(got, ",") != "alice@example.com,bob@example.com" {
		t.Fatalf("normalized addresses = %v", got)
	}

	got, err = parseSenderAddressList([]string{" Alice@Example.COM "}, false)
	if err != nil {
		t.Fatalf("parseSenderAddressList(delete) error = %v", err)
	}
	if got[0] != "Alice@Example.COM" {
		t.Fatalf("delete address should preserve case, got %q", got[0])
	}

	_, err = parseSenderAddressList([]string{"not-an-address"}, true)
	assertValidationError(t, err, "invalid email address")
}

func TestMailSenderValidateErrors(t *testing.T) {
	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		want     string
	}{
		{
			name:     "set requires type",
			shortcut: MailSenderSet,
			args:     []string{"+sender-set", "--address", "a@example.com"},
			want:     "--type is required",
		},
		{
			name:     "set rejects all",
			shortcut: MailSenderSet,
			args:     []string{"+sender-set", "--type", "all", "--address", "a@example.com"},
			want:     "allowed: allow, block",
		},
		{
			name:     "set requires address",
			shortcut: MailSenderSet,
			args:     []string{"+sender-set", "--type", "allow"},
			want:     "--address is required",
		},
		{
			name:     "query requires query",
			shortcut: MailSenderQuery,
			args:     []string{"+sender-query", "--type", "allow"},
			want:     "--query is required",
		},
		{
			name:     "list page size",
			shortcut: MailSenderList,
			args:     []string{"+sender-list", "--page-size", "101"},
			want:     "--page-size must be between",
		},
		{
			name:     "bot me",
			shortcut: MailSenderList,
			args:     []string{"+sender-list", "--as", "bot", "--mailbox", "me"},
			want:     "does not support --mailbox me",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			assertValidationError(t, err, tc.want)
		})
	}
}

func TestMailSenderDryRun(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderQuery, []string{
		"+sender-query",
		"--mailbox", "me",
		"--type", "all",
		"--query", "Example.COM",
		"--page-size", "25",
		"--page-token", "tok",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"method": "GET"`,
		`"url": "/open-apis/mail/v1/user_mailboxes/me/allow_senders"`,
		`"url": "/open-apis/mail/v1/user_mailboxes/me/blocked_senders"`,
		`"keyword": "Example.COM"`,
		`"page_size": 25`,
		`"page_token": "tok"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}

	stdout.Reset()
	err = runMountedMailShortcut(t, MailSenderSet, []string{
		"+sender-set",
		"--type", "allow",
		"--address", "Alice@Example.COM,bob@example.com",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("set dry-run error = %v", err)
	}
	out = stdout.String()
	for _, want := range []string{
		`"method": "POST"`,
		`"url": "/open-apis/mail/v1/user_mailboxes/me/allow_senders/batch_create"`,
		`"address": "alice@example.com"`,
		`"address": "bob@example.com"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("set dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestMailSenderListExecuteAll(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerSenderListStub(reg, "allow_senders", map[string]interface{}{
		"items": []map[string]interface{}{{"address": "allow@example.com", "timestamp": float64(171)}},
	})
	registerSenderListStub(reg, "blocked_senders", map[string]interface{}{
		"items":      []map[string]interface{}{{"address": "block@example.com", "timestamp": float64(172)}},
		"has_more":   true,
		"page_token": "next-block",
	})

	err := runMountedMailShortcut(t, MailSenderList, []string{
		"+sender-list",
		"--type", "all",
		"--page-size", "50",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute list error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["total"].(float64) != 2 {
		t.Fatalf("total = %v, want 2", data["total"])
	}
	items := data["items"].([]interface{})
	gotTypes := []string{
		items[0].(map[string]interface{})["list_type"].(string),
		items[1].(map[string]interface{})["list_type"].(string),
	}
	if strings.Join(gotTypes, ",") != "allow,block" {
		t.Fatalf("list types = %v", gotTypes)
	}
	tokens := data["next_page_tokens"].(map[string]interface{})
	if tokens["block"] != "next-block" {
		t.Fatalf("next_page_tokens = %v", tokens)
	}
}

func TestMailSenderQueryExactExecute(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerSenderListStub(reg, "allow_senders", map[string]interface{}{
		"items": []map[string]interface{}{
			{"address": "Alice@Example.com", "timestamp": float64(171)},
			{"address": "other@example.com", "timestamp": float64(172)},
		},
	})

	err := runMountedMailShortcut(t, MailSenderQuery, []string{
		"+sender-query",
		"--type", "allow",
		"--query", "alice@example.COM",
		"--exact",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute query error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1; data=%v", len(items), data)
	}
	item := items[0].(map[string]interface{})
	if item["address"] != "Alice@Example.com" || item["list_type"] != "allow" {
		t.Fatalf("item = %v", item)
	}
}

func TestMailSenderSetAndDeleteExecute(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"failed_items": []map[string]interface{}{{"address": "bad@example.com", "reason": "invalid"}},
			},
		},
	})
	err := runMountedMailShortcut(t, MailSenderSet, []string{
		"+sender-set",
		"--type", "block",
		"--address", "Bad@Example.COM",
	}, f, stdout)
	if err != nil {
		t.Fatalf("set error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	failed := data["failed_items"].([]interface{})
	if len(failed) != 1 || failed[0].(map[string]interface{})["address"] != "bad@example.com" {
		t.Fatalf("failed_items = %v", failed)
	}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_remove",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"deleted_count": 1},
		},
	})
	err = runMountedMailShortcut(t, MailSenderDelete, []string{
		"+sender-delete",
		"--type", "block",
		"--address", "Bad@Example.COM",
	}, f, stdout)
	if err != nil {
		t.Fatalf("delete error = %v", err)
	}
	data = decodeShortcutEnvelopeData(t, stdout)
	if data["deleted_count"].(float64) != 1 {
		t.Fatalf("deleted_count = %v", data["deleted_count"])
	}
	addresses := data["addresses"].([]interface{})
	if addresses[0] != "Bad@Example.COM" {
		t.Fatalf("delete should preserve address case, got %v", addresses)
	}
}

func TestMailSenderAPI456AddsRetryHint(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders",
		Body: map[string]interface{}{
			"code": 456,
			"msg":  "search cache warming",
		},
	})
	err := runMountedMailShortcut(t, MailSenderQuery, []string{
		"+sender-query",
		"--type", "allow",
		"--query", "a",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail == nil || exitErr.Detail.Code != 456 || !strings.Contains(exitErr.Detail.Hint, "retry later") {
		t.Fatalf("problem = %#v", exitErr.Detail)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want ExitAPI", exitErr.Code)
	}
}

func registerSenderListStub(reg *httpmock.Registry, resource string, data map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/" + resource,
		Body: map[string]interface{}{
			"code": 0,
			"data": data,
		},
	})
}
