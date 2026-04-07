// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "time"

// Source identifies the adapter that delivered an inbound event.
type Source string

const (
	SourceWebSocket Source = "websocket"
	SourceWebhook   Source = "webhook"
)

// BuildWebSocketEnvelope adapts a raw WebSocket body into the inbound pipeline shape.
func BuildWebSocketEnvelope(body []byte) InboundEnvelope {
	return InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: time.Now(),
		RawPayload: append([]byte(nil), body...),
	}
}

// InboundEnvelope is the raw input shape for the routing layer.
type InboundEnvelope struct {
	Source     Source            `json:"source"`
	ReceivedAt time.Time         `json:"received_at"`
	Headers    map[string]string `json:"headers,omitempty"`
	RawPayload []byte            `json:"raw_payload"`
}

// EventHeader is the normalized event header shared across source adapters.
type EventHeader struct {
	EventID    string `json:"event_id,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	CreateTime string `json:"create_time,omitempty"`
	TenantKey  string `json:"tenant_key,omitempty"`
	AppID      string `json:"app_id,omitempty"`
}

// NormalizedPayload stores events in header + data form.
type NormalizedPayload struct {
	Header EventHeader            `json:"header"`
	Data   map[string]interface{} `json:"data"`
}

// Event is the normalized routing-layer event model.
type Event struct {
	Source         Source                 `json:"source"`
	EventID        string                 `json:"event_id,omitempty"`
	EventType      string                 `json:"event_type"`
	Domain         string                 `json:"domain,omitempty"`
	Payload        NormalizedPayload      `json:"payload"`
	RawPayload     []byte                 `json:"raw_payload"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

// MalformedEventError reports why an inbound payload could not be normalized.
type MalformedEventError struct {
	Reason string
	Err    error
}

func (e *MalformedEventError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Reason
	}
	return e.Reason + ": " + e.Err.Error()
}

func (e *MalformedEventError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
