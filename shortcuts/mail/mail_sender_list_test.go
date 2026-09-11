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
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "open-apis/mail/v1/user_mailboxes/me/allow_senders", Body: map[string]any{"code": 0, "data": map[string]any{"items": []any{map[string]any{"email": "alice@example.com"}, map[string]any{"email": "bob@example.com"}}}}})
	if err := runMountedMailShortcut(t, MailSenderSearch, []string{"+sender-search", "--kind", "allow", "--query", "ALICE", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +sender-search: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	items, ok := data["items"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["email"] != "alice@example.com" {
		t.Fatalf("search data = %#v", data)
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
		{name: "set", shortcut: MailSenderSet, args: []string{"+sender-set", "--kind", "allow", "--senders", "a@example.com,b@example.com", "--senders", "a@example.com", "--format", "json"}, url: "open-apis/mail/v1/user_mailboxes/me/allow_senders/batch_create", result: "added"},
		{name: "delete", shortcut: MailSenderDelete, args: []string{"+sender-delete", "--kind", "block", "--senders", "a@example.com", "--format", "json"}, url: "open-apis/mail/v1/user_mailboxes/me/blocked_senders/batch_remove", result: "removed"},
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
			if tc.name == "set" && !reflect.DeepEqual(data["senders"], []any{"a@example.com", "b@example.com"}) {
				t.Fatalf("deduplicated senders = %#v", data["senders"])
			}
			if len(stub.CapturedBodies) != 1 || !strings.Contains(string(stub.CapturedBodies[0]), "a@example.com") {
				t.Fatalf("request bodies = %q", stub.CapturedBodies)
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
