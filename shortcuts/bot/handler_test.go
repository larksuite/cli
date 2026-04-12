// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

// TestNewBotHandler tests creating a new bot handler
func TestNewBotHandler(t *testing.T) {
	claudeClient := NewClaudeClient(ClaudeClientConfig{})
	sessionMgr, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: t.TempDir(),
		TTL:     1 * time.Hour,
	})

	config := BotHandlerConfig{
		ClaudeClient:   claudeClient,
		SessionManager: sessionMgr,
		WorkDir:        "/tmp/test",
	}

	handler, err := NewBotHandler(config)
	if err != nil {
		t.Fatalf("NewBotHandler() failed: %v", err)
	}
	if handler == nil {
		t.Fatal("NewBotHandler() returned nil")
	}
}

// TestNewBotHandler_MissingClaudeClient tests error handling
func TestNewBotHandler_MissingClaudeClient(t *testing.T) {
	sessionMgr, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: t.TempDir(),
		TTL:     1 * time.Hour,
	})

	config := BotHandlerConfig{
		SessionManager: sessionMgr,
		WorkDir:        "/tmp/test",
	}

	_, err := NewBotHandler(config)
	if err == nil {
		t.Error("NewBotHandler() should fail without ClaudeClient")
	}
}

// TestNewBotHandler_MissingSessionManager tests error handling
func TestNewBotHandler_MissingSessionManager(t *testing.T) {
	claudeClient := NewClaudeClient(ClaudeClientConfig{})

	config := BotHandlerConfig{
		ClaudeClient: claudeClient,
		WorkDir:      "/tmp/test",
	}

	_, err := NewBotHandler(config)
	if err == nil {
		t.Error("NewBotHandler() should fail without SessionManager")
	}
}

// TestBotHandler_extractTextContent tests text content extraction
func TestBotHandler_extractTextContent(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil, // OK for this test
		WorkDir:        "/tmp",
	})

	tests := []struct {
		name         string
		content      string
		messageType  string
		wantText     string
	}{
		{
			name:        "plain text message",
			content:     `{"text":"Hello world"}`,
			messageType: "text",
			wantText:    "Hello world",
		},
		{
			name:        "post message",
			content:     `{"post":{"content":[{"text":"Line 1"},{"text":"Line 2"}]}}`,
			messageType: "post",
			wantText:    "Line 1\nLine 2\n",
		},
		{
			name:        "empty text",
			content:     `{"text":""}`,
			messageType: "text",
			wantText:    "",
		},
		{
			name:        "invalid JSON",
			content:     `not json`,
			messageType: "text",
			wantText:    "not json",
		},
		{
			name:        "unknown message type",
			content:     `{"data":"value"}`,
			messageType: "unknown",
			wantText:    "map[data:value]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractTextContent(tt.content, tt.messageType)

			if result != tt.wantText {
				t.Errorf("extractTextContent() = %s, want %s", result, tt.wantText)
			}
		})
	}
}

// TestBotHandler_parseMessageEvent tests message event parsing
func TestBotHandler_parseMessageEvent(t *testing.T) {
	sessionMgr, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: t.TempDir(),
		TTL:     1 * time.Hour,
	})

	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: sessionMgr,
		WorkDir:        "/tmp",
	})

	// Create a mock event
	eventBody := map[string]interface{}{
		"header": map[string]interface{}{
			"event_type": "im.message.receive_v1",
			"create_time": "1234567890",
		},
		"event": map[string]interface{}{
			"chat_id":    "oc_test123",
			"message_id": "om_msg456",
			"sender": map[string]interface{}{
				"sender_id":   "user_789",
				"sender_type": "user",
			},
			"message_type": "text",
			"content":     `{"text":"Test message"}`,
		},
	}

	eventJSON, _ := json.Marshal(eventBody)
	event := &larkevent.EventReq{
		Body: eventJSON,
	}

	msgEvent, err := handler.parseMessageEvent(event)
	if err != nil {
		t.Fatalf("parseMessageEvent() failed: %v", err)
	}

	if msgEvent.ChatID != "oc_test123" {
		t.Errorf("ChatID = %s, want oc_test123", msgEvent.ChatID)
	}
	if msgEvent.MessageID != "om_msg456" {
		t.Errorf("MessageID = %s, want om_msg456", msgEvent.MessageID)
	}
	if msgEvent.SenderID != "user_789" {
		t.Errorf("SenderID = %s, want user_789", msgEvent.SenderID)
	}
	if msgEvent.MessageType != "text" {
		t.Errorf("MessageType = %s, want text", msgEvent.MessageType)
	}
	if msgEvent.Content != "Test message" {
		t.Errorf("Content = %s, want 'Test message'", msgEvent.Content)
	}
}

// TestBotHandler_parseMessageEvent_MissingFields tests error handling for incomplete events
func TestBotHandler_parseMessageEvent_MissingFields(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	tests := []struct {
		name     string
		event    *larkevent.EventReq
		wantErr  bool
	}{
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
		{
			name:    "nil body",
			event:   &larkevent.EventReq{},
			wantErr: true,
		},
		{
			name: "invalid JSON body",
			event: &larkevent.EventReq{
				Body: []byte("invalid json"),
			},
			wantErr: true,
		},
		{
			name: "missing event data",
			event: &larkevent.EventReq{
				Body: []byte(`{"header":{}}`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler.parseMessageEvent(tt.event)

			if tt.wantErr && err == nil {
				t.Error("parseMessageEvent() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseMessageEvent() failed: %v", err)
			}
		})
	}
}

// TestBotHandler_GetStats tests statistics retrieval
func TestBotHandler_GetStats(t *testing.T) {
	sessionMgr, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: t.TempDir(),
		TTL:     1 * time.Hour,
	})

	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: sessionMgr,
		WorkDir:        "/tmp/test",
	})

	stats, err := handler.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() failed: %v", err)
	}

	if stats["active_sessions"] != 0 {
		t.Errorf("active_sessions = %v, want 0", stats["active_sessions"])
	}
	if stats["work_dir"] != "/tmp/test" {
		t.Errorf("work_dir = %s, want /tmp/test", stats["work_dir"])
	}
}

// TestBotHandler_HandleMessage tests the main message handling flow
func TestBotHandler_HandleMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode - requires Claude CLI")
	}

	sessionMgr, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: t.TempDir(),
		TTL:     1 * time.Hour,
	})

	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: sessionMgr,
		WorkDir:        "/tmp",
	})

	// Create a mock event
	eventBody := map[string]interface{}{
		"header": map[string]interface{}{
			"event_type": "im.message.receive_v1",
		},
		"event": map[string]interface{}{
			"chat_id":      "oc_test123",
			"message_id":   "om_msg456",
			"sender": map[string]interface{}{
				"sender_id":   "user_789",
				"sender_type": "user",
			},
			"message_type": "text",
			"content":      `{"text":"Test message"}`,
		},
	}

	eventJSON, _ := json.Marshal(eventBody)
	event := &larkevent.EventReq{
		Body: eventJSON,
	}

	// Handle message with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := handler.HandleMessage(ctx, event)

	// We expect an error since claude CLI is not configured or times out
	if err == nil && response == "" {
		t.Error("HandleMessage() should return either error or response")
	}
}

// TestBotHandler_HandleMessage_EmptyContent tests handling empty messages
func TestBotHandler_HandleMessage_EmptyContent(t *testing.T) {
	sessionMgr, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: t.TempDir(),
		TTL:     1 * time.Hour,
	})

	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: sessionMgr,
		WorkDir:        "/tmp",
	})

	// Create event with empty content
	eventBody := map[string]interface{}{
		"header": map[string]interface{}{
			"event_type": "im.message.receive_v1",
		},
		"event": map[string]interface{}{
			"chat_id":      "oc_test_empty",
			"message_id":   "om_empty",
			"sender": map[string]interface{}{
				"sender_id": "user_123",
			},
			"message_type": "text",
			"content":      `{"text":""}`,
		},
	}

	eventJSON, _ := json.Marshal(eventBody)
	event := &larkevent.EventReq{
		Body: eventJSON,
	}

	// Handle empty message
	_, err := handler.HandleMessage(context.Background(), event)

	// Empty messages should return empty response without error
	if err != nil {
		t.Errorf("HandleMessage() with empty content should not error, got: %v", err)
	}
}
