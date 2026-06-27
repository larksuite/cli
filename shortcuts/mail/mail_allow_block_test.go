// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailAllowBlock_Metadata(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		command  string
		risk     string
		scopes   []string
		flags    []string
	}{
		{
			name:     "list",
			shortcut: MailAllowBlockList,
			command:  "+allow-block-list",
			risk:     "read",
			scopes:   allowBlockReadScopes,
			flags:    []string{"mailbox", "type", "query", "page-size", "page-token"},
		},
		{
			name:     "set",
			shortcut: MailAllowBlockSet,
			command:  "+allow-block-set",
			risk:     "write",
			scopes:   allowBlockWriteScopes,
			flags:    []string{"mailbox", "type", "address", "scene"},
		},
		{
			name:     "delete",
			shortcut: MailAllowBlockDelete,
			command:  "+allow-block-delete",
			risk:     "write",
			scopes:   allowBlockWriteScopes,
			flags:    []string{"mailbox", "type", "address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shortcut.Service != "mail" {
				t.Errorf("Service = %q, want mail", tt.shortcut.Service)
			}
			if tt.shortcut.Command != tt.command {
				t.Errorf("Command = %q, want %q", tt.shortcut.Command, tt.command)
			}
			if tt.shortcut.Risk != tt.risk {
				t.Errorf("Risk = %q, want %q", tt.shortcut.Risk, tt.risk)
			}
			if strings.Join(tt.shortcut.Scopes, " ") != strings.Join(tt.scopes, " ") {
				t.Errorf("Scopes = %#v, want %#v", tt.shortcut.Scopes, tt.scopes)
			}
			if strings.Join(tt.shortcut.AuthTypes, " ") != "user bot" {
				t.Errorf("AuthTypes = %#v, want [user bot]", tt.shortcut.AuthTypes)
			}
			gotFlags := map[string]common.Flag{}
			for _, fl := range tt.shortcut.Flags {
				gotFlags[fl.Name] = fl
			}
			for _, name := range tt.flags {
				if _, ok := gotFlags[name]; !ok {
					t.Errorf("missing flag %s", name)
				}
			}
		})
	}
}

func TestMailAllowBlockList_MapsAllowRequest(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders",
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if q.Get("keyword") != "example.com" {
				t.Fatalf("keyword query = %q, want example.com", q.Get("keyword"))
			}
			if q.Get("page_size") != "20" {
				t.Fatalf("page_size query = %q, want 20", q.Get("page_size"))
			}
			if q.Get("page_token") != "cursor-0" {
				t.Fatalf("page_token query = %q, want cursor-0", q.Get("page_token"))
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"sender": "a@example.com", "scene": "sender"},
				},
				"has_more":        true,
				"next_page_token": "cursor-1",
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailAllowBlockList, []string{
		"+allow-block-list",
		"--type", "allow",
		"--query", "example.com",
		"--page-size", "20",
		"--page-token", "cursor-0",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if got := stub.CapturedHeaders.Get("Authorization"); got == "" {
		t.Fatal("expected Authorization header to be set")
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["type"] != allowBlockTypeAllow {
		t.Fatalf("type = %v, want allow", data["type"])
	}
	if data["next_page_token"] != "cursor-1" {
		t.Fatalf("next_page_token = %v, want cursor-1", data["next_page_token"])
	}
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	item := items[0].(map[string]interface{})
	if item["type"] != allowBlockTypeAllow {
		t.Fatalf("item type = %v, want allow", item["type"])
	}
}

func TestMailAllowBlockList_AllCallsBothResources(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{{"sender": "ally@example.com"}},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/blocked_senders",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{{"sender": "spam.example.com"}},
			},
		},
	})

	err := runMountedMailShortcut(t, MailAllowBlockList, []string{
		"+allow-block-list",
		"--type", "all",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	data := decodeShortcutEnvelopeData(t, stdout)
	items := data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	if items[0].(map[string]interface{})["type"] != allowBlockTypeAllow {
		t.Fatalf("first item type = %v, want allow", items[0])
	}
	if items[1].(map[string]interface{})["type"] != allowBlockTypeBlock {
		t.Fatalf("second item type = %v, want block", items[1])
	}
}

func TestMailAllowBlockSet_MapsBatchCreateAndWarnsFailedItems(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_create",
		BodyFilter: func(body []byte) bool {
			var got map[string]interface{}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("request JSON: %v", err)
			}
			items := got["items"].([]interface{})
			if len(items) != 2 {
				t.Fatalf("items len = %d, want 2", len(items))
			}
			first := items[0].(map[string]interface{})
			second := items[1].(map[string]interface{})
			return first["sender"] == "spam@example.com" &&
				first["sender_type"].(float64) == 1 &&
				first["scene"] == "web_image" &&
				second["sender_type"].(float64) == 2
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"success_count": 1,
				"failed_items": []map[string]interface{}{
					{"sender": "bad", "reason": "invalid address"},
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailAllowBlockSet, []string{
		"+allow-block-set",
		"--type", "block",
		"--address", "spam@example.com,bad",
		"--scene", "web_image",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stderr.String(), "warning: 1 allow/block item") {
		t.Fatalf("stderr missing failed_items warning: %s", stderr.String())
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["success_count"].(float64) != 1 {
		t.Fatalf("success_count = %v, want 1", data["success_count"])
	}
	if len(data["failed_items"].([]interface{})) != 1 {
		t.Fatalf("failed_items = %#v, want one item", data["failed_items"])
	}
}

func TestMailAllowBlockDelete_MapsBatchRemove(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/alice@example.com/allow_senders/batch_remove",
		BodyFilter: func(body []byte) bool {
			var got map[string]interface{}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("request JSON: %v", err)
			}
			senders := got["senders"].([]interface{})
			return len(senders) == 2 && senders[0] == "a@example.com" && senders[1] == "example.org"
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"deleted_count": 2},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailAllowBlockDelete, []string{
		"+allow-block-delete",
		"--mailbox", "alice@example.com",
		"--type", "allow",
		"--address", "a@example.com",
		"--address", "example.org",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["success_count"].(float64) != 2 {
		t.Fatalf("success_count = %v, want 2", data["success_count"])
	}
}

func TestMailAllowBlockValidation(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		param    string
		cobraErr string
	}{
		{
			name:     "bot me rejected",
			shortcut: MailAllowBlockList,
			args:     []string{"+allow-block-list", "--as", "bot", "--mailbox", "me"},
			param:    "--mailbox",
		},
		{
			name:     "set rejects all",
			shortcut: MailAllowBlockSet,
			args:     []string{"+allow-block-set", "--type", "all", "--address", "a@example.com"},
			param:    "--type",
		},
		{
			name:     "delete requires address",
			shortcut: MailAllowBlockDelete,
			args:     []string{"+allow-block-delete", "--type", "allow"},
			cobraErr: `required flag(s) "address" not set`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tt.shortcut, tt.args, f, stdout)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if tt.cobraErr != "" {
				if !strings.Contains(err.Error(), tt.cobraErr) {
					t.Fatalf("error = %v, want %q", err, tt.cobraErr)
				}
				return
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected validation error, got %T: %v", err, err)
			}
			if validationErr.Param != tt.param {
				t.Fatalf("Param = %q, want %q", validationErr.Param, tt.param)
			}
		})
	}
}

func TestMailAllowBlockAPIHints(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		want string
	}{
		{
			name: "self address",
			body: map[string]interface{}{"code": 400, "msg": "cannot add self address"},
			want: "do not add your own email address",
		},
		{
			name: "cache not ready",
			body: map[string]interface{}{"code": 456, "msg": "search cache is building, retry later"},
			want: "search cache may still be building",
		},
		{
			name: "scope denied",
			body: map[string]interface{}{"code": 99991679, "msg": "scope denied"},
			want: "scope",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/user_mailboxes/me/allow_senders",
				Body:   tt.body,
			})
			err := runMountedMailShortcut(t, MailAllowBlockList, []string{
				"+allow-block-list",
				"--type", "allow",
			}, f, stdout)
			if err == nil {
				t.Fatal("expected API error, got nil")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T: %v", err, err)
			}
			combined := p.Message + " " + p.Hint
			if !strings.Contains(strings.ToLower(combined), strings.ToLower(tt.want)) {
				t.Fatalf("error missing %q: message=%q hint=%q", tt.want, p.Message, p.Hint)
			}
		})
	}
}
