// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailSenderListUsesAllowAndBlockRoutes(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		url  string
	}{
		{name: "allow", kind: "allow", url: "open-apis/mail/v1/user_mailboxes/me/allow_senders"},
		{name: "block", kind: "block", url: "open-apis/mail/v1/user_mailboxes/me/blocked_senders"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: tc.url,
				OnMatch: func(req *http.Request) {
					if got := req.URL.Query().Get("page_size"); got != "2" {
						t.Fatalf("page_size = %q, want 2", got)
					}
					if got := req.URL.Query().Get("page_token"); got != "next" {
						t.Fatalf("page_token = %q, want next", got)
					}
				},
				Body: map[string]any{"code": 0, "data": map[string]any{"items": []any{map[string]any{"email": "alice@example.com"}, map[string]any{"email": "bob@example.com"}}, "page_token": "after", "has_more": true}},
			})
			if err := runMountedMailShortcut(t, MailSenderList, []string{"+sender-list", "--kind", tc.kind, "--page-size", "2", "--page-token", "next", "--format", "json"}, f, stdout); err != nil {
				t.Fatalf("run +sender-list: %v", err)
			}
			data := decodeShortcutEnvelopeData(t, stdout)
			if data["total"] != float64(2) || data["page_token"] != "after" {
				t.Fatalf("list data = %#v", data)
			}
		})
	}
}

func TestMailSenderSearchFiltersOnePage(t *testing.T) {
	for _, tc := range []struct {
		kind string
		path string
	}{
		{kind: "allow", path: "allow_senders"},
		{kind: "block", path: "blocked_senders"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "open-apis/mail/v1/user_mailboxes/me/" + tc.path, OnMatch: func(req *http.Request) {
				if got := req.URL.Query().Get("keyword"); got != "ALICE" {
					t.Fatalf("keyword = %q, want ALICE", got)
				}
			}, Body: map[string]any{"code": 0, "data": map[string]any{"items": []any{map[string]any{"sender": "alice@example.com"}}}}})
			if err := runMountedMailShortcut(t, MailSenderSearch, []string{"+sender-search", "--kind", tc.kind, "--query", "ALICE", "--format", "json"}, f, stdout); err != nil {
				t.Fatalf("run +sender-search: %v", err)
			}
			data := decodeShortcutEnvelopeData(t, stdout)
			items, ok := data["items"].([]any)
			if !ok || len(items) != 1 || items[0].(map[string]any)["sender"] != "alice@example.com" {
				t.Fatalf("search data = %#v", data)
			}
		})
	}
}

func TestMailSenderMutationsUseBatchRoutesAndBody(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		url      string
		result   string
	}{
		{name: "set allow", shortcut: MailSenderSet, args: []string{"+sender-set", "--kind", "allow", "--senders", "a@example.com,b@example.com", "--senders", "a@example.com", "--format", "json"}, url: "open-apis/mail/v1/user_mailboxes/me/allow_senders/batch_create", result: "added"},
		{name: "set block", shortcut: MailSenderSet, args: []string{"+sender-set", "--kind", "block", "--senders", "a@example.com", "--format", "json"}, url: "open-apis/mail/v1/user_mailboxes/me/blocked_senders/batch_create", result: "added"},
		{name: "delete allow", shortcut: MailSenderDelete, args: []string{"+sender-delete", "--kind", "allow", "--senders", "a@example.com", "--yes", "--format", "json"}, url: "open-apis/mail/v1/user_mailboxes/me/allow_senders/batch_remove", result: "removed"},
		{name: "delete block", shortcut: MailSenderDelete, args: []string{"+sender-delete", "--kind", "block", "--senders", "a@example.com", "--yes", "--format", "json"}, url: "open-apis/mail/v1/user_mailboxes/me/blocked_senders/batch_remove", result: "removed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: tc.url, OnMatch: func(req *http.Request) {
				if !strings.Contains(req.Header.Get("Content-Type"), "application/json") {
					t.Fatalf("content type = %q", req.Header.Get("Content-Type"))
				}
			}, Body: map[string]any{"code": 0, "data": map[string]any{"ok": true}}}
			reg.Register(stub)
			if err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout); err != nil {
				t.Fatalf("run %s: %v", tc.name, err)
			}
			data := decodeShortcutEnvelopeData(t, stdout)
			if data["result"] != tc.result {
				t.Fatalf("result = %#v", data)
			}
			if tc.name == "set allow" && !reflect.DeepEqual(data["senders"], []any{"a@example.com", "b@example.com"}) {
				t.Fatalf("deduplicated senders = %#v", data["senders"])
			}
			if len(stub.CapturedBodies) != 1 || !strings.Contains(string(stub.CapturedBodies[0]), "a@example.com") {
				t.Fatalf("request bodies = %q", stub.CapturedBodies)
			}
			body := string(stub.CapturedBodies[0])
			if strings.HasPrefix(tc.name, "set") && (!strings.Contains(body, "\"items\"") || !strings.Contains(body, "\"sender_type\":1")) {
				t.Fatalf("create body must use UserSenderItem objects: %s", body)
			}
			if strings.HasPrefix(tc.name, "delete") && strings.Contains(body, "\"items\"") {
				t.Fatalf("delete body must use senders: %s", body)
			}
		})
	}
}

func TestMailSenderScopesAndDryRun(t *testing.T) {
	if !reflect.DeepEqual(MailSenderList.Scopes, []string{senderListReadScope}) || !reflect.DeepEqual(MailSenderSearch.Scopes, []string{senderListReadScope}) {
		t.Fatal("read shortcuts must declare the readonly scope")
	}
	if !reflect.DeepEqual(MailSenderSet.Scopes, []string{senderListWriteScope}) || !reflect.DeepEqual(MailSenderDelete.Scopes, []string{senderListWriteScope}) {
		t.Fatal("write shortcuts must declare the modify scope")
	}
	for _, shortcut := range []common.Shortcut{MailSenderList, MailSenderSearch, MailSenderSet, MailSenderDelete} {
		f, _, _, _ := mailShortcutTestFactory(t)
		args := []string{shortcut.Command, "--kind", "allow", "--dry-run"}
		if shortcut.Command == "+sender-search" {
			args = append(args, "--query", "alice")
		}
		if shortcut.Command == "+sender-set" || shortcut.Command == "+sender-delete" {
			args = append(args, "--senders", "alice@example.com")
		}
		if err := runMountedMailShortcut(t, shortcut, args, f, nil); err != nil {
			t.Fatalf("%s dry run: %v", shortcut.Command, err)
		}
	}
}

func TestMailSenderValidation(t *testing.T) {
	f, _, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderSet, []string{"+sender-set", "--kind", "allow", "--senders", " , "}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "--senders") {
		t.Fatalf("empty sender error = %v", err)
	}
}

func TestMailSenderSetInfersDomainSenderType(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{Method: "POST", URL: "open-apis/mail/v1/user_mailboxes/me/allow_senders/batch_create", Body: map[string]any{"code": 0, "data": map[string]any{}}}
	reg.Register(stub)
	if err := runMountedMailShortcut(t, MailSenderSet, []string{"+sender-set", "--kind", "allow", "--senders", "example.com", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +sender-set: %v", err)
	}
	if body := string(stub.CapturedBodies[0]); !strings.Contains(body, "\"sender_type\":2") {
		t.Fatalf("domain sender_type missing: %s", body)
	}
}

func TestMailSenderDeleteRequiresConfirmation(t *testing.T) {
	if MailSenderDelete.Risk != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write", MailSenderDelete.Risk)
	}
	f, _, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderDelete, []string{"+sender-delete", "--kind", "block", "--senders", "sender@example.com"}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("missing confirmation error = %v", err)
	}
}
