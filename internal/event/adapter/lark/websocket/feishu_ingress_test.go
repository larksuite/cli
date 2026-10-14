// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package websocket

import (
	"context"
	"encoding/json"
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"

	event "github.com/larksuite/cli/internal/event"
)

func TestCardActionUsesCallbackResponse(t *testing.T) {
	s := &FeishuSource{}
	d := dispatcher.NewEventDispatcher("", "")
	var received *event.RawEvent
	s.registerHandlers(
		d,
		[]string{"card.action.trigger"},
		s.buildRawHandler(func(raw *event.RawEvent) { received = raw }),
	)

	payload := []byte(`{
        "schema":"2.0",
        "header":{"event_id":"evt-card","event_type":"card.action.trigger","create_time":"1"},
        "event":{"operator":{"open_id":"ou_test"},"token":"token","action":{"tag":"select_static","option":"task"}}
    }`)
	response, err := d.Do(context.Background(), payload)
	if err != nil {
		t.Fatalf("dispatch card action: %v", err)
	}
	if received == nil || received.EventID != "evt-card" {
		t.Fatalf("card action was not emitted: %#v", received)
	}
	if _, ok := response.(*callback.CardActionTriggerResponse); !ok {
		t.Fatalf("response type = %T, want *callback.CardActionTriggerResponse", response)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(body) != `{}` {
		t.Fatalf("response = %s, want {}", body)
	}
}

// The websocket ingress is the only place that parses the envelope header;
// every canonical fact consumers rely on must be captured here, once.
func TestBuildRawHandler_ParsesCanonicalHeaderOnce(t *testing.T) {
	s := &FeishuSource{}
	var got *event.RawEvent
	handler := s.buildRawHandler(func(ev *event.RawEvent) { got = ev })

	body := []byte(`{"schema":"2.0","header":{"event_id":"evt-1","event_type":"im.message.receive_v1",` +
		`"create_time":"1700000000000","app_id":"cli_test_app","tenant_key":"tenant_test"},"event":{}}`)
	if err := handler(context.Background(), &larkevent.EventReq{Body: body}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got == nil {
		t.Fatal("event was not emitted")
	}
	if got.EventID != "evt-1" || got.EventType != "im.message.receive_v1" {
		t.Errorf("identity facts wrong: id=%q type=%q", got.EventID, got.EventType)
	}
	if got.SourceTime != "1700000000000" {
		t.Errorf("SourceTime = %q, want upstream create_time", got.SourceTime)
	}
	if got.AppID != "cli_test_app" || got.TenantKey != "tenant_test" {
		t.Errorf("tenant identity not captured: app_id=%q tenant_key=%q", got.AppID, got.TenantKey)
	}
	if got.Timestamp.IsZero() {
		t.Error("local observation Timestamp must be set at ingress")
	}
}

// A header that omits optional facts leaves them visibly empty — the ingress
// never substitutes local configuration for missing upstream facts.
func TestBuildRawHandler_MissingOptionalFactsStayEmpty(t *testing.T) {
	s := &FeishuSource{}
	var got *event.RawEvent
	handler := s.buildRawHandler(func(ev *event.RawEvent) { got = ev })

	body := []byte(`{"schema":"2.0","header":{"event_id":"evt-2","event_type":"im.message.receive_v1"},"event":{}}`)
	if err := handler(context.Background(), &larkevent.EventReq{Body: body}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got == nil {
		t.Fatal("event was not emitted")
	}
	if got.SourceTime != "" || got.AppID != "" || got.TenantKey != "" {
		t.Errorf("missing facts must stay empty: source_time=%q app_id=%q tenant_key=%q",
			got.SourceTime, got.AppID, got.TenantKey)
	}
}
