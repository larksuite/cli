// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestNewClaudeClient tests creating a new Claude client
func TestNewClaudeClient(t *testing.T) {
	config := ClaudeClientConfig{
		WorkDir:         "/tmp/test-claude-bot",
		Timeout:         30 * time.Second,
		MaxRetries:      2,
		SkipPermissions: true,
	}

	client := NewClaudeClient(config)
	if client == nil {
		t.Fatal("NewClaudeClient() returned nil")
	}

	if client.workDir != config.WorkDir {
		t.Errorf("workDir = %s, want %s", client.workDir, config.WorkDir)
	}
	if client.timeout != config.Timeout {
		t.Errorf("timeout = %v, want %v", client.timeout, config.Timeout)
	}
	if client.maxRetries != config.MaxRetries {
		t.Errorf("maxRetries = %d, want %d", client.maxRetries, config.MaxRetries)
	}
}

// TestNewClaudeClient_Defaults tests default values
func TestNewClaudeClient_Defaults(t *testing.T) {
	client := NewClaudeClient(ClaudeClientConfig{})

	if client.workDir != "" {
		t.Errorf("default workDir should be empty, got %s", client.workDir)
	}
	if client.timeout != 5*time.Minute {
		t.Errorf("default timeout = %v, want 5m", client.timeout)
	}
	if client.maxRetries != 3 {
		t.Errorf("default maxRetries = %d, want 3", client.maxRetries)
	}
}

// TestClaudeClient_isRetryableError tests retry logic
func TestClaudeClient_isRetryableError(t *testing.T) {
	client := NewClaudeClient(ClaudeClientConfig{})

	tests := []struct {
		name     string
		errMsg   string
		retryable bool
	}{
		{
			name:     "timeout error",
			errMsg:   "context deadline exceeded",
			retryable: true,
		},
		{
			name:     "connection refused",
			errMsg:   "connection refused",
			retryable: true,
		},
		{
			name:     "temporary failure",
			errMsg:   "temporary failure",
			retryable: true,
		},
		{
			name:     "permanent error",
			errMsg:   "invalid argument",
			retryable: false,
		},
		{
			name:     "nil error",
			errMsg:   "",
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = &testError{msg: tt.errMsg}
			}

			result := client.isRetryableError(err)
			if result != tt.retryable {
				t.Errorf("isRetryableError(%q) = %v, want %v", tt.errMsg, result, tt.retryable)
			}
		})
	}
}

// TestClaudeResponse tests Claude response JSON parsing
func TestClaudeResponse(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantErr  bool
		wantResult string
		wantSessionID string
	}{
		{
			name: "valid response",
			json: `{"result": "Hello!", "session_id": "sess123"}`,
			wantErr: false,
			wantResult: "Hello!",
			wantSessionID: "sess123",
		},
		{
			name: "response with error",
			json: `{"error": "Something went wrong"}`,
			wantErr: true,
		},
		{
			name: "invalid JSON",
			json: `{invalid json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp ClaudeResponse
			err := json.Unmarshal([]byte(tt.json), &resp)

			if tt.wantErr && err == nil && resp.Error == "" {
				t.Error("response should contain error field")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("json.Unmarshal() failed: %v", err)
			}

			if !tt.wantErr {
				if resp.Result != tt.wantResult {
					t.Errorf("Result = %s, want %s", resp.Result, tt.wantResult)
				}
				if resp.SessionID != tt.wantSessionID {
					t.Errorf("SessionID = %s, want %s", resp.SessionID, tt.wantSessionID)
				}
			}
		})
	}
}

// TestClaudeClient_ProcessMessage_ContextCancellation tests context cancellation
func TestClaudeClient_ProcessMessage_ContextCancellation(t *testing.T) {
	client := NewClaudeClient(ClaudeClientConfig{
		Timeout:    10 * time.Second,
		MaxRetries: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// This should fail fast due to context cancellation
	_, err := client.ProcessMessage(ctx, "test message", "")
	if err == nil {
		t.Error("ProcessMessage() should fail with cancelled context")
	}
}

// TestValidateClaudeCLI tests Claude CLI validation
func TestValidateClaudeCLI(t *testing.T) {
	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ValidateClaudeCLI(ctx)
	if err == nil {
		t.Error("ValidateClaudeCLI() should fail with cancelled context")
	}

	// Test with expired context
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond)

	err = ValidateClaudeCLI(ctx)
	if err == nil {
		t.Error("ValidateClaudeCLI() should fail with expired context")
	}
}

// TestClaudeClient_processMessageOnce tests the internal retry logic
func TestClaudeClient_processMessageOnce(t *testing.T) {
	client := NewClaudeClient(ClaudeClientConfig{
		MaxRetries: 2,
	})
	ctx := context.Background()

	// Test with empty message
	_, err := client.processMessageOnce(ctx, "", "")
	if err == nil {
		t.Error("processMessageOnce() should fail with empty message")
	}
}

// Helper types for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
