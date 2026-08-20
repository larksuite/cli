// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailSenderListShortcuts_Metadata(t *testing.T) {
	if MailSenderAllowlist.Command != "+sender-allowlist" {
		t.Fatalf("allowlist command = %q", MailSenderAllowlist.Command)
	}
	if MailSenderBlocklist.Command != "+sender-blocklist" {
		t.Fatalf("blocklist command = %q", MailSenderBlocklist.Command)
	}
	if MailSenderAllowlistModify.Command != "+sender-allowlist-modify" {
		t.Fatalf("allowlist modify command = %q", MailSenderAllowlistModify.Command)
	}
	if MailSenderBlocklistModify.Command != "+sender-blocklist-modify" {
		t.Fatalf("blocklist modify command = %q", MailSenderBlocklistModify.Command)
	}
	if MailSenderAllowlist.Risk != "read" || MailSenderBlocklist.Risk != "read" {
		t.Fatalf("read risk = %q/%q, want read", MailSenderAllowlist.Risk, MailSenderBlocklist.Risk)
	}
	if MailSenderAllowlistModify.Risk != "write" || MailSenderBlocklistModify.Risk != "write" {
		t.Fatalf("write risk = %q/%q, want write", MailSenderAllowlistModify.Risk, MailSenderBlocklistModify.Risk)
	}
	if MailSenderAllowlist.Scopes[0] != "mail:user_mailbox.message:readonly" {
		t.Fatalf("read scope = %v", MailSenderAllowlist.Scopes)
	}
	if MailSenderAllowlistModify.Scopes[0] != "mail:user_mailbox.message:modify" {
		t.Fatalf("write scope = %v", MailSenderAllowlistModify.Scopes)
	}
	if len(MailSenderAllowlist.ConditionalScopes) != 0 || len(MailSenderAllowlistModify.ConditionalScopes) != 0 {
		t.Fatalf("conditional scopes should be empty for split read/write shortcuts")
	}
	for _, shortcut := range []struct {
		name  string
		flags []common.Flag
	}{
		{name: MailSenderAllowlist.Command, flags: MailSenderAllowlist.Flags},
		{name: MailSenderBlocklist.Command, flags: MailSenderBlocklist.Flags},
		{name: MailSenderAllowlistModify.Command, flags: MailSenderAllowlistModify.Flags},
		{name: MailSenderBlocklistModify.Command, flags: MailSenderBlocklistModify.Flags},
	} {
		for _, flag := range shortcut.flags {
			if flag.Name == "yes" || flag.Name == "type" {
				t.Fatalf("%s must not register --%s", shortcut.name, flag.Name)
			}
		}
	}
}

func TestMailSenderListShortcut_ListsOrSearches(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders?keyword=fixture&page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []map[string]interface{}{
					{"sender": "fixture.one@sender.test"},
					{"sender": "fixture.two@sender.test"},
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSenderAllowlist, []string{"+sender-allowlist", "--query", "fixture"}, f, stdout)
	if err != nil {
		t.Fatalf("execute shortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v, want 2 items", data["items"])
	}
	reg.Verify(t)
}

func TestMailSenderListShortcut_SkipsEmptyPagesUntilItems(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders?keyword=fixture&page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":   true,
				"items":      []interface{}{},
				"page_token": "tok_2",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "page_token=tok_2",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":   true,
				"items":      []interface{}{},
				"page_token": "tok_3",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "page_token=tok_3",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_4",
				"items": []map[string]interface{}{
					{"sender": "fixture.sender.test"},
				},
			},
		},
	})

	err := runMountedMailShortcut(t, MailSenderAllowlist, []string{"+sender-allowlist", "--query", "fixture"}, f, stdout)
	if err != nil {
		t.Fatalf("execute shortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want 1 item from first non-empty page", data["items"])
	}
	if data["page_token"] != "tok_4" || data["has_more"] != true {
		t.Fatalf("pagination data = %#v, want token tok_4 and has_more=true", data)
	}
	reg.Verify(t)
}

func TestMailSenderListShortcut_ReturnsEmptyAfterRetryLimit(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	tokens := []string{"", "tok_2", "tok_3", "tok_4"}
	nextTokens := []string{"tok_2", "tok_3", "tok_4", "tok_5"}
	for i, token := range tokens {
		url := "/user_mailboxes/me/blocked_senders?page_size=20"
		if token != "" {
			url = "page_token=" + token
		}
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    url,
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more":   true,
					"items":      []interface{}{},
					"page_token": nextTokens[i],
				},
			},
		})
	}

	err := runMountedMailShortcut(t, MailSenderBlocklist, []string{"+sender-blocklist"}, f, stdout)
	if err != nil {
		t.Fatalf("execute shortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 0 {
		t.Fatalf("items = %#v, want empty items after retry limit", data["items"])
	}
	if data["page_token"] != "tok_5" || data["has_more"] != true {
		t.Fatalf("pagination data = %#v, want final empty page token tok_5 and has_more=true", data)
	}
	reg.Verify(t)
}

func TestMailSenderListShortcut_DryRunReadShowsPlan(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderAllowlist, []string{
		"+sender-allowlist",
		"--mailbox", "me",
		"--query", "fixture",
		"--page-size", "20",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/allow_senders`,
		`"method"`,
		`"GET"`,
		`"keyword"`,
		`"fixture"`,
		`"page_size"`,
		`20`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
}

func TestMailSenderListModifyShortcut_AddInfersSenderType(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"failed_items": []interface{}{}},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSenderBlocklistModify, []string{
		"+sender-blocklist-modify",
		"--add", "bad.example,spam@example.com",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute shortcut: %v", err)
	}
	body := decodeCapturedSenderListJSONBody(t, stub)
	items, ok := body["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v, want two item objects", body["items"])
	}
	first := items[0].(map[string]interface{})
	if first["sender"] != "bad.example" || first["sender_type"].(float64) != 2 {
		t.Fatalf("first item = %#v, want inferred domain", first)
	}
	second := items[1].(map[string]interface{})
	if second["sender"] != "spam@example.com" || second["sender_type"].(float64) != 1 {
		t.Fatalf("second item = %#v, want inferred email", second)
	}
}

func TestMailSenderListModifyShortcut_DryRunWriteShowsPlan(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderBlocklistModify, []string{
		"+sender-blocklist-modify",
		"--add", "bad.example,spam@example.com",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/blocked_senders/batch_create`,
		`"method"`,
		`"POST"`,
		`"sender"`,
		`"bad.example"`,
		`"spam@example.com"`,
		`"sender_type"`,
		`2`,
		`1`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
}

func TestMailSenderListModifyShortcut_CreateAliasAdds(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/allow_senders/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"failed_items": []interface{}{}},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSenderAllowlistModify, []string{
		"+sender-allowlist-modify",
		"--create", "fixture.sender.test",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute shortcut: %v", err)
	}
	body := decodeCapturedSenderListJSONBody(t, stub)
	items := body["items"].([]interface{})
	first := items[0].(map[string]interface{})
	if first["sender"] != "fixture.sender.test" || first["sender_type"].(float64) != 2 {
		t.Fatalf("first item = %#v", first)
	}
}

func TestMailSenderListModifyShortcut_RemoveAliasesBuildSendersBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
	}{
		{name: "remove", flag: "--remove"},
		{name: "delete", flag: "--delete"},
		{name: "trash", flag: "--trash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/user_mailboxes/me/allow_senders/batch_remove",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"deleted_count": 2},
				},
			}
			reg.Register(stub)

			err := runMountedMailShortcut(t, MailSenderAllowlistModify, []string{
				"+sender-allowlist-modify",
				tc.flag, "fixture.one@sender.test,fixture.two@sender.test",
			}, f, stdout)
			if err != nil {
				t.Fatalf("execute shortcut: %v", err)
			}
			body := decodeCapturedSenderListJSONBody(t, stub)
			senders, ok := body["senders"].([]interface{})
			if !ok || len(senders) != 2 {
				t.Fatalf("senders = %#v, want two senders", body["senders"])
			}
		})
	}
}

func TestMailSenderListShortcut_ValidateInputs(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderAllowlist, []string{
		"+sender-allowlist",
		"--page-size", "0",
	}, f, stdout)
	requireSenderListValidationParam(t, err, "--page-size")

	err = runMountedMailShortcut(t, MailSenderAllowlist, []string{
		"+sender-allowlist",
		"--page-size", "101",
	}, f, stdout)
	requireSenderListValidationParam(t, err, "--page-size")

	err = runMountedMailShortcut(t, MailSenderAllowlistModify, []string{
		"+sender-allowlist-modify",
		"--add", "valid@example.com,",
	}, f, stdout)
	requireSenderListValidationParam(t, err, "--add")

	err = runMountedMailShortcut(t, MailSenderAllowlistModify, []string{
		"+sender-allowlist-modify",
		"--add", "a@example.com",
		"--remove", "b@example.com",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v, want mutually exclusive", err)
	}

	err = runMountedMailShortcut(t, MailSenderAllowlistModify, []string{
		"+sender-allowlist-modify",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "one of --add") {
		t.Fatalf("error = %v, want missing action", err)
	}
}

func decodeCapturedSenderListJSONBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode captured body: %v\n%s", err, string(stub.CapturedBody))
	}
	return body
}

func requireSenderListValidationParam(t *testing.T, err error, param string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error for %s", param)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Param != param {
		t.Fatalf("param = %q, want %q", validationErr.Param, param)
	}
}
