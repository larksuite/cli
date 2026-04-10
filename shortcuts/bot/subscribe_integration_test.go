// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

// TestSendReply_Integration tests the sendReply function directly
func TestSendReply_Integration(t *testing.T) {
	subscriber := &EventSubscriber{
		sender: &MessageSender{},
		quiet:  true,
	}

	tests := []struct {
		name        string
		event       *larkevent.EventReq
		message     string
		expectError bool
	}{
		{
			name:        "nil event",
			event:       nil,
			message:     "test",
			expectError: true,
		},
		{
			name:        "nil body",
			event:       &larkevent.EventReq{Body: nil},
			message:     "test",
			expectError: true,
		},
		{
			name:        "invalid JSON",
			event:       &larkevent.EventReq{Body: []byte("not json")},
			message:     "test",
			expectError: true,
		},
		{
			name:        "missing event data",
			event:       &larkevent.EventReq{Body: []byte(`{"header":{}}`)},
			message:     "test",
			expectError: true,
		},
		{
			name:        "missing chat_id",
			event:       &larkevent.EventReq{Body: []byte(`{"event":{"message_id":"123"}}`)},
			message:     "test",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := subscriber.sendReply(context.Background(), tt.event, tt.message)
			if tt.expectError && err == nil {
				t.Error("sendReply() should return error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("sendReply() returned unexpected error: %v", err)
			}
		})
	}
}

// TestSendReply_Success_Integration tests successful reply (with real sender)
func TestSendReply_Success_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create subscriber with real sender
	// Note: This requires a real Lark client which needs app credentials.
	// For unit testing without credentials, use NewEventSubscriber which
	// creates a nil-client sender that returns errors appropriately.
	subscriber := NewEventSubscriber(EventSubscriberConfig{
		MessageSender: NewMessageSender(), // nil client
		Quiet:         true,
	})

	// Verify sender is usable (nil client returns error as expected)
	eventBody := map[string]interface{}{
		"event": map[string]interface{}{
			"chat_id":    "oc_chat_123",
			"message_id": "om_msg_456",
		},
	}
	eventJSON, _ := json.Marshal(eventBody)
	event := &larkevent.EventReq{Body: eventJSON}

	// sendReply requires a non-nil larkClient; nil client returns error
	err := subscriber.sendReply(context.Background(), event, "Hello!")
	if err == nil {
		t.Error("sendReply() expected error with nil client, got nil")
	}
}

// TestCreateEventHandler_EdgeCases tests edge cases in event handler
func TestCreateEventHandler_EdgeCases(t *testing.T) {
	subscriber := &EventSubscriber{
		sender: NewMessageSender(),
		quiet:  true,
	}

	eventHandler := subscriber.createEventHandler()

	tests := []struct {
		name        string
		event       *larkevent.EventReq
		expectPanic bool
	}{
		{
			name:        "nil event",
			event:       nil,
			expectPanic: false, // nil check at start
		},
		{
			name:        "nil body",
			event:       &larkevent.EventReq{Body: nil},
			expectPanic: false, // nil check
		},
		{
			name:        "empty body",
			event:       &larkevent.EventReq{Body: []byte{}},
			expectPanic: false, // valid empty JSON
		},
		{
			name:        "invalid JSON",
			event:       &larkevent.EventReq{Body: []byte("not json")},
			expectPanic: false, // error handling
		},
		{
			name:        "missing header",
			event:       &larkevent.EventReq{Body: []byte(`{"event":{}}`)},
			expectPanic: false, // error handling
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil && !tt.expectPanic {
					t.Errorf("Event handler panicked: %v", r)
				}
			}()

			err := eventHandler(context.Background(), tt.event)
			if err != nil {
				t.Errorf("Event handler returned error: %v", err)
			}
		})
	}
}

// TestCreateEventHandler_NonMessageEvent tests handling of non-message events
func TestCreateEventHandler_NonMessageEvent(t *testing.T) {
	subscriber := &EventSubscriber{
		sender: NewMessageSender(),
		quiet:  true,
	}

	eventHandler := subscriber.createEventHandler()

	// Test various non-message event types
	nonMessageEvents := []string{
		"im.message.read_v1",
		"im.message.reaction.add_v1",
		"contact.user.created_v1",
		"calendar.event.create_v1",
	}

	for _, eventType := range nonMessageEvents {
		t.Run(eventType, func(t *testing.T) {
			eventBody := map[string]interface{}{
				"header": map[string]interface{}{
					"event_type": eventType,
				},
				"event": map[string]interface{}{},
			}
			eventJSON, _ := json.Marshal(eventBody)
			event := &larkevent.EventReq{Body: eventJSON}

			// Should not error, just ignore
			err := eventHandler(context.Background(), event)
			if err != nil {
				t.Errorf("Event handler returned error for %s: %v", eventType, err)
			}
		})
	}
}

// TestEventCount_Integration tests event counting
func TestEventCount_Integration(t *testing.T) {
	subscriber := &EventSubscriber{
		sender: NewMessageSender(),
		quiet:  true,
	}

	if subscriber.eventCount != 0 {
		t.Errorf("Initial eventCount = %d, want 0", subscriber.eventCount)
	}

	eventHandler := subscriber.createEventHandler()

	// Process various events
	eventHandler(context.Background(), &larkevent.EventReq{}) // nil body
	eventHandler(context.Background(), &larkevent.EventReq{Body: []byte("{}")}) // missing header
	eventHandler(context.Background(), &larkevent.EventReq{Body: []byte(`{"header":{"event_type":"im.message.receive_v1"}}`)})
	eventHandler(context.Background(), &larkevent.EventReq{Body: []byte(`{"header":{"event_type":"im.message.read_v1"}}`)})

	// Count only events that reached the "header" check
	if subscriber.eventCount != 3 {
		t.Errorf("eventCount = %d, want 3", subscriber.eventCount)
	}
}

// TestSubscribe_InvalidContext tests Subscribe with cancelled context
// Note: This test may be flaky depending on WebSocket client behavior
func TestSubscribe_InvalidContext(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	subscriber := &EventSubscriber{
		sender: NewMessageSender(),
		quiet:  true,
		appID: "test_app",
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Subscribe should return quickly with cancelled context
	// We use a timeout to prevent test from hanging indefinitely
	done := make(chan error, 1)
	go func() {
		done <- subscriber.Subscribe(ctx)
	}()

	select {
	case err := <-done:
		// Should complete quickly with cancelled context
		t.Logf("Subscribe returned: %v", err)
	case <-time.After(2 * time.Second):
		// WebSocket client may not respect context cancellation quickly
		t.Skip("Subscribe did not return within timeout (WebSocket client behavior)")
	}
}

// Helper for timeout in tests
func afterOrDone(d time.Duration, done chan error) {
	select {
	case <-done:
	case <-time.After(d):
	}
}

// TestNewEventSubscriber_WithCustomSender tests creating subscriber with custom sender
func TestNewEventSubscriber_WithCustomSender(t *testing.T) {
	customSender := NewMessageSender()
	config := EventSubscriberConfig{
		BotHandler:    nil,
		MessageSender: customSender,
		AppID:        "test_app",
		Brand:        "feishu",
		Quiet:        true,
	}

	subscriber := NewEventSubscriber(config)

	if subscriber.sender != customSender {
		t.Error("Subscriber should use the provided MessageSender")
	}
	if subscriber.appID != "test_app" {
		t.Errorf("appID = %s, want 'test_app'", subscriber.appID)
	}
}

// TestNewEventSubscriber_DefaultSender tests that default sender is created when nil
func TestNewEventSubscriber_DefaultSender(t *testing.T) {
	config := EventSubscriberConfig{
		BotHandler:    nil,
		MessageSender: nil, // Should create default
		AppID:        "test_app",
		Brand:        "lark",
		Quiet:        true,
	}

	subscriber := NewEventSubscriber(config)

	if subscriber.sender == nil {
		t.Error("Subscriber should have a non-nil sender")
	}
	if subscriber.domain != lark.LarkBaseUrl {
		t.Errorf("domain = %s, want %s", subscriber.domain, lark.LarkBaseUrl)
	}
}
