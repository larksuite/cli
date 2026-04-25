// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package source

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"

	"github.com/larksuite/cli/internal/event"
)

func TestRawHandlerLogsMalformedJSON(t *testing.T) {
	var buf bytes.Buffer
	s := &FeishuSource{Logger: log.New(&buf, "", 0)}
	emitted := 0
	handler := s.buildRawHandler(func(_ *event.RawEvent) { emitted++ })

	req := &larkevent.EventReq{Body: []byte("not-json-{{{")}
	if err := handler(context.Background(), req); err != nil {
		t.Fatalf("handler returned err: %v", err)
	}

	if emitted != 0 {
		t.Errorf("expected 0 emits, got %d", emitted)
	}
	out := buf.String()
	if !strings.Contains(out, "malformed") {
		t.Errorf("expected log to mention 'malformed', got: %s", out)
	}
	if !strings.Contains(out, "not-json") {
		t.Errorf("expected log to include body preview, got: %s", out)
	}
}

func TestRawHandlerLogsMissingHeaderFields(t *testing.T) {
	var buf bytes.Buffer
	s := &FeishuSource{Logger: log.New(&buf, "", 0)}
	emitted := 0
	handler := s.buildRawHandler(func(_ *event.RawEvent) { emitted++ })

	req := &larkevent.EventReq{Body: []byte(`{"header":{"event_type":"im.receive"}}`)}
	handler(context.Background(), req)
	req2 := &larkevent.EventReq{Body: []byte(`{"header":{"event_id":"abc"}}`)}
	handler(context.Background(), req2)

	if emitted != 0 {
		t.Errorf("expected 0 emits (both missing fields), got %d", emitted)
	}
	out := buf.String()
	if strings.Count(out, "missing header fields") != 2 {
		t.Errorf("expected 2 'missing header fields' logs, got: %s", out)
	}
}

func TestRawHandlerNilBodyNoLog(t *testing.T) {
	var buf bytes.Buffer
	s := &FeishuSource{Logger: log.New(&buf, "", 0)}
	emitted := 0
	handler := s.buildRawHandler(func(_ *event.RawEvent) { emitted++ })

	req := &larkevent.EventReq{Body: nil}
	handler(context.Background(), req)

	if emitted != 0 {
		t.Errorf("expected 0 emits, got %d", emitted)
	}
	if buf.Len() > 0 {
		t.Errorf("expected no log output, got: %s", buf.String())
	}
}

func TestRawHandlerValidEnvelopeEmits(t *testing.T) {
	s := &FeishuSource{}
	var captured *event.RawEvent
	handler := s.buildRawHandler(func(e *event.RawEvent) { captured = e })

	body := []byte(`{"header":{"event_id":"evt-42","event_type":"im.message.receive_v1","create_time":"1700000000000"}}`)
	handler(context.Background(), &larkevent.EventReq{Body: body})

	if captured == nil {
		t.Fatal("expected emit to fire")
	}
	if captured.EventID != "evt-42" {
		t.Errorf("EventID: got %q, expected evt-42", captured.EventID)
	}
	if captured.EventType != "im.message.receive_v1" {
		t.Errorf("EventType: got %q, expected im.message.receive_v1", captured.EventType)
	}
	if captured.SourceTime != "1700000000000" {
		t.Errorf("SourceTime: got %q, expected 1700000000000", captured.SourceTime)
	}
	if string(captured.Payload) != string(body) {
		t.Errorf("Payload should be raw bytes")
	}
}

func TestRawHandlerNilLoggerDoesNotPanic(t *testing.T) {
	s := &FeishuSource{Logger: nil}
	handler := s.buildRawHandler(func(_ *event.RawEvent) {})
	handler(context.Background(), &larkevent.EventReq{Body: []byte("bad json")})
}
