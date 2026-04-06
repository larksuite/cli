// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type testEventHandler struct {
	id        string
	eventType string
	domain    string
	result    HandlerResult
	called    *[]string
}

func (h *testEventHandler) ID() string { return h.id }

func (h *testEventHandler) EventType() string { return h.eventType }

func (h *testEventHandler) Domain() string { return h.domain }

func (h *testEventHandler) Handle(_ context.Context, _ *Event) HandlerResult {
	if h.called != nil {
		*h.called = append(*h.called, h.id)
	}
	return h.result
}

func TestDispatcher_EventHandlerThenDomainHandlerOrder(t *testing.T) {
	registry := NewHandlerRegistry()
	var calls []string

	eventHandler := &testEventHandler{
		id:        "event-handler",
		eventType: "im.message.receive_v1",
		result:    HandlerResult{Status: HandlerStatusHandled},
		called:    &calls,
	}
	domainHandler := &testEventHandler{
		id:     "domain-handler",
		domain: "im",
		result: HandlerResult{Status: HandlerStatusHandled},
		called: &calls,
	}

	if err := registry.RegisterEventHandler(eventHandler); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}
	if err := registry.RegisterDomainHandler(domainHandler); err != nil {
		t.Fatalf("RegisterDomainHandler() error = %v", err)
	}

	result := NewDispatcher(registry).Dispatch(context.Background(), &Event{
		EventType: "im.message.receive_v1",
		Domain:    "im",
	})

	if got, want := calls, []string{"event-handler", "domain-handler"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
	if len(result.Results) != 2 {
		t.Fatalf("len(result.Results) = %d, want 2", len(result.Results))
	}
	if result.Results[0].HandlerID != "event-handler" || result.Results[1].HandlerID != "domain-handler" {
		t.Fatalf("dispatch results = %+v", result.Results)
	}
}

func TestNewBuiltinHandlerRegistry_RegistersRequiredIMHandlers(t *testing.T) {
	registry := NewBuiltinHandlerRegistry()

	requiredIMEvents := []string{
		"im.message.receive_v1",
		"im.message.message_read_v1",
		"im.message.reaction.created_v1",
		"im.message.reaction.deleted_v1",
		"im.chat.member.bot.added_v1",
		"im.chat.member.bot.deleted_v1",
		"im.chat.member.user.added_v1",
		"im.chat.member.user.withdrawn_v1",
		"im.chat.member.user.deleted_v1",
		"im.chat.updated_v1",
		"im.chat.disbanded_v1",
	}

	subscribedSet := make(map[string]struct{}, len(subscribedEventTypes))
	for _, eventType := range subscribedEventTypes {
		subscribedSet[eventType] = struct{}{}
	}

	for _, eventType := range requiredIMEvents {
		if _, ok := subscribedSet[eventType]; !ok {
			t.Fatalf("subscribedEventTypes missing required IM event %q", eventType)
		}
		handlers := registry.EventHandlers(eventType)
		if len(handlers) != 1 {
			t.Fatalf("EventHandlers(%q) len = %d, want 1", eventType, len(handlers))
		}
		if handlers[0].EventType() != eventType {
			t.Fatalf("EventHandlers(%q)[0].EventType() = %q", eventType, handlers[0].EventType())
		}
		if handlers[0].Domain() != "im" {
			t.Fatalf("EventHandlers(%q)[0].Domain() = %q, want im", eventType, handlers[0].Domain())
		}
	}

	fallback := registry.FallbackHandler()
	if fallback == nil {
		t.Fatal("FallbackHandler() = nil")
	}
	if fallback.ID() != genericHandlerID {
		t.Fatalf("fallback ID = %q, want %q", fallback.ID(), genericHandlerID)
	}
}

func TestDispatcher_UsesFallbackWhenNoHandlersMatch(t *testing.T) {
	registry := NewHandlerRegistry()
	var calls []string
	fallback := &testEventHandler{
		id:     "fallback",
		result: HandlerResult{Status: HandlerStatusSkipped, Reason: "no route"},
		called: &calls,
	}
	registry.SetFallbackHandler(fallback)

	result := NewDispatcher(registry).Dispatch(context.Background(), &Event{
		EventType: "unknown.event",
		Domain:    "unknown",
	})

	if got, want := calls, []string{"fallback"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(result.Results) != 1 {
		t.Fatalf("len(result.Results) = %d, want 1", len(result.Results))
	}
	if result.Results[0].HandlerID != "fallback" {
		t.Fatalf("fallback handler ID = %q, want fallback", result.Results[0].HandlerID)
	}
	if result.Results[0].Status != HandlerStatusSkipped {
		t.Fatalf("fallback status = %q, want %q", result.Results[0].Status, HandlerStatusSkipped)
	}
}

func TestHandlerRegistry_RejectsDuplicateHandlerID(t *testing.T) {
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(&testEventHandler{
		id:        "dup",
		eventType: "im.message.receive_v1",
		result:    HandlerResult{Status: HandlerStatusHandled},
	}); err != nil {
		t.Fatalf("first registration error = %v", err)
	}

	err := registry.RegisterDomainHandler(&testEventHandler{
		id:     "dup",
		domain: "im",
		result: HandlerResult{Status: HandlerStatusHandled},
	})
	if err == nil {
		t.Fatal("expected duplicate handler ID error")
	}
}

func TestDispatcher_FailedHandlerDoesNotStopNextHandler(t *testing.T) {
	registry := NewHandlerRegistry()
	var calls []string
	boom := errors.New("boom")

	failed := &testEventHandler{
		id:        "failed-handler",
		eventType: "im.message.receive_v1",
		result: HandlerResult{
			Status: HandlerStatusFailed,
			Reason: "failed",
			Err:    boom,
		},
		called: &calls,
	}
	next := &testEventHandler{
		id:        "next-handler",
		eventType: "im.message.receive_v1",
		result:    HandlerResult{Status: HandlerStatusHandled},
		called:    &calls,
	}

	if err := registry.RegisterEventHandler(failed); err != nil {
		t.Fatalf("RegisterEventHandler(failed) error = %v", err)
	}
	if err := registry.RegisterEventHandler(next); err != nil {
		t.Fatalf("RegisterEventHandler(next) error = %v", err)
	}

	result := NewDispatcher(registry).Dispatch(context.Background(), &Event{EventType: "im.message.receive_v1"})

	if got, want := calls, []string{"failed-handler", "next-handler"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(result.Results) != 2 {
		t.Fatalf("len(result.Results) = %d, want 2", len(result.Results))
	}
	if result.Results[0].Status != HandlerStatusFailed {
		t.Fatalf("first status = %q, want %q", result.Results[0].Status, HandlerStatusFailed)
	}
	if !errors.Is(result.Results[0].Err, boom) {
		t.Fatalf("first error = %v, want boom", result.Results[0].Err)
	}
	if result.Results[1].Status != HandlerStatusHandled {
		t.Fatalf("second status = %q, want %q", result.Results[1].Status, HandlerStatusHandled)
	}
}
