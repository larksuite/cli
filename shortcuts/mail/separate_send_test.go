// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package mail

import (
	"encoding/base64"
	"encoding/json"
	"github.com/larksuite/cli/internal/httpmock"
	"strings"
	"testing"
)

func TestDraftCreateSeparatelyWirePresence(t *testing.T) {
	for _, value := range []string{"", "true", "false"} {
		t.Run(value, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/profile", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"primary_email_address": "me@example.com"}}})
			stub := &httpmock.Stub{Method: "POST", URL: "/user_mailboxes/me/drafts", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"draft_id": "draft-separate"}}}
			reg.Register(stub)
			args := []string{"+draft-create", "--to", "a@example.com", "--to", "b@example.com", "--subject", "Hello", "--body", "Hello", "--no-signature"}
			if value != "" {
				args = append(args, "--send-separately="+value)
			}
			if err := runMountedMailShortcut(t, MailDraftCreate, args, f, stdout); err != nil {
				t.Fatal(err)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
				t.Fatal(err)
			}
			actual, present := body["is_send_separately"]
			if present != (value != "") || (present && actual != (value == "true")) {
				t.Fatalf("unexpected state %v", body)
			}
		})
	}
}

func TestDraftSeparateStateReadAndToggle(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			raw := base64.URLEncoding.EncodeToString([]byte("From: me@example.com\r\nTo: a@example.com, b@example.com\r\nSubject: hello\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhello"))
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/drafts/draft-separate", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"draft": map[string]interface{}{"id": "draft-separate", "message": map[string]interface{}{"raw": raw, "is_send_separately": value == "true"}}}}})
			err := runMountedMailShortcut(t, MailDraftEdit, []string{"+draft-edit", "--draft-id", "draft-separate", "--inspect"}, f, stdout)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), "is_send_separately") {
				t.Fatalf("missing saved state: %s", stdout.String())
			}
			f, stdout, _, reg = mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/drafts/draft-separate", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"draft": map[string]interface{}{"id": "draft-separate", "message": map[string]interface{}{"raw": raw, "is_send_separately": value != "true"}}}}})
			stub := &httpmock.Stub{Method: "PUT", URL: "/user_mailboxes/me/drafts/draft-separate", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"draft_id": "draft-separate"}}}
			reg.Register(stub)
			err = runMountedMailShortcut(t, MailDraftEdit, []string{"+draft-edit", "--draft-id", "draft-separate", "--send-separately=" + value}, f, stdout)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]interface{}
			if err = json.Unmarshal(stub.CapturedBody, &body); err != nil {
				t.Fatal(err)
			}
			if body["is_send_separately"] != (value == "true") {
				t.Fatalf("toggle lost: %v", body)
			}
		})
	}
}
