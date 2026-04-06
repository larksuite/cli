// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNormalizeEnvelopePreservesRawPayloadAndExtractsFields(t *testing.T) {
	receivedAt := time.Unix(1712345678, 0).UTC()
	rawPayload := []byte(`{"schema":"2.0","header":{"event_id":"evt_123","event_type":"im.message.receive_v1","create_time":"1712345600","tenant_key":"tenant_1","app_id":"cli_app"},"event":{"message":{"message_id":"om_123"},"sender":{"sender_id":{"open_id":"ou_1"}}}}`)
	env := InboundEnvelope{
		Source:     SourceWebhook,
		ReceivedAt: receivedAt,
		Headers: map[string]string{
			"X-Lark-Request-Timestamp": "1712345678",
		},
		RawPayload: rawPayload,
	}

	event, err := NormalizeEnvelope(env)
	if err != nil {
		t.Fatalf("NormalizeEnvelope returned error: %v", err)
	}

	if event.Source != SourceWebhook {
		t.Fatalf("Source = %q, want %q", event.Source, SourceWebhook)
	}
	if event.EventID != "evt_123" {
		t.Fatalf("EventID = %q, want evt_123", event.EventID)
	}
	if event.EventType != "im.message.receive_v1" {
		t.Fatalf("EventType = %q, want im.message.receive_v1", event.EventType)
	}
	if event.Domain != "im" {
		t.Fatalf("Domain = %q, want im", event.Domain)
	}
	if event.IdempotencyKey != "webhook:evt_123" {
		t.Fatalf("IdempotencyKey = %q, want webhook:evt_123", event.IdempotencyKey)
	}
	if !bytes.Equal(event.RawPayload, rawPayload) {
		t.Fatal("RawPayload bytes differ from input")
	}
	if &event.RawPayload[0] == &rawPayload[0] {
		t.Fatal("RawPayload should be copied, but shares backing array with input")
	}

	if got := event.Metadata["received_at"]; got != receivedAt.Format(time.RFC3339Nano) {
		t.Fatalf("Metadata[received_at] = %v, want %q", got, receivedAt.Format(time.RFC3339Nano))
	}

	if event.Payload.Header.EventID != "evt_123" || event.Payload.Header.EventType != "im.message.receive_v1" {
		t.Fatalf("Payload.Header = %+v", event.Payload.Header)
	}
	if event.Payload.Header.CreateTime != "1712345600" {
		t.Fatalf("Payload.Header.CreateTime = %q, want 1712345600", event.Payload.Header.CreateTime)
	}
	if event.Payload.Header.TenantKey != "tenant_1" {
		t.Fatalf("Payload.Header.TenantKey = %q, want tenant_1", event.Payload.Header.TenantKey)
	}
	if event.Payload.Header.AppID != "cli_app" {
		t.Fatalf("Payload.Header.AppID = %q, want cli_app", event.Payload.Header.AppID)
	}

	message, ok := event.Payload.Data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("Payload.Data[message] has unexpected type %T", event.Payload.Data["message"])
	}
	if message["message_id"] != "om_123" {
		t.Fatalf("Payload.Data[message][message_id] = %v, want om_123", message["message_id"])
	}

	sender, ok := event.Payload.Data["sender"].(map[string]interface{})
	if !ok {
		t.Fatalf("Payload.Data[sender] has unexpected type %T", event.Payload.Data["sender"])
	}
	senderID, ok := sender["sender_id"].(map[string]interface{})
	if !ok {
		t.Fatalf("Payload.Data[sender][sender_id] has unexpected type %T", sender["sender_id"])
	}
	if senderID["open_id"] != "ou_1" {
		t.Fatalf("Payload.Data[sender][sender_id][open_id] = %v, want ou_1", senderID["open_id"])
	}
}

func TestNormalizeEnvelopeMissingEventTypeReturnsMalformedError(t *testing.T) {
	env := InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: time.Unix(1712345678, 0).UTC(),
		RawPayload: []byte(`{"schema":"2.0","header":{"event_id":"evt_123"},"event":{"message":{}}}`),
	}

	_, err := NormalizeEnvelope(env)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var malformed *MalformedEventError
	if !errors.As(err, &malformed) {
		t.Fatalf("expected MalformedEventError, got %T", err)
	}
	if malformed.Reason != "missing_event_type" {
		t.Fatalf("Reason = %q, want missing_event_type", malformed.Reason)
	}
}

func TestNormalizeEnvelopeMissingOrUnsupportedSchemaReturnsMalformedError(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "missing schema",
			payload: `{"header":{"event_type":"im.message.receive_v1"},"event":{}}`,
			want:    "unsupported_schema",
		},
		{
			name:    "unsupported schema",
			payload: `{"schema":"1.0","header":{"event_type":"im.message.receive_v1"},"event":{}}`,
			want:    "unsupported_schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeEnvelope(InboundEnvelope{
				Source:     SourceWebhook,
				ReceivedAt: time.Unix(1712345678, 0).UTC(),
				RawPayload: []byte(tt.payload),
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var malformed *MalformedEventError
			if !errors.As(err, &malformed) {
				t.Fatalf("expected MalformedEventError, got %T", err)
			}
			if malformed.Reason != tt.want {
				t.Fatalf("Reason = %q, want %q", malformed.Reason, tt.want)
			}
		})
	}
}

func TestNormalizeEnvelopeUsesRawPayloadFingerprintWhenEventIDMissing(t *testing.T) {
	firstPayload := []byte(`{"schema":"2.0","header":{"event_type":"contact.user.created_v3","tenant_key":"tenant_1"},"event":{"user":{"user_id":"ou_123"}}}`)
	secondPayload := []byte(`{"schema":"2.0","header":{"tenant_key":"tenant_1","event_type":"contact.user.created_v3"},"event":{"user":{"user_id":"ou_123"}}}`)

	first, err := NormalizeEnvelope(InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: time.Unix(1712345678, 0).UTC(),
		RawPayload: firstPayload,
	})
	if err != nil {
		t.Fatalf("first NormalizeEnvelope returned error: %v", err)
	}
	second, err := NormalizeEnvelope(InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: time.Unix(1712345678, 0).UTC(),
		RawPayload: secondPayload,
	})
	if err != nil {
		t.Fatalf("second NormalizeEnvelope returned error: %v", err)
	}

	if first.EventID != "" || second.EventID != "" {
		t.Fatalf("EventID values = %q, %q; want both empty", first.EventID, second.EventID)
	}
	if first.IdempotencyKey == "" || second.IdempotencyKey == "" {
		t.Fatal("IdempotencyKey should not be empty")
	}
	if first.IdempotencyKey == string(SourceWebSocket)+":" || second.IdempotencyKey == string(SourceWebSocket)+":" {
		t.Fatal("IdempotencyKey should not fall back to empty event_id format")
	}
	if first.IdempotencyKey == second.IdempotencyKey {
		t.Fatalf("IdempotencyKey should differ for distinct raw payloads, got %q", first.IdempotencyKey)
	}

	replay, err := NormalizeEnvelope(InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: time.Unix(1712345678, 0).UTC(),
		RawPayload: firstPayload,
	})
	if err != nil {
		t.Fatalf("replay NormalizeEnvelope returned error: %v", err)
	}
	if first.IdempotencyKey != replay.IdempotencyKey {
		t.Fatalf("IdempotencyKey should be deterministic, got %q and %q", first.IdempotencyKey, replay.IdempotencyKey)
	}
}

func TestNormalizeEnvelopeInvalidJSONReturnsMalformedError(t *testing.T) {
	_, err := NormalizeEnvelope(InboundEnvelope{
		Source:     SourceWebhook,
		ReceivedAt: time.Unix(1712345678, 0).UTC(),
		RawPayload: []byte(`{"schema":`),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var malformed *MalformedEventError
	if !errors.As(err, &malformed) {
		t.Fatalf("expected MalformedEventError, got %T", err)
	}
	if malformed.Reason != "invalid_json" {
		t.Fatalf("Reason = %q, want invalid_json", malformed.Reason)
	}
	if malformed.Err == nil {
		t.Fatal("Err should be populated")
	}
}

func TestNormalizeEnvelopeNilEventNormalizesToEmptyDataObject(t *testing.T) {
	event, err := NormalizeEnvelope(InboundEnvelope{
		Source:     SourceWebhook,
		ReceivedAt: time.Unix(1712345678, 0).UTC(),
		RawPayload: []byte(`{"schema":"2.0","header":{"event_id":"evt_nil","event_type":"im.message.receive_v1"},"event":null}`),
	})
	if err != nil {
		t.Fatalf("NormalizeEnvelope returned error: %v", err)
	}

	if event.Payload.Data == nil {
		t.Fatal("Payload.Data = nil, want empty map")
	}
	if len(event.Payload.Data) != 0 {
		t.Fatalf("len(Payload.Data) = %d, want 0", len(event.Payload.Data))
	}

	body, err := json.Marshal(event.Payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(body) != `{"header":{"event_id":"evt_nil","event_type":"im.message.receive_v1"},"data":{}}` {
		t.Fatalf("payload JSON = %s, want data to serialize as empty object", body)
	}
}

func TestNormalizedPayloadJSONShape(t *testing.T) {
	payload := NormalizedPayload{
		Header: EventHeader{
			EventID:    "evt_1",
			EventType:  "im.message.receive_v1",
			CreateTime: "1712345600",
			TenantKey:  "tenant_1",
			AppID:      "cli_app",
		},
		Data: map[string]interface{}{
			"message": map[string]interface{}{"message_id": "om_1"},
			"sender":  map[string]interface{}{"sender_id": map[string]interface{}{"open_id": "ou_1"}},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	const want = `{"header":{"event_id":"evt_1","event_type":"im.message.receive_v1","create_time":"1712345600","tenant_key":"tenant_1","app_id":"cli_app"},"data":{"message":{"message_id":"om_1"},"sender":{"sender_id":{"open_id":"ou_1"}}}}`
	if string(body) != want {
		t.Fatalf("payload JSON = %s, want %s", body, want)
	}
}
