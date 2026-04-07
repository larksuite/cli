// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"time"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

// TransformMode defines the event transformation mode.
type TransformMode int

const (
	// TransformRaw passes through with minimal processing.
	TransformRaw TransformMode = iota
	// TransformCompact extracts core fields, suitable for AI agent consumption.
	TransformCompact
)

// WindowConfig configures event windowing strategy (not implemented yet).
// Zero value means disabled.
type WindowConfig struct {
	Duration time.Duration
	GroupBy  string
}

// RawEvent is the strongly-typed V2 event envelope.
// Parsed directly from event.Body JSON bytes.
type RawEvent struct {
	Schema string                `json:"schema"`
	Header larkevent.EventHeader `json:"header"`
	Event  json.RawMessage       `json:"event"`
}

// EventProcessor defines the processing strategy for each event type.
//
// Each processor implements its own Transform logic supporting Raw/Compact modes.
// The framework decides which mode to pass based on CLI flags; the processor
// decides the output format for that mode.
//
// Raw mode: return raw (the complete *RawEvent) to preserve the full original event.
// Compact mode: return a flat map[string]interface{} ready for JSON serialization,
// including semantic fields like "type", "id", "from", "to" plus domain-specific fields.
type EventProcessor interface {
	// EventType returns the event type handled, e.g. "im.message.receive_v1".
	// The fallback processor returns an empty string.
	EventType() string

	// Transform converts raw event data to the target format.
	// The returned value is serialized directly to JSON by the pipeline.
	Transform(ctx context.Context, raw *RawEvent, mode TransformMode) interface{}

	// DeduplicateKey returns a deduplication key. Empty string means no dedup.
	DeduplicateKey(raw *RawEvent) string

	// WindowStrategy returns window configuration. Zero value means disabled.
	WindowStrategy() WindowConfig
}

// transformViaHandler keeps the legacy EventProcessor surface aligned with
// the handler-based runtime by normalizing raw input and delegating compact
// rendering to the current EventHandler implementation.
func transformViaHandler(ctx context.Context, raw *RawEvent, mode TransformMode, handler EventHandler) interface{} {
	if mode == TransformRaw {
		return raw
	}
	evt, ok := legacyEventFromRaw(raw)
	if !ok || handler == nil {
		return raw
	}
	result := handler.Handle(ctx, evt)
	if result.Output == nil {
		return raw
	}
	return result.Output
}

func legacyEventFromRaw(raw *RawEvent) (*Event, bool) {
	if raw == nil {
		return nil, false
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	evt, err := NormalizeEnvelope(InboundEnvelope{
		Source:     SourceWebSocket,
		RawPayload: body,
	})
	if err != nil {
		return nil, false
	}
	return evt, true
}
