// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"encoding/json"
	"testing"
)

// TestNewMessageSender tests creating a new message sender
func TestNewMessageSender(t *testing.T) {
	sender := NewMessageSender()
	if sender == nil {
		t.Fatal("NewMessageSender() returned nil")
	}
}

// TestMessageSender_buildMessageContent tests building message content
func TestMessageSender_buildMessageContent(t *testing.T) {
	sender := NewMessageSender()

	tests := []struct {
		name    string
		message string
		wantLen int // Minimum expected JSON length
	}{
		{
			name:    "simple text",
			message: "Hello world",
			wantLen: 20,
		},
		{
			name:    "empty message",
			message: "",
			wantLen: 10, // {"text":""} is 11 chars
		},
		{
			name:    "long message",
			message: "This is a longer message with more content",
			wantLen: 30,
		},
		{
			name:    "message with newlines",
			message: "Line 1\nLine 2\nLine 3",
			wantLen: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := sender.buildMessageContent(tt.message)

			if err != nil {
				t.Errorf("buildMessageContent() failed: %v", err)
			}

			if len(content) < tt.wantLen {
				t.Errorf("buildMessageContent() length = %d, want >= %d", len(content), tt.wantLen)
			}

			// Verify it's valid JSON and contains text field
			var result map[string]string
			if err := json.Unmarshal([]byte(content), &result); err != nil {
				t.Error("buildMessageContent() returned invalid JSON")
			}
			if result["text"] != tt.message {
				t.Errorf("buildMessageContent() text field = %s, want %s", result["text"], tt.message)
			}
		})
	}
}

// TestMessageSender_SendMessage_NilClient tests error handling when Lark client is nil
func TestMessageSender_SendMessage_NilClient(t *testing.T) {
	sender := NewMessageSender() // nil client
	ctx := context.Background()

	tests := []struct {
		name      string
		chatID    string
		message   string
		parentMsg string
		wantErr   bool
	}{
		{
			name:    "nil client",
			chatID:  "oc_test",
			message: "hello",
			parentMsg: "",
			wantErr: true, // nil larkClient causes error
		},
		{
			name:    "empty chat ID with nil client",
			chatID:  "",
			message: "test",
			parentMsg: "",
			wantErr: true, // empty chat_id causes error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sender.SendMessage(ctx, tt.chatID, tt.message, tt.parentMsg)
			if tt.wantErr && err == nil {
				t.Error("SendMessage() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("SendMessage() unexpected error: %v", err)
			}
		})
	}
}

// Helper function to check if string is valid JSON
func jsonValid(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
