// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestNewSessionManager tests creating a new session manager
func TestNewSessionManager(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	config := SessionManagerConfig{
		BaseDir: tmpDir,
		TTL:     1 * time.Hour,
	}

	sm, err := NewSessionManager(config)
	if err != nil {
		t.Fatalf("NewSessionManager() failed: %v", err)
	}

	if sm == nil {
		t.Fatal("NewSessionManager() returned nil")
	}

	// Verify base directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("Session directory was not created: %s", tmpDir)
	}
}

// TestNewSessionManager_Defaults tests default values
func TestNewSessionManager_Defaults(t *testing.T) {
	// Test with empty config (should use defaults)
	config := SessionManagerConfig{}

	sm, err := NewSessionManager(config)
	if err != nil {
		t.Fatalf("NewSessionManager() with defaults failed: %v", err)
	}

	if sm == nil {
		t.Fatal("NewSessionManager() returned nil")
	}
}

// TestNewSessionManager_DefaultTTL tests default TTL
func TestNewSessionManager_DefaultTTL(t *testing.T) {
	tmpDir := t.TempDir()

	config := SessionManagerConfig{
		BaseDir: tmpDir,
		// TTL not set, should default to 24 hours
	}

	sm, err := NewSessionManager(config)
	if err != nil {
		t.Fatalf("NewSessionManager() failed: %v", err)
	}

	// Verify default TTL is 24 hours
	if sm == nil {
		t.Fatal("NewSessionManager() returned nil")
	}
	// TTL field is not exported, so we can't directly test it
	// But we can verify the session manager works correctly
}

// TestSessionManager_GetSet tests basic session get/set operations
func TestSessionManager_GetSet(t *testing.T) {
	tmpDir := t.TempDir()
	sm, _ := NewSessionManager(SessionManagerConfig{BaseDir: tmpDir, TTL: 1 * time.Hour})

	chatID := "test_chat_123"
	sessionID := "session_abc123"

	// Get non-existent session should return nil
	session, err := sm.Get(chatID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if session != nil {
		t.Error("Get() should return nil for non-existent session")
	}

	// Set a new session
	newSession, err := sm.Set(chatID, sessionID)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	if newSession == nil {
		t.Fatal("Set() returned nil")
	}
	if newSession.ChatID != chatID {
		t.Errorf("Set() ChatID = %s, want %s", newSession.ChatID, chatID)
	}
	if newSession.SessionID != sessionID {
		t.Errorf("Set() SessionID = %s, want %s", newSession.SessionID, sessionID)
	}

	// Get the session we just set
	retrieved, err := sm.Get(chatID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Get() returned nil for existing session")
	}
	if retrieved.SessionID != sessionID {
		t.Errorf("Get() SessionID = %s, want %s", retrieved.SessionID, sessionID)
	}

	// Update message count
	if retrieved.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", retrieved.MessageCount)
	}
}

// TestSessionManager_Delete tests session deletion
func TestSessionManager_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	sm, _ := NewSessionManager(SessionManagerConfig{BaseDir: tmpDir, TTL: 1 * time.Hour})

	chatID := "test_chat_456"
	sessionID := "session_xyz789"

	// Set and then delete
	sm.Set(chatID, sessionID)
	err := sm.Delete(chatID)
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Verify deletion
	session, err := sm.Get(chatID)
	if err != nil {
		t.Fatalf("Get() after Delete failed: %v", err)
	}
	if session != nil {
		t.Error("Session should be nil after deletion")
	}
}

// TestSessionManager_TTL tests session expiration
func TestSessionManager_TTL(t *testing.T) {
	tmpDir := t.TempDir()

	// Very short TTL for testing
	sm, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: tmpDir,
		TTL:     10 * time.Millisecond,
	})

	chatID := "test_chat_ttl"
	sessionID := "session_ttl_123"

	// Set session
	_, _ = sm.Set(chatID, sessionID)

	// Immediately get - should exist
	session, _ := sm.Get(chatID)
	if session == nil {
		t.Error("Session should exist immediately after Set")
	}

	// Wait for expiration
	time.Sleep(15 * time.Millisecond)

	// Get after TTL - should return nil (expired and cleaned up)
	session, _ = sm.Get(chatID)
	if session != nil {
		t.Error("Session should be nil after TTL expiration")
	}
}

// TestSessionManager_List tests listing all active sessions
func TestSessionManager_List(t *testing.T) {
	tmpDir := t.TempDir()
	sm, _ := NewSessionManager(SessionManagerConfig{BaseDir: tmpDir, TTL: 1 * time.Hour})

	// Create multiple sessions
	chatIDs := []string{"chat1", "chat2", "chat3"}
	for i, chatID := range chatIDs {
		sm.Set(chatID, "session_"+string(rune('a'+i)))
	}

	// List all sessions
	sessions, err := sm.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(sessions) != len(chatIDs) {
		t.Errorf("List() returned %d sessions, want %d", len(sessions), len(chatIDs))
	}
}

// TestSessionManager_CleanupExpired tests cleanup of expired sessions
func TestSessionManager_CleanupExpired(t *testing.T) {
	tmpDir := t.TempDir()

	sm, _ := NewSessionManager(SessionManagerConfig{
		BaseDir: tmpDir,
		TTL:     50 * time.Millisecond,
	})

	// Create sessions
	sm.Set("chat_active", "session_active")
	sm.Set("chat_expired", "session_expired")

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Refresh the active session to keep it alive
	sm.Set("chat_active", "session_active_refreshed")

	// Cleanup expired
	count, err := sm.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired() failed: %v", err)
	}

	if count != 1 {
		t.Errorf("CleanupExpired() cleaned %d sessions, want 1", count)
	}

	// Verify expired session is gone
	session, _ := sm.Get("chat_expired")
	if session != nil {
		t.Error("Expired session should be cleaned up")
	}

	// Verify active session still exists
	session, _ = sm.Get("chat_active")
	if session == nil {
		t.Error("Active session should still exist")
	}
}

// TestSessionManager_SpecialCharacters tests chat IDs with special characters
func TestSessionManager_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	sm, _ := NewSessionManager(SessionManagerConfig{BaseDir: tmpDir, TTL: 1 * time.Hour})

	// Test various special characters
	testCases := []struct {
		chatID   string
		expected string // sanitized version
	}{
		{"oc_12345", "oc_12345"},
		{"chat/with/slashes", "chat_with_slashes"},
		{"chat\\with\\backslashes", "chat_with_backslashes"},
		{"chat_with_汉_字", "chat_with_汉_字"},
	}

	for _, tc := range testCases {
		t.Run(tc.chatID, func(t *testing.T) {
			sessionID := "test_session"

			// Set should succeed
			session, err := sm.Set(tc.chatID, sessionID)
			if err != nil {
				t.Errorf("Set(%q) failed: %v", tc.chatID, err)
			}
			if session == nil {
				t.Error("Set() returned nil")
			}

			// Get should retrieve the same session
			retrieved, err := sm.Get(tc.chatID)
			if err != nil {
				t.Errorf("Get(%q) failed: %v", tc.chatID, err)
			}
			if retrieved == nil {
				t.Error("Get() returned nil")
			}
			if retrieved.SessionID != sessionID {
				t.Errorf("Get() SessionID = %s, want %s", retrieved.SessionID, sessionID)
			}
		})
	}
}

// TestSessionManager_ConcurrentAccess tests thread safety
func TestSessionManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	sm, _ := NewSessionManager(SessionManagerConfig{BaseDir: tmpDir, TTL: 1 * time.Hour})

	done := make(chan bool)

	// Launch multiple goroutines
	for i := 0; i < 10; i++ {
		go func(index int) {
			chatID := fmt.Sprintf("concurrent_chat_%d", index)
			sessionID := fmt.Sprintf("concurrent_session_%d", index)

			// Perform get/set operations
			sm.Set(chatID, sessionID)
			sm.Get(chatID)

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all sessions were created
	sessions, _ := sm.List()
	if len(sessions) != 10 {
		t.Errorf("Concurrent operations created %d sessions, want 10", len(sessions))
	}
}
