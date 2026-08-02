// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func runMountedUserAllowBlock(t *testing.T, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "mail"}
	InstallOnMail(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	return parent.Execute()
}

func decodeUserAllowBlockEnvelopeData(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok envelope, got stdout=%s", stdout.String())
	}
	return envelope.Data
}

func TestMailUserAllowBlockAddAllowBuildsBatchCreate(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/allow_senders/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"failed_items": []interface{}{},
			},
		},
	}
	reg.Register(stub)

	err := runMountedUserAllowBlock(t, []string{
		"user-allow-block", "add", "--allow", "alice@example.com", "example.com",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("captured body JSON error: %v body=%s", err, string(stub.CapturedBody))
	}
	items := body["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items length = %d, want 2", len(items))
	}
	first := items[0].(map[string]interface{})
	if first["sender"] != "alice@example.com" || first["sender_type"].(float64) != 1 {
		t.Errorf("first item = %#v, want email sender_type=1", first)
	}
	second := items[1].(map[string]interface{})
	if second["sender"] != "example.com" || second["sender_type"].(float64) != 2 {
		t.Errorf("second item = %#v, want domain sender_type=2", second)
	}

	data := decodeUserAllowBlockEnvelopeData(t, stdout)
	if data["action"] != "add" || data["type"] != "allow" || data["total"].(float64) != 2 {
		t.Errorf("output summary unexpected: %#v", data)
	}
}

func TestInstallOnMailRegistersUserAllowBlock(t *testing.T) {
	f, _, _, _ := mailShortcutTestFactory(t)
	parent := &cobra.Command{Use: "mail"}
	InstallOnMail(parent, f)
	root := findChildCommand(parent, "user-allow-block")
	if root == nil {
		t.Fatal("user-allow-block command not registered")
	}
	for _, name := range []string{"list", "search", "add", "delete", "get"} {
		if findChildCommand(root, name) == nil {
			t.Fatalf("user-allow-block %s command not registered", name)
		}
	}
}

func TestMailUserAllowBlockDeleteBlockBuildsBatchRemove(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/blocked_senders/batch_remove",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"deleted_count": 2},
		},
	}
	reg.Register(stub)

	err := runMountedUserAllowBlock(t, []string{
		"user-allow-block", "delete", "--block", "spammer@example.com", "bad.example",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("captured body JSON error: %v", err)
	}
	senders := body["senders"].([]interface{})
	if got := strings.Join([]string{senders[0].(string), senders[1].(string)}, ","); got != "spammer@example.com,bad.example" {
		t.Errorf("senders = %s", got)
	}
	data := decodeUserAllowBlockEnvelopeData(t, stdout)
	if data["action"] != "delete" || data["type"] != "block" {
		t.Errorf("output summary unexpected: %#v", data)
	}
}

func TestMailUserAllowBlockListAllowUsesQueryParams(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders?page_size=50&page_token=abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []map[string]interface{}{{"sender": "alice@example.com", "sender_type": 1}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})

	err := runMountedUserAllowBlock(t, []string{
		"user-allow-block", "list", "--type", "allow", "--page-size", "50", "--cursor", "abc",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	data := decodeUserAllowBlockEnvelopeData(t, stdout)
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}
}

func TestMailUserAllowBlockSearchAllCallsBothLists(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders?keyword=alice&page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []map[string]interface{}{{"sender": "alice@example.com"}}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/blocked_senders?keyword=alice&page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	})

	err := runMountedUserAllowBlock(t, []string{"user-allow-block", "search", "alice"}, f, stdout)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	data := decodeUserAllowBlockEnvelopeData(t, stdout)
	if data["type"] != "all" || data["keyword"] != "alice" {
		t.Errorf("combined output unexpected: %#v", data)
	}
	if len(data["allow_senders"].([]interface{})) != 1 || len(data["blocked_senders"].([]interface{})) != 0 {
		t.Errorf("combined lists unexpected: %#v", data)
	}
	items := data["items"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["type"] != "allow" {
		t.Errorf("combined table items unexpected: %#v", data["items"])
	}
}

func TestMailUserAllowBlockGetFindsBlockRecord(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders?keyword=bad.example&page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/blocked_senders?keyword=bad.example&page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []map[string]interface{}{{"sender": "bad.example", "sender_type": 2}}},
		},
	})

	err := runMountedUserAllowBlock(t, []string{"user-allow-block", "get", "bad.example"}, f, stdout)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	data := decodeUserAllowBlockEnvelopeData(t, stdout)
	if data["found"] != true || data["type"] != "block" {
		t.Errorf("get output unexpected: %#v", data)
	}
}

func TestMailUserAllowBlockValidationErrors(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "write needs exactly one kind", args: []string{"user-allow-block", "add", "a@example.com"}, want: "exactly one of --allow or --block"},
		{name: "invalid type", args: []string{"user-allow-block", "list", "--type", "bogus"}, want: "--type must be one of"},
		{name: "page size", args: []string{"user-allow-block", "list", "--page-size", "101"}, want: "--page-size must be between"},
		{name: "bad record", args: []string{"user-allow-block", "add", "--allow", "@example.com"}, want: "must be an email address or domain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runMountedUserAllowBlock(t, tt.args, f, stdout)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			var problem interface{ Error() string }
			if !errors.As(err, &problem) {
				t.Fatalf("expected error, got %T", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestMailUserAllowBlockRateLimitHint(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders?page_size=20",
		Body: map[string]interface{}{
			"code": output.LarkErrRateLimit,
			"msg":  "rate limit",
		},
	})

	err := runMountedUserAllowBlock(t, []string{"user-allow-block", "list", "--type", "allow"}, f, stdout)
	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if !strings.Contains(p.Hint, "smaller --page-size") {
		t.Fatalf("hint = %q", p.Hint)
	}
}
