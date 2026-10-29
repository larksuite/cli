// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailSenderListShortcutsAreRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, shortcut := range Shortcuts() {
		registered[shortcut.Command] = true
	}
	for _, command := range []string{"+sender-list", "+sender-search", "+sender-set", "+sender-delete"} {
		if !registered[command] {
			t.Fatalf("shortcut %s is not registered", command)
		}
	}

	if got := MailSenderList.Scopes; len(got) != 1 || got[0] != "mail:user_mailbox.message:readonly" {
		t.Fatalf("MailSenderList scopes = %v, want readonly", got)
	}
	if got := MailSenderSet.Scopes; len(got) != 1 || got[0] != "mail:user_mailbox.message:modify" {
		t.Fatalf("MailSenderSet scopes = %v, want modify", got)
	}

	typeFlag, ok := senderListFlagByName(MailSenderSet.Flags, "type")
	if !ok {
		t.Fatal("missing --type flag")
	}
	if got := strings.Join(typeFlag.Enum, ","); got != "allow,block" {
		t.Fatalf("--type enum = %q, want allow,block", got)
	}
}

func TestMailSenderListUsesPaginationAndKeyword(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	list := &httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/allow_senders",
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if got := q.Get("page_size"); got != "50" {
				t.Fatalf("page_size = %q, want 50", got)
			}
			if got := q.Get("page_token"); got != "cursor-1" {
				t.Fatalf("page_token = %q, want cursor-1", got)
			}
			if got := q.Get("keyword"); got != "example.com" {
				t.Fatalf("keyword = %q, want example.com", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"senders": []interface{}{
					map[string]interface{}{"sender": "alice@example.com", "sender_type": 1},
				},
				"has_more":        true,
				"next_page_token": "cursor-2",
			},
		},
	}
	reg.Register(list)

	err := runMountedMailShortcut(t, MailSenderList, []string{
		"+sender-list",
		"--type", "allow",
		"--keyword", " Example.COM ",
		"--page-size", "50",
		"--page-token", "cursor-1",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("sender-list failed: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["next_page_token"] != "cursor-2" || data["has_more"] != true {
		t.Fatalf("pagination fields mismatch: %#v", data)
	}
	if data["keyword"] != "example.com" {
		t.Fatalf("keyword = %v, want example.com", data["keyword"])
	}
}

func TestMailSenderSetFiltersInvalidAndLowercases(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := &httpmock.Stub{
		Method: "POST",
		URL:    "open-apis/mail/v1/user_mailboxes/mbx_1/allow_senders/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ok": true},
		},
	}
	reg.Register(post)

	err := runMountedMailShortcut(t, MailSenderSet, []string{
		"+sender-set",
		"--type", "allow",
		"--mailbox", "mbx_1",
		"--sender", " Alice@Example.COM ",
		"--sender", "EXAMPLE.ORG",
		"--sender", "bad sender",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("sender-set failed: %v", err)
	}

	body := decodeSenderListCapturedBody(t, post)
	senders := body["senders"].([]interface{})
	if len(senders) != 2 {
		t.Fatalf("senders = %#v, want two valid entries", senders)
	}
	assertSenderEntry(t, senders[0], "alice@example.com", 1)
	assertSenderEntry(t, senders[1], "example.org", 2)

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["success_count"] != float64(2) || data["filtered_count"] != float64(1) {
		t.Fatalf("counts mismatch: %#v", data)
	}
	invalid := data["invalid_senders"].([]interface{})
	if got := invalid[0].(map[string]interface{})["input"]; got != "bad sender" {
		t.Fatalf("invalid input = %v, want bad sender", got)
	}
}

func TestMailSenderDeleteIncludesMixedCaseCompatibility(t *testing.T) {
	f, _, _, reg := mailShortcutTestFactory(t)
	post := &httpmock.Stub{
		Method: "POST",
		URL:    "open-apis/mail/v1/user_mailboxes/me/blocked_senders/batch_remove",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ok": true},
		},
	}
	reg.Register(post)

	err := runMountedMailShortcut(t, MailSenderDelete, []string{
		"+sender-delete",
		"--type", "block",
		"--user-mailbox-id", "me",
		"--sender", "Alice@Example.COM",
		"--format", "json",
	}, f, nil)
	if err != nil {
		t.Fatalf("sender-delete failed: %v", err)
	}

	body := decodeSenderListCapturedBody(t, post)
	senders := body["senders"].([]interface{})
	if len(senders) != 2 {
		t.Fatalf("senders = %#v, want lowercase and original-case variants", senders)
	}
	assertSenderEntry(t, senders[0], "Alice@Example.COM", 1)
	assertSenderEntry(t, senders[1], "alice@example.com", 1)
}

func TestMailSenderSearchRequiresKeyword(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderSearch, []string{
		"+sender-search",
		"--type", "allow",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected missing keyword validation error")
	}
	if !strings.Contains(err.Error(), "--keyword is required") {
		t.Fatalf("error = %v, want keyword hint", err)
	}
}

func TestMailSenderListDecoratesBackendPermissionError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/blocked_senders",
		Status: http.StatusForbidden,
		Body: map[string]interface{}{
			"code": 99991663,
			"msg":  "Permission denied",
		},
	})

	err := runMountedMailShortcut(t, MailSenderList, []string{
		"+sender-list",
		"--type", "block",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected backend permission error")
	}
	for _, want := range []string{"list block senders failed", "Permission denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should contain %q, got %v", want, err)
		}
	}
}

func TestMailSenderSetDecoratesBackendMutualExclusionError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "open-apis/mail/v1/user_mailboxes/me/allow_senders/batch_create",
		Status: http.StatusBadRequest,
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "sender already exists in blocked sender list",
		},
	})

	err := runMountedMailShortcut(t, MailSenderSet, []string{
		"+sender-set",
		"--type", "allow",
		"--sender", "blocked@example.com",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected backend mutual exclusion error")
	}
	for _, want := range []string{"set allow senders failed", "blocked sender list"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should contain %q, got %v", want, err)
		}
	}
}

func senderListFlagByName(flags []common.Flag, name string) (common.Flag, bool) {
	for _, flag := range flags {
		if flag.Name == name {
			return flag, true
		}
	}
	return common.Flag{}, false
}

func decodeSenderListCapturedBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &out); err != nil {
		t.Fatalf("decode captured body: %v (raw=%s)", err, string(stub.CapturedBody))
	}
	return out
}

func assertSenderEntry(t *testing.T, value interface{}, sender string, senderType int) {
	t.Helper()
	entry, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("sender entry = %#v, want object", value)
	}
	if got := entry["sender"]; got != sender {
		t.Fatalf("sender = %v, want %s", got, sender)
	}
	if got := entry["sender_type"]; got != float64(senderType) {
		t.Fatalf("sender_type = %v, want %d", got, senderType)
	}
}
