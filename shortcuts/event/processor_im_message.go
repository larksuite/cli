// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"

	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

const imMessageHandlerID = "builtin.im.message.receive"

// ImMessageProcessor handles im.message.receive_v1 events.
//
// Compact output fields:
//   - type, id, message_id, create_time, timestamp
//   - chat_id, chat_type, message_type, sender_id
//   - content: human-readable text converted via convertlib (supports text, post, image, file, card, etc.)
type ImMessageProcessor struct{}

func NewIMMessageReceiveHandler() *ImMessageProcessor { return &ImMessageProcessor{} }

func (p *ImMessageProcessor) ID() string        { return imMessageHandlerID }
func (p *ImMessageProcessor) EventType() string { return "im.message.receive_v1" }
func (p *ImMessageProcessor) Domain() string    { return "im" }

func (p *ImMessageProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	out, ok := imMessageCompactOutput(evt)
	if !ok {
		return HandlerResult{
			Status: HandlerStatusHandled,
			Output: genericCompactOutput(evt),
			Reason: "interactive_fallback",
		}
	}
	return HandlerResult{Status: HandlerStatusHandled, Output: out}
}

func (p *ImMessageProcessor) Transform(ctx context.Context, raw *RawEvent, mode TransformMode) interface{} {
	return transformViaHandler(ctx, raw, mode, p)
}

func (p *ImMessageProcessor) DeduplicateKey(raw *RawEvent) string { return raw.Header.EventID }
func (p *ImMessageProcessor) WindowStrategy() WindowConfig        { return WindowConfig{} }

type imMessagePayload struct {
	Message struct {
		MessageID   string        `json:"message_id"`
		ChatID      string        `json:"chat_id"`
		ChatType    string        `json:"chat_type"`
		MessageType string        `json:"message_type"`
		Content     string        `json:"content"`
		CreateTime  string        `json:"create_time"`
		Mentions    []interface{} `json:"mentions"`
	} `json:"message"`
	Sender struct {
		SenderID struct {
			OpenID string `json:"open_id"`
		} `json:"sender_id"`
	} `json:"sender"`
}

func imMessageCompactOutput(evt *Event) (map[string]interface{}, bool) {
	if evt == nil {
		return nil, false
	}
	data, err := json.Marshal(evt.Payload.Data)
	if err != nil {
		return nil, false
	}
	var payload imMessagePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, false
	}
	return buildIMMessageCompactOutput(evt.EventType, evt.Payload.Header.CreateTime, payload)
}

func buildIMMessageCompactOutput(eventType, headerCreateTime string, ev imMessagePayload) (map[string]interface{}, bool) {
	// Card messages (interactive) are not yet supported for compact conversion;
	// return raw event data directly.
	if ev.Message.MessageType == "interactive" {
		return nil, false
	}

	// Use convertlib to convert raw content JSON into human-readable text.
	// Resolves @mention keys (e.g. @_user_1) to display names.
	content := convertlib.ConvertBodyContent(ev.Message.MessageType, &convertlib.ConvertContext{
		RawContent: ev.Message.Content,
		MentionMap: convertlib.BuildMentionKeyMap(ev.Message.Mentions),
	})

	out := map[string]interface{}{
		"type": eventType,
	}
	if ev.Message.MessageID != "" {
		out["id"] = ev.Message.MessageID
		out["message_id"] = ev.Message.MessageID
	}
	if ev.Message.CreateTime != "" {
		out["create_time"] = ev.Message.CreateTime
	}
	if headerCreateTime != "" {
		out["timestamp"] = headerCreateTime
	} else if ev.Message.CreateTime != "" {
		out["timestamp"] = ev.Message.CreateTime
	}
	if ev.Message.ChatID != "" {
		out["chat_id"] = ev.Message.ChatID
	}
	if ev.Message.ChatType != "" {
		out["chat_type"] = ev.Message.ChatType
	}
	if ev.Message.MessageType != "" {
		out["message_type"] = ev.Message.MessageType
	}
	if ev.Sender.SenderID.OpenID != "" {
		out["sender_id"] = ev.Sender.SenderID.OpenID
	}
	if content != "" {
		out["content"] = content
	}
	return out, true
}
