// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEncodeDecodeHello(t *testing.T) {
	msg := &Hello{
		Type:       MsgTypeHello,
		PID:        12345,
		EventKey:   "mail.user_mailbox.event.message_received_v1",
		EventTypes: []string{"mail.user_mailbox.event.message_received_v1"},
		Version:    "v1",
	}

	var buf bytes.Buffer
	if err := Encode(&buf, msg); err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	hello, ok := decoded.(*Hello)
	if !ok {
		t.Fatalf("expected *Hello, got %T", decoded)
	}
	if hello.PID != 12345 || hello.EventKey != "mail.user_mailbox.event.message_received_v1" {
		t.Errorf("unexpected hello: %+v", hello)
	}
}

func TestEncodeDecodeEvent(t *testing.T) {
	payload := json.RawMessage(`{"foo":"bar"}`)
	msg := &Event{
		Type:      MsgTypeEvent,
		EventType: "im.message.receive_v1",
		Payload:   payload,
	}

	var buf bytes.Buffer
	if err := Encode(&buf, msg); err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	evt, ok := decoded.(*Event)
	if !ok {
		t.Fatalf("expected *Event, got %T", decoded)
	}
	if evt.EventType != "im.message.receive_v1" {
		t.Errorf("got event_type %q", evt.EventType)
	}
}

func TestEncodeAddsNewline(t *testing.T) {
	msg := &Bye{Type: MsgTypeBye}
	var buf bytes.Buffer
	Encode(&buf, msg)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		t.Error("encoded message should end with newline")
	}
}

func TestDecodeUnknownType(t *testing.T) {
	_, err := Decode([]byte(`{"type":"unknown_xyz"}`))
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestEncodeDecodeHello_WithSubscriptionID(t *testing.T) {
	msg := &Hello{
		Type:           MsgTypeHello,
		PID:            12345,
		EventKey:       "mail.user_mailbox.event.message_received_v1",
		EventTypes:     []string{"mail.user_mailbox.event.message_received_v1"},
		Version:        "v1",
		SubscriptionID: "mail.user_mailbox.event.message_received_v1:a7Bx9Kp2Lm3Qv4Rs",
	}
	buf := &bytes.Buffer{}
	if err := Encode(buf, msg); err != nil {
		t.Fatal(err)
	}
	line := buf.Bytes()
	if !bytes.Contains(line, []byte(`"subscription_id":"mail.user_mailbox.event.message_received_v1:a7Bx9Kp2Lm3Qv4Rs"`)) {
		t.Errorf("subscription_id not serialized: %s", string(line))
	}
	decoded, err := Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := decoded.(*Hello)
	if !ok {
		t.Fatalf("expected *Hello, got %T", decoded)
	}
	if hello.SubscriptionID != msg.SubscriptionID {
		t.Errorf("roundtrip subscription_id: got %q want %q", hello.SubscriptionID, msg.SubscriptionID)
	}
}

func TestEncodeDecodeHello_EmptySubscriptionIDOmitted(t *testing.T) {
	msg := &Hello{
		Type:       MsgTypeHello,
		PID:        1,
		EventKey:   "k",
		EventTypes: []string{"k"},
		Version:    "v1",
	}
	buf := &bytes.Buffer{}
	if err := Encode(buf, msg); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("subscription_id")) {
		t.Errorf("empty subscription_id should be omitted: %s", buf.String())
	}
	decoded, _ := Decode(bytes.TrimRight(buf.Bytes(), "\n"))
	hello := decoded.(*Hello)
	if hello.SubscriptionID != "" {
		t.Errorf("got %q, want empty", hello.SubscriptionID)
	}
}

func TestEncodeDecodePreShutdownCheck_WithSubscriptionID(t *testing.T) {
	msg := &PreShutdownCheck{
		Type:           MsgTypePreShutdownCheck,
		EventKey:       "mail.x",
		SubscriptionID: "mail.x:abc",
	}
	buf := &bytes.Buffer{}
	if err := Encode(buf, msg); err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(bytes.TrimRight(buf.Bytes(), "\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(*PreShutdownCheck)
	if got.SubscriptionID != msg.SubscriptionID {
		t.Errorf("roundtrip: got %q want %q", got.SubscriptionID, msg.SubscriptionID)
	}
}

func TestStatusResponse_ConsumerInfo_SubscriptionID(t *testing.T) {
	msg := NewStatusResponse(7, 120, 1, []ConsumerInfo{
		{PID: 99, EventKey: "mail.x", SubscriptionID: "mail.x:abc", Received: 5, Dropped: 0},
	})
	buf := &bytes.Buffer{}
	if err := Encode(buf, msg); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"subscription_id":"mail.x:abc"`)) {
		t.Errorf("ConsumerInfo.SubscriptionID missing from JSON: %s", buf.String())
	}
}
