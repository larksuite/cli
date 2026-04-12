// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/cli/internal/core"
)

// TestNewEventSubscriber tests creating a new event subscriber
func TestNewEventSubscriber(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	config := EventSubscriberConfig{
		BotHandler: handler,
		AppID:      "test_app_id",
		AppSecret:  core.PlainSecret("test_secret"),
		Brand:      "feishu",
		Quiet:      true,
	}

	subscriber := NewEventSubscriber(config)
	if subscriber == nil {
		t.Fatal("NewEventSubscriber() returned nil")
	}

	if subscriber.appID != "test_app_id" {
		t.Errorf("appID = %s, want test_app_id", subscriber.appID)
	}

	if subscriber.domain != lark.FeishuBaseUrl {
		t.Errorf("domain = %s, want %s", subscriber.domain, lark.FeishuBaseUrl)
	}
}

// TestNewEventSubscriber_LarkBrand tests Lark brand configuration
func TestNewEventSubscriber_LarkBrand(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	config := EventSubscriberConfig{
		BotHandler: handler,
		AppID:      "test_app_id",
		AppSecret:  core.PlainSecret("test_secret"),
		Brand:      "lark",
		Quiet:      true,
	}

	subscriber := NewEventSubscriber(config)
	if subscriber.domain != lark.LarkBaseUrl {
		t.Errorf("domain = %s, want %s", subscriber.domain, lark.LarkBaseUrl)
	}
}

// TestEventSubscriber_GetStats tests getting subscriber statistics
func TestEventSubscriber_GetStats(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	config := EventSubscriberConfig{
		BotHandler: handler,
		AppID:      "test_app_id",
		AppSecret:  core.PlainSecret("test_secret"),
		Brand:      "feishu",
		Quiet:      true,
	}

	subscriber := NewEventSubscriber(config)
	stats := subscriber.GetStats()

	if stats["events_received"] != 0 {
		t.Errorf("events_received = %v, want 0", stats["events_received"])
	}

	if stats["app_id"] != "test_app_id" {
		t.Errorf("app_id = %v, want test_app_id", stats["app_id"])
	}

	if stats["domain"] != lark.FeishuBaseUrl {
		t.Errorf("domain = %v, want %s", stats["domain"], lark.FeishuBaseUrl)
	}
}

// TestEventSubscriber_info tests info message printing (should not panic with Quiet=true)
func TestEventSubscriber_info(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	subscriber := &EventSubscriber{
		botHandler: handler,
		quiet:      true, // Should suppress output
	}

	// Should not panic with quiet=true
	subscriber.info("This should not be printed")
}

// TestEventSubscriber_info_NotQuiet tests info message printing when not quiet
func TestEventSubscriber_info_NotQuiet(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	subscriber := &EventSubscriber{
		botHandler: handler,
		quiet:      false, // Should print output
	}

	// Should not panic with quiet=false
	subscriber.info("This message will be printed")
}

// TestEventSubscriber_error tests error message printing
func TestEventSubscriber_error(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	subscriber := &EventSubscriber{
		botHandler: handler,
		quiet:      true,
	}

	// Should not panic
	subscriber.error("Test error message")
}

// TestEventSubscriber_debug tests debug message printing
func TestEventSubscriber_debug(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	subscriber := &EventSubscriber{
		botHandler: handler,
		quiet:      true,
	}

	// Should not panic (debug is currently a no-op)
	subscriber.debug("Test debug message")
}

// TestEventSubscriber_createEventHandler tests the event handler factory
func TestEventSubscriber_createEventHandler(t *testing.T) {
	handler, _ := NewBotHandler(BotHandlerConfig{
		ClaudeClient:   NewClaudeClient(ClaudeClientConfig{}),
		SessionManager: nil,
		WorkDir:        "/tmp",
	})

	subscriber := &EventSubscriber{
		botHandler: handler,
		quiet:      true,
	}

	// Create the event handler
	eventHandler := subscriber.createEventHandler()

	// Test with nil event body
	err := eventHandler(context.Background(), &larkevent.EventReq{})
	if err != nil {
		t.Errorf("Event handler returned error for nil body: %v", err)
	}

	// Test with invalid JSON
	err = eventHandler(context.Background(), &larkevent.EventReq{
		Body: []byte("not json"),
	})
	if err != nil {
		t.Errorf("Event handler returned error for invalid JSON: %v", err)
	}

	// Test with missing header
	err = eventHandler(context.Background(), &larkevent.EventReq{
		Body: []byte(`{"event":{}}`),
	})
	if err != nil {
		t.Errorf("Event handler returned error for missing header: %v", err)
	}

	// Test with non-message event type
	err = eventHandler(context.Background(), &larkevent.EventReq{
		Body: []byte(`{"header":{"event_type":"other.event"},"event":{}}`),
	})
	if err != nil {
		t.Errorf("Event handler returned error for non-message event: %v", err)
	}

	// Verify event count
	if subscriber.eventCount != 3 {
		t.Errorf("eventCount = %d, want 3", subscriber.eventCount)
	}
}
