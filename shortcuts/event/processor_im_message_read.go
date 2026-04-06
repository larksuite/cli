// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
)

const imMessageReadHandlerID = "builtin.im.message.read"

// ── im.message.message_read_v1 ───────────────────────────────────────────────

// ImMessageReadProcessor handles im.message.message_read_v1 events.
//
// Compact output fields:
//   - type, event_id, timestamp (from compactBase)
//   - reader_id: the open_id of the user who read the message
//   - read_time: Unix timestamp of the read action
//   - message_ids: list of message IDs that were read
type ImMessageReadProcessor struct{}

func NewIMMessageReadHandler() *ImMessageReadProcessor { return &ImMessageReadProcessor{} }

func (p *ImMessageReadProcessor) ID() string        { return imMessageReadHandlerID }
func (p *ImMessageReadProcessor) EventType() string { return "im.message.message_read_v1" }
func (p *ImMessageReadProcessor) Domain() string    { return "im" }

func (p *ImMessageReadProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled, Output: imMessageReadCompactOutput(evt)}
}

func (p *ImMessageReadProcessor) Transform(_ context.Context, raw *RawEvent, mode TransformMode) interface{} {
	if mode == TransformRaw {
		return raw
	}
	var ev imMessageReadPayload
	if err := json.Unmarshal(raw.Event, &ev); err != nil {
		return raw
	}
	return buildIMMessageReadCompactOutput(raw.Header.EventType, raw.Header.EventID, raw.Header.CreateTime, ev)
}

func (p *ImMessageReadProcessor) DeduplicateKey(raw *RawEvent) string {
	return raw.Header.EventID
}
func (p *ImMessageReadProcessor) WindowStrategy() WindowConfig { return WindowConfig{} }

type imMessageReadPayload struct {
	Reader struct {
		ReaderID struct {
			OpenID string `json:"open_id"`
		} `json:"reader_id"`
		ReadTime string `json:"read_time"`
	} `json:"reader"`
	MessageIDList []string `json:"message_id_list"`
}

func imMessageReadCompactOutput(evt *Event) map[string]interface{} {
	if evt == nil {
		return map[string]interface{}{"type": ""}
	}
	data, _ := json.Marshal(evt.Payload.Data)
	var payload imMessageReadPayload
	_ = json.Unmarshal(data, &payload)
	return buildIMMessageReadCompactOutput(evt.EventType, evt.EventID, evt.Payload.Header.CreateTime, payload)
}

func buildIMMessageReadCompactOutput(eventType, eventID, createTime string, ev imMessageReadPayload) map[string]interface{} {
	out := map[string]interface{}{"type": eventType}
	if eventID != "" {
		out["event_id"] = eventID
	}
	if createTime != "" {
		out["timestamp"] = createTime
	}
	if ev.Reader.ReaderID.OpenID != "" {
		out["reader_id"] = ev.Reader.ReaderID.OpenID
	}
	if ev.Reader.ReadTime != "" {
		out["read_time"] = ev.Reader.ReadTime
	}
	if len(ev.MessageIDList) > 0 {
		out["message_ids"] = ev.MessageIDList
	}
	return out
}
