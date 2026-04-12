// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"testing"
)

// TestNewRouter tests creating a new router
func TestNewRouter(t *testing.T) {
	config := RouterConfig{
		EnableWhitelist: false,
	}

	router := NewRouter(config)
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}

	if len(router.commands) == 0 {
		t.Error("NewRouter() should register built-in commands")
	}
}

// TestRouter_RegisterCommand tests registering custom commands
func TestRouter_RegisterCommand(t *testing.T) {
	router := NewRouter(RouterConfig{EnableWhitelist: false})

	handler := func(ctx context.Context, args []string, chatID string) (string, error) {
		return "ok", nil
	}

	// Register a new command
	err := router.RegisterCommand("test", handler)
	if err != nil {
		t.Fatalf("RegisterCommand() failed: %v", err)
	}

	// Verify command was registered
	commands := router.ListCommands()
	found := false
	for _, cmd := range commands {
		if cmd == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Registered command not found in ListCommands()")
	}
}

// TestRouter_RegisterCommand_EmptyName tests error handling for empty command name
func TestRouter_RegisterCommand_EmptyName(t *testing.T) {
	router := NewRouter(RouterConfig{EnableWhitelist: false})

	err := router.RegisterCommand("", func(ctx context.Context, args []string, chatID string) (string, error) {
		return "", nil
	})

	if err == nil {
		t.Error("RegisterCommand() should fail with empty command name")
	}
}

// TestRouter_RegisterCommand_Whitelist tests whitelist enforcement
func TestRouter_RegisterCommand_Whitelist(t *testing.T) {
	config := RouterConfig{
		EnableWhitelist:      true,
		WhitelistedCommands: []string{"status", "help"},
	}

	router := NewRouter(config)

	// Try to register a command not in whitelist
	err := router.RegisterCommand("unauthorized", func(ctx context.Context, args []string, chatID string) (string, error) {
		return "", nil
	})

	if err == nil {
		t.Error("RegisterCommand() should fail for non-whitelisted command")
	}
}

// TestRouter_RegisterAlias tests registering command aliases
func TestRouter_RegisterAlias(t *testing.T) {
	router := NewRouter(RouterConfig{EnableWhitelist: false})

	// Register target command first
	router.RegisterCommand("target", func(ctx context.Context, args []string, chatID string) (string, error) {
		return "target", nil
	})

	// Register alias
	err := router.RegisterAlias("alias", "target")
	if err != nil {
		t.Fatalf("RegisterAlias() failed: %v", err)
	}

	// Test routing through alias
	response, err := router.Route(context.Background(), "/alias", "chat123")
	if err != nil {
		t.Fatalf("Route() failed: %v", err)
	}
	if response != "target" {
		t.Errorf("Route() through alias returned %s, want 'target'", response)
	}
}

// TestRouter_RegisterAlias_NonExistentTarget tests error handling for alias to non-existent command
func TestRouter_RegisterAlias_NonExistentTarget(t *testing.T) {
	router := NewRouter(RouterConfig{EnableWhitelist: false})

	err := router.RegisterAlias("alias", "nonexistent")
	if err == nil {
		t.Error("RegisterAlias() should fail for non-existent target command")
	}
}

// TestRouter_Route_BuiltInCommands tests built-in command routing
func TestRouter_Route_BuiltInCommands(t *testing.T) {
	router := NewRouter(RouterConfig{EnableWhitelist: false})

	tests := []struct {
		name     string
		message  string
		wantOK   bool
		wantResp string
	}{
		{
			name:     "status command",
			message:  "/status",
			wantOK:   true,
			wantResp: "Bot is running",
		},
		{
			name:     "help command",
			message:  "/help",
			wantOK:   true,
			wantResp: "Available commands",
		},
		{
			name:     "clear command",
			message:  "/clear",
			wantOK:   true,
			wantResp: "Session cleared",
		},
		{
			name:     "unknown command",
			message:  "/unknown",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := router.Route(context.Background(), tt.message, "chat123")

			if tt.wantOK && err != nil {
				t.Errorf("Route(%q) failed: %v", tt.message, err)
			}
			if !tt.wantOK && err == nil {
				t.Errorf("Route(%q) should fail for unknown command", tt.message)
			}
			if tt.wantOK && response == "" {
				t.Errorf("Route(%q) returned empty response", tt.message)
			}
			if tt.wantResp != "" && response != "" {
				// Check if response contains expected substring
				if len(response) < len(tt.wantResp) || response[:len(tt.wantResp)] != tt.wantResp {
					t.Errorf("Route(%q) response = %s, should contain %s", tt.message, response, tt.wantResp)
				}
			}
		})
	}
}

// TestRouter_Route_NonCommand tests routing non-command messages
func TestRouter_Route_NonCommand(t *testing.T) {
	called := false
	defaultHandler := func(ctx context.Context, args []string, chatID string) (string, error) {
		called = true
		return "default", nil
	}

	router := NewRouter(RouterConfig{
		DefaultHandler: defaultHandler,
	})

	// Test plain message (no slash)
	response, err := router.Route(context.Background(), "plain message", "chat123")
	if err != nil {
		t.Fatalf("Route() failed for plain message: %v", err)
	}
	if !called {
		t.Error("DefaultHandler should be called for plain message")
	}
	if response != "default" {
		t.Errorf("Route() returned %s, want 'default'", response)
	}
}

// TestRouter_Route_CommandWithArgs tests command with arguments
func TestRouter_Route_CommandWithArgs(t *testing.T) {
	argsReceived := []string{}
	handler := func(ctx context.Context, args []string, chatID string) (string, error) {
		argsReceived = args
		return "handled", nil
	}

	router := NewRouter(RouterConfig{
		DefaultHandler: handler,
	})

	// Register a custom command to test argument parsing
	router.RegisterCommand("testcmd", func(ctx context.Context, args []string, chatID string) (string, error) {
		argsReceived = args
		return "handled", nil
	})

	// Route command with arguments
	_, err := router.Route(context.Background(), "/testcmd verbose", "chat123")
	if err != nil {
		t.Fatalf("Route() failed: %v", err)
	}

	if len(argsReceived) != 1 || argsReceived[0] != "verbose" {
		t.Errorf("Route() args = %v, want [verbose]", argsReceived)
	}
}

// TestPatternRouter tests pattern-based routing
func TestPatternRouter(t *testing.T) {
	pr := NewPatternRouter()

	// Add a pattern for URLs
	called := false
	handler := func(ctx context.Context, args []string, chatID string) (string, error) {
		called = true
		return "url_detected", nil
	}

	err := pr.AddPattern(`https?://\S+`, handler, 10)
	if err != nil {
		t.Fatalf("AddPattern() failed: %v", err)
	}

	// Test URL message
	response, err := pr.Route(context.Background(), "Check out https://example.com", "chat123")
	if err != nil {
		t.Fatalf("Route() failed: %v", err)
	}
	if !called {
		t.Error("Pattern handler should be called for URL message")
	}
	if response != "url_detected" {
		t.Errorf("Route() returned %s, want 'url_detected'", response)
	}
}

// TestPatternRouter_Priority tests pattern priority ordering
func TestPatternRouter_Priority(t *testing.T) {
	pr := NewPatternRouter()

	// Add patterns with different priorities
	lowPriorityCalled := false
	highPriorityCalled := false

	pr.AddPattern(`test.*`, func(ctx context.Context, args []string, chatID string) (string, error) {
		lowPriorityCalled = true
		return "low", nil
	}, 1)

	pr.AddPattern(`test_exact`, func(ctx context.Context, args []string, chatID string) (string, error) {
		highPriorityCalled = true
		return "high", nil
	}, 10)

	// Route message that matches both
	response, _ := pr.Route(context.Background(), "test_exact", "chat123")

	if !highPriorityCalled {
		t.Error("High priority pattern should be matched first")
	}
	if lowPriorityCalled {
		t.Error("Low priority pattern should not be called when high priority matches")
	}
	if response != "high" {
		t.Errorf("Route() returned %s, want 'high'", response)
	}
}

// TestPatternRouter_Fallback tests fallback handler
func TestPatternRouter_Fallback(t *testing.T) {
	pr := NewPatternRouter()

	fallbackCalled := false
	fallback := func(ctx context.Context, args []string, chatID string) (string, error) {
		fallbackCalled = true
		return "fallback", nil
	}

	pr.SetFallback(fallback)

	// Route message that doesn't match any pattern
	response, err := pr.Route(context.Background(), "nomatch", "chat123")
	if err != nil {
		t.Fatalf("Route() failed: %v", err)
	}

	if !fallbackCalled {
		t.Error("Fallback should be called for unmatched message")
	}
	if response != "fallback" {
		t.Errorf("Route() returned %s, want 'fallback'", response)
	}
}
