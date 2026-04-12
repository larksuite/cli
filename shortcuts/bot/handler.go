package bot

import (
	"context"
	"encoding/json"
	"fmt"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

// MessageEvent represents a parsed Lark message event
type MessageEvent struct {
	ChatID     string `json:"chat_id"`
	MessageID  string `json:"message_id"`
	SenderID   string `json:"sender_id"`
	Content    string `json:"content"`
	MessageType string `json:"message_type"`
CreateTime  string `json:"create_time"`
}

// BotHandler handles Lark bot message events and routes them to Claude
type BotHandler struct {
	claudeClient  *ClaudeClient
	sessionMgr    *SessionManager
	workDir       string
}

// BotHandlerConfig configures a new BotHandler
type BotHandlerConfig struct {
	ClaudeClient   *ClaudeClient
	SessionManager *SessionManager
	WorkDir        string
}

// NewBotHandler creates a new bot handler
func NewBotHandler(config BotHandlerConfig) (*BotHandler, error) {
	if config.ClaudeClient == nil {
		return nil, fmt.Errorf("claudeClient is required")
	}
	if config.SessionManager == nil {
		return nil, fmt.Errorf("sessionManager is required")
	}
	if config.WorkDir == "" {
		config.WorkDir = "/tmp/lark-claude-bot"
	}

	return &BotHandler{
		claudeClient: config.ClaudeClient,
		sessionMgr:   config.SessionManager,
		workDir:      config.WorkDir,
	}, nil
}

// HandleMessage processes an incoming Lark message event
func (h *BotHandler) HandleMessage(ctx context.Context, event *larkevent.EventReq) (string, error) {
	// Parse message event
	msgEvent, err := h.parseMessageEvent(event)
	if err != nil {
		return "", fmt.Errorf("failed to parse message event: %w", err)
	}

	// Skip empty messages
	if msgEvent.Content == "" {
		return "", nil
	}

	// Get or create session
	session, err := h.sessionMgr.Get(msgEvent.ChatID)
	var sessionID string
	if err == nil && session != nil {
		sessionID = session.SessionID
	}

	// Process message with Claude
	response, err := h.claudeClient.ProcessMessage(ctx, msgEvent.Content, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to process message with claude: %w", err)
	}

	// Update session
	if _, err := h.sessionMgr.Set(msgEvent.ChatID, response.SessionID); err != nil {
		// Log error but don't fail the response
		fmt.Printf("Warning: failed to save session: %v\n", err)
	}

	return response.Result, nil
}

// parseMessageEvent extracts message data from Lark event
func (h *BotHandler) parseMessageEvent(event *larkevent.EventReq) (*MessageEvent, error) {
	if event == nil || event.Body == nil {
		return nil, fmt.Errorf("nil event or event body")
	}

	// Parse event body as JSON
	var rawData map[string]interface{}
	if err := json.Unmarshal(event.Body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event body: %w", err)
	}

	// Extract event data
	eventData, ok := rawData["event"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("event data not found")
	}

	// Extract message fields
	msgEvent := &MessageEvent{}

	// Chat ID
	if v, ok := eventData["chat_id"].(string); ok {
		msgEvent.ChatID = v
	}

	// Message ID
	if v, ok := eventData["message_id"].(string); ok {
		msgEvent.MessageID = v
	}

	// Sender ID
	if sender, ok := eventData["sender"].(map[string]interface{}); ok {
		if v, ok := sender["sender_id"].(string); ok {
			msgEvent.SenderID = v
		}
		if v, ok := sender["sender_type"].(string); ok {
			// Could filter by sender_type if needed
			_ = v
		}
	}

	// Message type
	if v, ok := eventData["message_type"].(string); ok {
		msgEvent.MessageType = v
	}

	// Message content (needs parsing based on message_type)
	if v, ok := eventData["content"].(string); ok {
		msgEvent.Content = h.extractTextContent(v, msgEvent.MessageType)
	}

	// Create time
	if v, ok := eventData["create_time"].(string); ok {
		msgEvent.CreateTime = v
	}

	return msgEvent, nil
}

// extractTextContent extracts plain text from Lark message content
// Lark message content is JSON-encoded, format depends on message_type
func (h *BotHandler) extractTextContent(content string, messageType string) string {
	if content == "" {
		return ""
	}

	// Parse content JSON
	var contentData map[string]interface{}
	if err := json.Unmarshal([]byte(content), &contentData); err != nil {
		// If parsing fails, return raw content
		return content
	}

	switch messageType {
	case "text":
		// Text messages: {"text":"..."}
		if v, ok := contentData["text"].(string); ok {
			return v
		}
	case "post":
		// Post messages: {"post":{"content":[...]}}
		if post, ok := contentData["post"].(map[string]interface{}); ok {
			if content, ok := post["content"].([]interface{}); ok {
				// Extract text from post structure
				return h.extractPostText(content)
			}
		}
	}

	// Fallback: try to convert to string
	return fmt.Sprintf("%v", contentData)
}

// extractPostText recursively extracts text from post content structure
func (h *BotHandler) extractPostText(content []interface{}) string {
	var result string

	for _, item := range content {
		if segment, ok := item.(map[string]interface{}); ok {
			if text, ok := segment["text"].(string); ok {
				result += text + "\n"
			}
		}
	}

	return result
}

// GetStats returns handler statistics
func (h *BotHandler) GetStats(ctx context.Context) (map[string]interface{}, error) {
	sessions, err := h.sessionMgr.List()
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	stats := map[string]interface{}{
		"active_sessions": len(sessions),
		"work_dir":        h.workDir,
	}

	return stats, nil
}
