// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"testing"
)

func TestShareToChatValidation(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{
			name:    "missing both message-id and thread-id",
			flags:   map[string]string{"receive-id": "oc_xxx", "receive-id-type": "chat_id"},
			wantErr: "either --message-id or --thread-id is required",
		},
		{
			name:    "both message-id and thread-id",
			flags:   map[string]string{"message-id": "m1", "thread-id": "t1", "receive-id": "oc_xxx", "receive-id-type": "chat_id"},
			wantErr: "--message-id and --thread-id are mutually exclusive",
		},
		{
			name:    "invalid receive-id-type",
			flags:   map[string]string{"message-id": "m1", "receive-id": "oc_xxx", "receive-id-type": "invalid"},
			wantErr: "--receive-id-type must be one of",
		},
		{
			name:  "valid with message-id and chat_id",
			flags: map[string]string{"message-id": "m1", "receive-id": "oc_xxx", "receive-id-type": "chat_id"},
		},
		{
			name:  "valid with thread-id and email",
			flags: map[string]string{"thread-id": "t1", "receive-id": "user@example.com", "receive-id-type": "email"},
		},
		{
			name:  "valid with open_id",
			flags: map[string]string{"message-id": "m1", "receive-id": "ou_xxx", "receive-id-type": "open_id"},
		},
		{
			name:  "valid with user_id",
			flags: map[string]string{"message-id": "m1", "receive-id": "uid", "receive-id-type": "user_id"},
		},
		{
			name:  "valid with union_id",
			flags: map[string]string{"message-id": "m1", "receive-id": "on_xxx", "receive-id-type": "union_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShareToChat(tt.flags)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
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

// validateShareToChat extracts the validation logic for unit testing
// without needing a full RuntimeContext.
func validateShareToChat(flags map[string]string) error {
	msgID := flags["message-id"]
	threadID := flags["thread-id"]
	if msgID == "" && threadID == "" {
		return fmt.Errorf("either --message-id or --thread-id is required")
	}
	if msgID != "" && threadID != "" {
		return fmt.Errorf("--message-id and --thread-id are mutually exclusive")
	}
	idType := flags["receive-id-type"]
	if !validReceiveIDTypes[idType] {
		return fmt.Errorf("--receive-id-type must be one of: chat_id, open_id, user_id, union_id, email")
	}
	return nil
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
