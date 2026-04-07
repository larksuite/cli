// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"strings"
)

// ── im.chat.member.bot.added_v1 / deleted_v1 ────────────────────────────────

// ImChatBotProcessor handles im.chat.member.bot.added_v1 and deleted_v1.
// A single struct serves both event types; the concrete type is set via constructor.
//
// Compact output fields:
//   - type, event_id, timestamp (from compactBase)
//   - action: "added" or "removed"
//   - chat_id: the group chat where the bot was added/removed
//   - operator_id: open_id of the user who performed the action
//   - external: whether this is an external (cross-tenant) chat
type ImChatBotProcessor struct {
	eventType string
}

// NewImChatBotAddedProcessor creates a processor for im.chat.member.bot.added_v1.
func NewImChatBotAddedProcessor() *ImChatBotProcessor {
	return &ImChatBotProcessor{eventType: "im.chat.member.bot.added_v1"}
}

func NewIMChatMemberBotAddedHandler() *ImChatBotProcessor { return NewImChatBotAddedProcessor() }

// NewImChatBotDeletedProcessor creates a processor for im.chat.member.bot.deleted_v1.
func NewImChatBotDeletedProcessor() *ImChatBotProcessor {
	return &ImChatBotProcessor{eventType: "im.chat.member.bot.deleted_v1"}
}

func NewIMChatMemberBotDeletedHandler() *ImChatBotProcessor { return NewImChatBotDeletedProcessor() }

func (p *ImChatBotProcessor) ID() string {
	if strings.Contains(p.eventType, "deleted") {
		return "builtin.im.chat.member.bot.deleted"
	}
	return "builtin.im.chat.member.bot.added"
}
func (p *ImChatBotProcessor) EventType() string { return p.eventType }
func (p *ImChatBotProcessor) Domain() string    { return "im" }

func (p *ImChatBotProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled, Output: imChatBotCompactOutput(evt, p.eventType)}
}

func (p *ImChatBotProcessor) Transform(ctx context.Context, raw *RawEvent, mode TransformMode) interface{} {
	return transformViaHandler(ctx, raw, mode, p)
}

func (p *ImChatBotProcessor) DeduplicateKey(raw *RawEvent) string { return raw.Header.EventID }
func (p *ImChatBotProcessor) WindowStrategy() WindowConfig        { return WindowConfig{} }

// ── im.chat.member.user.added_v1 / withdrawn_v1 / deleted_v1 ────────────────

// ImChatMemberUserProcessor handles im.chat.member.user.{added,withdrawn,deleted}_v1.
// A single struct serves all three event types; the concrete type is set via constructor.
//
// Compact output fields:
//   - type, event_id, timestamp (from compactBase)
//   - action: "added", "withdrawn" (user left), or "removed" (kicked by admin)
//   - chat_id: the group chat affected
//   - operator_id: open_id of the user who performed the action
//   - user_ids: list of open_ids of the affected users
//   - external: whether this is an external (cross-tenant) chat
type ImChatMemberUserProcessor struct {
	eventType string
}

// NewImChatMemberUserAddedProcessor creates a processor for im.chat.member.user.added_v1.
func NewImChatMemberUserAddedProcessor() *ImChatMemberUserProcessor {
	return &ImChatMemberUserProcessor{eventType: "im.chat.member.user.added_v1"}
}

func NewIMChatMemberUserAddedHandler() *ImChatMemberUserProcessor {
	return NewImChatMemberUserAddedProcessor()
}

// NewImChatMemberUserWithdrawnProcessor creates a processor for im.chat.member.user.withdrawn_v1.
func NewImChatMemberUserWithdrawnProcessor() *ImChatMemberUserProcessor {
	return &ImChatMemberUserProcessor{eventType: "im.chat.member.user.withdrawn_v1"}
}

func NewIMChatMemberUserWithdrawnHandler() *ImChatMemberUserProcessor {
	return NewImChatMemberUserWithdrawnProcessor()
}

// NewImChatMemberUserDeletedProcessor creates a processor for im.chat.member.user.deleted_v1.
func NewImChatMemberUserDeletedProcessor() *ImChatMemberUserProcessor {
	return &ImChatMemberUserProcessor{eventType: "im.chat.member.user.deleted_v1"}
}

func NewIMChatMemberUserDeletedHandler() *ImChatMemberUserProcessor {
	return NewImChatMemberUserDeletedProcessor()
}

func (p *ImChatMemberUserProcessor) ID() string {
	switch {
	case strings.Contains(p.eventType, "withdrawn"):
		return "builtin.im.chat.member.user.withdrawn"
	case strings.Contains(p.eventType, "deleted"):
		return "builtin.im.chat.member.user.deleted"
	default:
		return "builtin.im.chat.member.user.added"
	}
}
func (p *ImChatMemberUserProcessor) EventType() string { return p.eventType }
func (p *ImChatMemberUserProcessor) Domain() string    { return "im" }

func (p *ImChatMemberUserProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled, Output: imChatMemberUserCompactOutput(evt, p.eventType)}
}

func (p *ImChatMemberUserProcessor) Transform(ctx context.Context, raw *RawEvent, mode TransformMode) interface{} {
	return transformViaHandler(ctx, raw, mode, p)
}

func (p *ImChatMemberUserProcessor) DeduplicateKey(raw *RawEvent) string {
	return raw.Header.EventID
}
func (p *ImChatMemberUserProcessor) WindowStrategy() WindowConfig {
	return WindowConfig{}
}

type imChatMemberPayload struct {
	ChatID     string        `json:"chat_id"`
	OperatorID interface{}   `json:"operator_id"`
	External   bool          `json:"external"`
	Users      []interface{} `json:"users"`
}

func imChatBotCompactOutput(evt *Event, processorEventType string) map[string]interface{} {
	if evt == nil {
		return map[string]interface{}{"type": processorEventType}
	}
	data, _ := json.Marshal(evt.Payload.Data)
	var payload imChatMemberPayload
	_ = json.Unmarshal(data, &payload)
	return buildIMChatBotCompactOutput(evt.EventType, evt.EventID, evt.Payload.Header.CreateTime, processorEventType, payload)
}

func buildIMChatBotCompactOutput(eventType, eventID, createTime, processorEventType string, ev imChatMemberPayload) map[string]interface{} {
	out := map[string]interface{}{"type": eventType}
	if eventID != "" {
		out["event_id"] = eventID
	}
	if createTime != "" {
		out["timestamp"] = createTime
	}
	action := "added"
	if strings.Contains(processorEventType, "deleted") {
		action = "removed"
	}
	out["action"] = action
	if ev.ChatID != "" {
		out["chat_id"] = ev.ChatID
	}
	if id := openID(ev.OperatorID); id != "" {
		out["operator_id"] = id
	}
	out["external"] = ev.External
	return out
}

func imChatMemberUserCompactOutput(evt *Event, processorEventType string) map[string]interface{} {
	if evt == nil {
		return map[string]interface{}{"type": processorEventType}
	}
	data, _ := json.Marshal(evt.Payload.Data)
	var payload imChatMemberPayload
	_ = json.Unmarshal(data, &payload)
	return buildIMChatMemberUserCompactOutput(evt.EventType, evt.EventID, evt.Payload.Header.CreateTime, processorEventType, payload)
}

func buildIMChatMemberUserCompactOutput(eventType, eventID, createTime, processorEventType string, ev imChatMemberPayload) map[string]interface{} {
	out := map[string]interface{}{"type": eventType}
	if eventID != "" {
		out["event_id"] = eventID
	}
	if createTime != "" {
		out["timestamp"] = createTime
	}
	switch {
	case strings.Contains(processorEventType, "added"):
		out["action"] = "added"
	case strings.Contains(processorEventType, "withdrawn"):
		out["action"] = "withdrawn"
	case strings.Contains(processorEventType, "deleted"):
		out["action"] = "removed"
	}
	if ev.ChatID != "" {
		out["chat_id"] = ev.ChatID
	}
	if id := openID(ev.OperatorID); id != "" {
		out["operator_id"] = id
	}
	if userIDs := extractUserIDs(ev.Users); len(userIDs) > 0 {
		out["user_ids"] = userIDs
	}
	out["external"] = ev.External
	return out
}
