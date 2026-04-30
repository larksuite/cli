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
