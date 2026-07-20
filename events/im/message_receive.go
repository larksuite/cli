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
	Type        string          `json:"type"                   desc:"Event type; always im.message.receive_v1"`
	EventID     string          `json:"event_id,omitempty"     desc:"Event delivery ID. Do not use as the message deduplication key; use message_id instead."`
	Timestamp   string          `json:"timestamp,omitempty"    desc:"Event delivery time (ms timestamp string); prefers header.create_time"                                                                                                              kind:"timestamp_ms"`
	ID          string          `json:"id,omitempty"           desc:"Message ID (legacy alias of message_id, kept for compatibility)"                                                                                                                     kind:"message_id"`
	MessageID   string          `json:"message_id,omitempty"   desc:"Message ID; prefixed with om_. Recommended idempotency key for im.message.receive_v1 consumers."                                                                                     kind:"message_id"`
	CreateTime  string          `json:"create_time,omitempty"  desc:"Message creation time (ms timestamp string)"                                                                                                                                         kind:"timestamp_ms"`
	UpdateTime  string          `json:"update_time,omitempty"  desc:"Message update time (ms timestamp string); emitted only when different from create_time"                                                                                              kind:"timestamp_ms"`
	ChatID      string          `json:"chat_id,omitempty"      desc:"Chat/conversation ID; prefixed with oc_"                                                                                                                                             kind:"chat_id"`
	ChatType    string          `json:"chat_type,omitempty"    desc:"Conversation type"                                                                                                                                                                   enum:"p2p,group"`
	MessageType string          `json:"message_type,omitempty" desc:"Message type"`
	SenderID    string          `json:"sender_id,omitempty"    desc:"Sender open_id; prefixed with ou_"                                                                                                                                                   kind:"open_id"`
	SenderType  string          `json:"sender_type,omitempty"  desc:"Sender type"                                                                                                                                                                         enum:"user,bot"`
	RootID      string          `json:"root_id,omitempty"      desc:"Root message ID of the reply/thread context, when present"                                                                                                                           kind:"message_id"`
	ThreadID    string          `json:"thread_id,omitempty"    desc:"Thread ID, when present"`
	ReplyTo     string          `json:"reply_to,omitempty"     desc:"Parent message ID of the direct reply context, when present"                                                                                                                         kind:"message_id"`
	Content     string          `json:"content,omitempty"      desc:"Message content. For most types (text/post/image/file/audio, etc.) this is pre-rendered human-readable text."`
	Mentions    []MentionOutput `json:"mentions,omitempty" desc:"Compact mentions aligned with im +messages-mget"`
}

type MentionOutput struct {
	Key  string `json:"key,omitempty"  desc:"Mention placeholder key, for example @_user_1"`
	ID   string `json:"id,omitempty"   desc:"Mentioned user open_id; prefixed with ou_"        kind:"open_id"`
	Name string `json:"name,omitempty" desc:"Mentioned display name"`
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
				RootID      string        `json:"root_id"`
				ParentID    string        `json:"parent_id"`
				ThreadID    string        `json:"thread_id"`
				ChatID      string        `json:"chat_id"`
				ChatType    string        `json:"chat_type"`
				MessageType string        `json:"message_type"`
				Content     string        `json:"content"`
				CreateTime  string        `json:"create_time"`
				UpdateTime  string        `json:"update_time"`
				Mentions    []interface{} `json:"mentions"`
			} `json:"message"`
			Sender struct {
				SenderType string `json:"sender_type"`
				SenderID   struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
			} `json:"sender"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	msg := envelope.Event.Message
	var content string
	if msg.MessageType == "interactive" {
		content = convertlib.ConvertInteractiveEventContent(msg.Content, msg.Mentions)
	} else {
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
		SenderType:  envelope.Event.Sender.SenderType,
		RootID:      msg.RootID,
		ThreadID:    msg.ThreadID,
		ReplyTo:     msg.ParentID,
		Content:     content,
		Mentions:    compactMentions(msg.Mentions),
	}
	if msg.UpdateTime != "" && msg.UpdateTime != msg.CreateTime {
		out.UpdateTime = msg.UpdateTime
	}
	return json.Marshal(out)
}

func compactMentions(mentions []interface{}) []MentionOutput {
	if len(mentions) == 0 {
		return nil
	}
	out := make([]MentionOutput, 0, len(mentions))
	for _, raw := range mentions {
		item, _ := raw.(map[string]interface{})
		mention := MentionOutput{
			Key:  stringField(item, "key"),
			ID:   mentionOpenID(item["id"]),
			Name: stringField(item, "name"),
		}
		if mention.Key != "" || mention.ID != "" || mention.Name != "" {
			out = append(out, mention)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func mentionOpenID(raw interface{}) string {
	switch v := raw.(type) {
	case map[string]interface{}:
		openID, _ := v["open_id"].(string)
		return openID
	case string:
		return v
	default:
		return ""
	}
}
