package bot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/oapi-sdk-go/v3/service/im"
)

// MessageSender handles sending messages back to Lark
type MessageSender struct {
	// TODO: Add Lark client when integrating with im +messages-send
}

// NewMessageSender creates a new message sender
func NewMessageSender() *MessageSender {
	return &MessageSender{}
}

// SendMessage sends a text message to a Lark chat
// TODO: Integrate with shortcuts/im/im_messages_send.go
func (s *MessageSender) SendMessage(ctx context.Context, chatID, message, parentMessageID string) error {
	// Placeholder implementation
	// In production, this would call:
	// lark-cli im +messages-send --chat-id <chatID> --text <message> --parent-msg-id <parentMessageID>

	fmt.Printf("[TODO] Send message to chat %s: %s\n", chatID, message)

	return nil
}

// buildMessageContent builds the message content JSON for Lark
func (s *MessageSender) buildMessageContent(text string) (string, error) {
	content := map[string]string{
		"text": text,
	}

	data, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message content: %w", err)
	}

	return string(data), nil
}

// CreateMessageRequest creates a Lark message send request
func (s *MessageSender) CreateMessageRequest(chatID, content, parentMessageID string) *im.CreateMessageReq {
	req := &im.CreateMessageReq{}
	req.MsgType = "text"
	req.ReceiveIdType = "chat_id"
	req.ReceiveId = chatID
	req.Content = content

	if parentMessageID != "" {
		req.ReplyInThread = true
		req.ParentId = parentMessageID
	}

	return req
}
