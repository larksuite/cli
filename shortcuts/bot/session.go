package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionManager manages Claude session persistence
type SessionManager struct {
	mu       sync.RWMutex
	baseDir  string
	ttl      time.Duration
}

// SessionData stores session information
type SessionData struct {
	SessionID   string    `json:"session_id"`
	ChatID      string    `json:"chat_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	MessageCount int      `json:"message_count"`
}

// SessionManagerConfig configures a new SessionManager
type SessionManagerConfig struct {
	BaseDir string        // Base directory for session files
	TTL     time.Duration // Session TTL (default: 24 hours)
}

// NewSessionManager creates a new session manager
func NewSessionManager(config SessionManagerConfig) (*SessionManager, error) {
	if config.BaseDir == "" {
		// Default to ~/.lark-cli/sessions/
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		config.BaseDir = filepath.Join(homeDir, ".lark-cli", "sessions")
	}

	if config.TTL == 0 {
		config.TTL = 24 * time.Hour
	}

	// Ensure base directory exists
	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	return &SessionManager{
		baseDir: config.BaseDir,
		ttl:     config.TTL,
	}, nil
}

// Get retrieves a session by chat ID
func (sm *SessionManager) Get(chatID string) (*SessionData, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessionPath := sm.sessionPath(chatID)

	// Check if file exists
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return nil, nil // Session not found (not an error)
	}

	// Read session file
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session data: %w", err)
	}

	// Check if session has expired
	if sm.isExpired(&session) {
		// Don't delete here to avoid deadlock (Get holds RLock, Delete needs Lock)
		// Cleanup will be handled by CleanupExpired method
		return nil, nil
	}

	return &session, nil
}

// Set saves or updates a session
func (sm *SessionManager) Set(chatID string, sessionID string) (*SessionData, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	sessionPath := sm.sessionPath(chatID)

	// Try to load existing session
	var session SessionData
	if data, err := os.ReadFile(sessionPath); err == nil {
		_ = json.Unmarshal(data, &session)
	}

	// Update session data
	session.SessionID = sessionID
	session.ChatID = chatID
	session.UpdatedAt = now
	session.MessageCount++

	// Set creation time if new session
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}

	// Serialize session data
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize session data: %w", err)
	}

	// Write to file (atomic write)
	tmpPath := sessionPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write session file: %w", err)
	}

	if err := os.Rename(tmpPath, sessionPath); err != nil {
		return nil, fmt.Errorf("failed to atomic rename session file: %w", err)
	}

	return &session, nil
}

// Delete removes a session
func (sm *SessionManager) Delete(chatID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionPath := sm.sessionPath(chatID)

	if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	return nil
}

// List returns all active sessions
func (sm *SessionManager) List() ([]SessionData, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []SessionData
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Extract chat_id from filename
		chatID := strings.TrimSuffix(entry.Name(), ".json")

		session, err := sm.Get(chatID)
		if err != nil {
			continue // Skip invalid sessions
		}

		if session != nil {
			sessions = append(sessions, *session)
		}
	}

	return sessions, nil
}

// CleanupExpired removes all expired sessions
func (sm *SessionManager) CleanupExpired() (int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Read directory entries directly to avoid calling List (which needs RLock)
	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Extract chat ID from filename
		chatID := strings.TrimSuffix(entry.Name(), ".json")
		chatID = strings.ReplaceAll(chatID, "_", "/") // Reverse sanitization

		// Read session file
		sessionPath := sm.sessionPath(chatID)
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			continue
		}

		var session SessionData
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		// Check if expired and delete
		if sm.isExpired(&session) {
			sessionPath := sm.sessionPath(chatID)
			if err := os.Remove(sessionPath); err == nil {
				cleaned++
			}
		}
	}

	return cleaned, nil
}

// isExpired checks if a session has exceeded its TTL
func (sm *SessionManager) isExpired(session *SessionData) bool {
	return time.Since(session.UpdatedAt) > sm.ttl
}

// sessionPath returns the file path for a given chat ID
func (sm *SessionManager) sessionPath(chatID string) string {
	// Sanitize chat_id for filename (replace special chars)
	safeChatID := strings.ReplaceAll(chatID, "/", "_")
	safeChatID = strings.ReplaceAll(safeChatID, "\\", "_")
	return filepath.Join(sm.baseDir, safeChatID+".json")
}
