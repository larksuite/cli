// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"encoding/json"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// MessageSender handles sending messages back to Lark.
type MessageSender struct {
	larkClient *lark.Client
}

// NewMessageSender creates a new message sender (placeholder for testing).
func NewMessageSender() *MessageSender {
	return &MessageSender{}
}

// NewMessageSenderWithClient creates a new message sender backed by a real Lark client.
func NewMessageSenderWithClient(larkClient *lark.Client) *MessageSender {
	return &MessageSender{larkClient: larkClient}
}

// SendMessage sends a text message to a Lark chat.
// If parentMessageID is non-empty, the message is sent as a reply in the thread.
func (s *MessageSender) SendMessage(ctx context.Context, chatID, message, parentMessageID string) error {
	if s.larkClient == nil {
		return fmt.Errorf("lark client not initialized")
	}
	if chatID == "" {
		return fmt.Errorf("chat_id is required")
	}

	content, err := s.buildMessageContent(message)
	if err != nil {
		return fmt.Errorf("failed to build message content: %w", err)
	}

	// Use reply endpoint if parent message ID is provided
	if parentMessageID != "" {
		return s.sendReply(ctx, parentMessageID, "text", content)
	}

	// Build create message request
	body := larkim.NewCreateMessageReqBodyBuilder().
		ReceiveId(chatID).
		MsgType("text").
		Content(content).
		Build()

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(body).
		Build()

	resp, err := s.larkClient.Im.V1.Message.Create(ctx, req,
		larkcore.WithTenantAccessToken(""),
	)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send message API error: code=%d msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// sendReply sends a message as a reply to an existing message.
func (s *MessageSender) sendReply(ctx context.Context, parentMessageID, msgType, content string) error {
	body := larkim.NewReplyMessageReqBodyBuilder().
		Content(content).
		MsgType(msgType).
		Build()

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(parentMessageID).
		Body(body).
		Build()

	resp, err := s.larkClient.Im.V1.Message.Reply(ctx, req,
		larkcore.WithTenantAccessToken(""),
	)
	if err != nil {
		return fmt.Errorf("failed to send reply: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send reply API error: code=%d msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// buildMessageContent builds the message content JSON for Lark text messages.
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

// CreateMessageRequest creates a Lark message send request.
// Kept for reference; actual sending uses SendMessage.
func (s *MessageSender) CreateMessageRequest(chatID, content, parentMessageID string) *larkim.CreateMessageReq {
	body := larkim.NewCreateMessageReqBodyBuilder().
		ReceiveId(chatID).
		MsgType("text").
		Content(content).
		Build()

	return larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(body).
		Build()
}
