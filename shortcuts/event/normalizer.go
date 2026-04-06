// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type rawEnvelope struct {
	Schema string                 `json:"schema"`
	Header EventHeader            `json:"header"`
	Event  map[string]interface{} `json:"event"`
}

// NormalizeEnvelope converts a source-specific inbound envelope into the routing-layer event model.
func NormalizeEnvelope(env InboundEnvelope) (*Event, error) {
	var raw rawEnvelope
	if err := json.Unmarshal(env.RawPayload, &raw); err != nil {
		return nil, &MalformedEventError{Reason: "invalid_json", Err: err}
	}
	if raw.Schema != "2.0" {
		return nil, &MalformedEventError{Reason: "unsupported_schema"}
	}
	if raw.Header.EventType == "" {
		return nil, &MalformedEventError{Reason: "missing_event_type"}
	}
	if raw.Event == nil {
		raw.Event = map[string]interface{}{}
	}

	payload := NormalizedPayload{
		Header: raw.Header,
		Data:   raw.Event,
	}

	rawPayload := append([]byte(nil), env.RawPayload...)
	event := &Event{
		Source:     env.Source,
		EventID:    raw.Header.EventID,
		EventType:  raw.Header.EventType,
		Domain:     ResolveDomain(&Event{EventType: raw.Header.EventType}),
		Payload:    payload,
		RawPayload: rawPayload,
		Metadata: map[string]interface{}{
			"received_at": env.ReceivedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		},
	}
	event.IdempotencyKey = buildIdempotencyKey(env.Source, event.EventID, rawPayload)

	return event, nil
}

func buildIdempotencyKey(source Source, eventID string, rawPayload []byte) string {
	if eventID != "" {
		return string(source) + ":" + eventID
	}

	sum := sha256.Sum256(rawPayload)
	return string(source) + ":sha256:" + hex.EncodeToString(sum[:])
}
