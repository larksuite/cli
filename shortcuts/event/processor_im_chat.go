// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
)

// ── im.chat.updated_v1 ──────────────────────────────────────────────────────

// ImChatUpdatedProcessor handles im.chat.updated_v1 events.
//
// Compact output fields:
//   - type, event_id, timestamp (from compactBase)
//   - chat_id: the group chat that was updated
//   - operator_id: open_id of the user who made the change
//   - external: whether this is an external (cross-tenant) chat
//   - before_change: chat properties before the update (e.g. name, description)
//   - after_change: chat properties after the update
type ImChatUpdatedProcessor struct{}

func NewIMChatUpdatedHandler() *ImChatUpdatedProcessor { return &ImChatUpdatedProcessor{} }

func (p *ImChatUpdatedProcessor) ID() string        { return "builtin.im.chat.updated" }
func (p *ImChatUpdatedProcessor) EventType() string { return "im.chat.updated_v1" }
func (p *ImChatUpdatedProcessor) Domain() string    { return "im" }

func (p *ImChatUpdatedProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled, Output: imChatUpdatedCompactOutput(evt)}
}

func (p *ImChatUpdatedProcessor) Transform(ctx context.Context, raw *RawEvent, mode TransformMode) interface{} {
	return transformViaHandler(ctx, raw, mode, p)
}

func (p *ImChatUpdatedProcessor) DeduplicateKey(raw *RawEvent) string {
	return raw.Header.EventID
}
func (p *ImChatUpdatedProcessor) WindowStrategy() WindowConfig { return WindowConfig{} }

// ── im.chat.disbanded_v1 ────────────────────────────────────────────────────

// ImChatDisbandedProcessor handles im.chat.disbanded_v1 events.
//
// Compact output fields:
//   - type, event_id, timestamp (from compactBase)
//   - chat_id: the group chat that was disbanded
//   - operator_id: open_id of the user who disbanded the chat
//   - external: whether this is an external (cross-tenant) chat
type ImChatDisbandedProcessor struct{}

func NewIMChatDisbandedHandler() *ImChatDisbandedProcessor { return &ImChatDisbandedProcessor{} }

func (p *ImChatDisbandedProcessor) ID() string        { return "builtin.im.chat.disbanded" }
func (p *ImChatDisbandedProcessor) EventType() string { return "im.chat.disbanded_v1" }
func (p *ImChatDisbandedProcessor) Domain() string    { return "im" }

func (p *ImChatDisbandedProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled, Output: imChatDisbandedCompactOutput(evt)}
}

func (p *ImChatDisbandedProcessor) Transform(ctx context.Context, raw *RawEvent, mode TransformMode) interface{} {
	return transformViaHandler(ctx, raw, mode, p)
}

func (p *ImChatDisbandedProcessor) DeduplicateKey(raw *RawEvent) string {
	return raw.Header.EventID
}
func (p *ImChatDisbandedProcessor) WindowStrategy() WindowConfig { return WindowConfig{} }

type imChatPayload struct {
	ChatID       string      `json:"chat_id"`
	OperatorID   interface{} `json:"operator_id"`
	External     bool        `json:"external"`
	AfterChange  interface{} `json:"after_change"`
	BeforeChange interface{} `json:"before_change"`
}

func imChatUpdatedCompactOutput(evt *Event) map[string]interface{} {
	return imChatCompactOutput(evt, true)
}

func imChatDisbandedCompactOutput(evt *Event) map[string]interface{} {
	return imChatCompactOutput(evt, false)
}

func imChatCompactOutput(evt *Event, includeChanges bool) map[string]interface{} {
	if evt == nil {
		return map[string]interface{}{"type": ""}
	}
	data, _ := json.Marshal(evt.Payload.Data)
	var payload imChatPayload
	_ = json.Unmarshal(data, &payload)
	return buildIMChatCompactOutput(evt.EventType, evt.EventID, evt.Payload.Header.CreateTime, payload, includeChanges)
}

func buildIMChatCompactOutput(eventType, eventID, createTime string, ev imChatPayload, includeChanges bool) map[string]interface{} {
	out := map[string]interface{}{"type": eventType}
	if eventID != "" {
		out["event_id"] = eventID
	}
	if createTime != "" {
		out["timestamp"] = createTime
	}
	if ev.ChatID != "" {
		out["chat_id"] = ev.ChatID
	}
	if id := openID(ev.OperatorID); id != "" {
		out["operator_id"] = id
	}
	out["external"] = ev.External
	if includeChanges {
		if ev.AfterChange != nil {
			out["after_change"] = ev.AfterChange
		}
		if ev.BeforeChange != nil {
			out["before_change"] = ev.BeforeChange
		}
	}
	return out
}
