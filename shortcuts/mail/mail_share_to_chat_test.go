// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestShareToChatValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing both message-id and thread-id",
			args:    []string{"+share-to-chat", "--receive-id", "oc_xxx"},
			wantErr: "either --message-id or --thread-id is required",
		},
		{
			name:    "both message-id and thread-id",
			args:    []string{"+share-to-chat", "--message-id", "m1", "--thread-id", "t1", "--receive-id", "oc_xxx"},
			wantErr: "--message-id and --thread-id are mutually exclusive",
		},
		{
			name:    "invalid receive-id-type",
			args:    []string{"+share-to-chat", "--message-id", "m1", "--receive-id", "oc_xxx", "--receive-id-type", "invalid"},
			wantErr: "--receive-id-type must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailShareToChat, tt.args, f, stdout)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestShareToChatExecuteWithMessageID(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/share_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"card_id": "card_001",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/share_tokens/card_001/send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id": "om_001",
			},
		},
	})

	err := runMountedMailShortcut(t, MailShareToChat, []string{
		"+share-to-chat", "--message-id", "m1", "--receive-id", "oc_xxx",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "card_001") {
		t.Errorf("expected output to contain card_id, got %s", out)
	}
	if !strings.Contains(out, "om_001") {
		t.Errorf("expected output to contain im_message_id, got %s", out)
	}
}

func TestShareToChatExecuteWithThreadID(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/share_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"card_id": "card_002",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/share_tokens/card_002/send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id": "om_002",
			},
		},
	})

	err := runMountedMailShortcut(t, MailShareToChat, []string{
		"+share-to-chat", "--thread-id", "t1", "--receive-id", "user@example.com", "--receive-id-type", "email",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "card_002") {
		t.Errorf("expected output to contain card_id, got %s", out)
	}
}

func TestShareToChatStep1Failure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/share_token",
		Body: map[string]interface{}{
			"code": 4034,
			"msg":  "message not found",
		},
	})

	err := runMountedMailShortcut(t, MailShareToChat, []string{
		"+share-to-chat", "--message-id", "bad_id", "--receive-id", "oc_xxx",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for step 1 failure, got nil")
	}
	if !strings.Contains(err.Error(), "create share token") {
		t.Errorf("expected error to mention 'create share token', got %q", err.Error())
	}
}

func TestShareToChatStep2Failure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/share_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"card_id": "card_003",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/share_tokens/card_003/send",
		Body: map[string]interface{}{
			"code": 4046,
			"msg":  "user not in chat",
		},
	})

	err := runMountedMailShortcut(t, MailShareToChat, []string{
		"+share-to-chat", "--message-id", "m1", "--receive-id", "oc_not_in",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for step 2 failure, got nil")
	}
	if !strings.Contains(err.Error(), "card_003") {
		t.Errorf("expected error to contain card_id, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "send failed") {
		t.Errorf("expected error to mention 'send failed', got %q", err.Error())
	}
}

func TestValidReceiveIDTypes(t *testing.T) {
	expected := []string{"chat_id", "open_id", "user_id", "union_id", "email"}
	for _, typ := range expected {
		if !validReceiveIDTypes[typ] {
			t.Errorf("expected %q to be a valid receive ID type", typ)
		}
	}
	if validReceiveIDTypes["invalid"] {
		t.Error("expected 'invalid' to not be a valid receive ID type")
	}
}
