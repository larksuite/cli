// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
)

const genericHandlerID = "builtin.generic.fallback"

// GenericProcessor is the fallback for unregistered event types.
// Compact mode parses the event payload as a map; Raw mode passes through raw.Event.
type GenericProcessor struct{}

func NewGenericFallbackHandler() *GenericProcessor { return &GenericProcessor{} }

func (p *GenericProcessor) ID() string        { return genericHandlerID }
func (p *GenericProcessor) EventType() string { return "" }
func (p *GenericProcessor) Domain() string    { return "" }

func (p *GenericProcessor) Handle(_ context.Context, evt *Event) HandlerResult {
	return HandlerResult{
		Status: HandlerStatusHandled,
		Output: genericCompactOutput(evt),
	}
}

func (p *GenericProcessor) Transform(_ context.Context, raw *RawEvent, mode TransformMode) interface{} {
	if mode == TransformRaw {
		return raw
	}
	return genericCompactMap(raw)
}

func (p *GenericProcessor) DeduplicateKey(raw *RawEvent) string { return raw.Header.EventID }
func (p *GenericProcessor) WindowStrategy() WindowConfig        { return WindowConfig{} }

func genericCompactMap(raw *RawEvent) interface{} {
	// Compact: parse event as flat map, inject envelope metadata so AI
	// can always identify the event type regardless of which processor ran.
	var eventMap map[string]interface{}
	if err := json.Unmarshal(raw.Event, &eventMap); err != nil {
		return raw
	}
	eventMap["type"] = raw.Header.EventType
	if raw.Header.EventID != "" {
		eventMap["event_id"] = raw.Header.EventID
	}
	if raw.Header.CreateTime != "" {
		eventMap["timestamp"] = raw.Header.CreateTime
	}
	return eventMap
}

func genericCompactOutput(evt *Event) interface{} {
	if evt == nil {
		return map[string]interface{}{"type": ""}
	}

	out := map[string]interface{}{}
	for k, v := range evt.Payload.Data {
		out[k] = v
	}
	if len(out) == 0 && len(evt.RawPayload) > 0 {
		var rawMap map[string]interface{}
		if err := json.Unmarshal(evt.RawPayload, &rawMap); err == nil {
			for k, v := range rawMap {
				out[k] = v
			}
		}
	}
	out["type"] = evt.EventType
	if evt.EventID != "" {
		out["event_id"] = evt.EventID
	}
	if evt.Payload.Header.CreateTime != "" {
		out["timestamp"] = evt.Payload.Header.CreateTime
	}
	if len(evt.RawPayload) > 0 {
		if _, ok := out["raw_payload"]; !ok {
			out["raw_payload"] = string(evt.RawPayload)
		}
	}
	return out
}
