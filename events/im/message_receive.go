// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/internal/event"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

// ImMessageReceiveOutput is the flattened shape for im.message.receive_v1; `desc` tags drive the reflected schema.
type ImMessageReceiveOutput struct {
	Type        string `json:"type"                   desc:"Event type; always im.message.receive_v1"`
	EventID     string `json:"event_id,omitempty"     desc:"Globally unique event ID; safe for deduplication"`
	Timestamp   string `json:"timestamp,omitempty"    desc:"Event delivery time (ms timestamp string); prefers header.create_time"                                                                                                              kind:"timestamp_ms"`
	ID          string `json:"id,omitempty"           desc:"Message ID (legacy alias of message_id, kept for compatibility)"                                                                                                                     kind:"message_id"`
	MessageID   string `json:"message_id,omitempty"   desc:"Message ID; prefixed with om_"                                                                                                                                                       kind:"message_id"`
	CreateTime  string `json:"create_time,omitempty"  desc:"Message creation time (ms timestamp string)"                                                                                                                                         kind:"timestamp_ms"`
	ChatID      string `json:"chat_id,omitempty"      desc:"Chat/conversation ID; prefixed with oc_"                                                                                                                                             kind:"chat_id"`
	ChatType    string `json:"chat_type,omitempty"    desc:"Conversation type"                                                                                                                                                                   enum:"p2p,group"`
	MessageType string `json:"message_type,omitempty" desc:"Message type"`
	SenderID    string `json:"sender_id,omitempty"    desc:"Sender open_id; prefixed with ou_"                                                                                                                                                   kind:"open_id"`
	Content     string `json:"content,omitempty"      desc:"Message content. For most types (text/post/image/file/audio, etc.) this is pre-rendered human-readable text. For interactive (cards) it stays as the raw JSON string and callers must fromjson to parse it."`
}

func processImMessageReceive(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
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
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	msg := envelope.Event.Message
	content := msg.Content
	if msg.MessageType != "interactive" {
		content = convertlib.ConvertBodyContent(msg.MessageType, &convertlib.ConvertContext{
			RawContent: msg.Content,
			MentionMap: convertlib.BuildMentionKeyMap(msg.Mentions),
		})
	}

	timestamp := envelope.Header.CreateTime
	if timestamp == "" {
		timestamp = msg.CreateTime
	}

	out := &ImMessageReceiveOutput{
		Type:        envelope.Header.EventType,
		EventID:     envelope.Header.EventID,
		Timestamp:   timestamp,
		ID:          msg.MessageID,
		MessageID:   msg.MessageID,
		CreateTime:  msg.CreateTime,
		ChatID:      msg.ChatID,
		ChatType:    msg.ChatType,
		MessageType: msg.MessageType,
		SenderID:    envelope.Event.Sender.SenderID.OpenID,
		Content:     content,
	}
	return json.Marshal(out)
}
