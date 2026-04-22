// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/internal/event"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

// ImMessageReceiveOutput is the flattened shape emitted for im.message.receive_v1.
// The `desc` tags drive the reflection-based schema shown by `event schema`.
//
// Content semantics (see processImMessageReceive):
//   - text / post / image / file / audio: human-readable text from convertlib
//     with @mentions resolved to display names.
//   - interactive (card): raw `content` JSON string — card payloads carry
//     structured actions that a flat rendering would lose.
type ImMessageReceiveOutput struct {
	Type        string `json:"type"                   desc:"事件类型，恒定为 im.message.receive_v1"`
	EventID     string `json:"event_id,omitempty"     desc:"飞书事件唯一 ID，可用于去重"`
	Timestamp   string `json:"timestamp,omitempty"    desc:"事件投递时间（毫秒时间戳字符串），优先取自 header.create_time"                  kind:"timestamp_ms"`
	ID          string `json:"id,omitempty"           desc:"消息 ID（等同于 message_id，历史别名，保留兼容）"                                kind:"message_id"`
	MessageID   string `json:"message_id,omitempty"   desc:"消息 ID，以 om_ 开头"                                                          kind:"message_id"`
	CreateTime  string `json:"create_time,omitempty"  desc:"消息发送时间（毫秒时间戳字符串）"                                                  kind:"timestamp_ms"`
	ChatID      string `json:"chat_id,omitempty"      desc:"群组/会话 ID，以 oc_ 开头"                                                      kind:"chat_id"`
	ChatType    string `json:"chat_type,omitempty"    desc:"会话类型"                                                                     enum:"p2p,group"`
	MessageType string `json:"message_type,omitempty" desc:"消息类型；interactive 保留原始 JSON，其余已渲染为可读文本"                         enum:"text,post,image,file,audio,media,sticker,interactive,share_chat,share_user,system"`
	SenderID    string `json:"sender_id,omitempty"    desc:"发送者 open_id，以 ou_ 开头"                                                    kind:"open_id"`
	Content     string `json:"content,omitempty"      desc:"消息内容；interactive（卡片）保持原始 JSON 字符串，调用方需 fromjson 自行解析"`
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
		// Garbage in → original bytes back out so consumers still see the event.
		return raw.Payload, nil
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
