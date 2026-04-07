// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"testing"
)

func makeNormalizedEvent(eventType string, data map[string]interface{}) *Event {
	return &Event{
		Source:    SourceWebSocket,
		EventID:   "ev_test",
		EventType: eventType,
		Domain:    "im",
		Payload: NormalizedPayload{
			Header: EventHeader{
				EventID:    "ev_test",
				EventType:  eventType,
				CreateTime: "1700000000",
			},
			Data: data,
		},
		IdempotencyKey: "ws:ev_test",
	}
}

// --- im.message.receive_v1 handler ---

func TestIMMessageHandler_Handle(t *testing.T) {
	h := NewIMMessageReceiveHandler()
	evt := makeNormalizedEvent("im.message.receive_v1", map[string]interface{}{
		"message": map[string]interface{}{
			"message_id":   "om_123",
			"chat_id":      "oc_456",
			"chat_type":    "p2p",
			"message_type": "text",
			"content":      `{"text":"hello"}`,
			"create_time":  "1700000001",
		},
		"sender": map[string]interface{}{
			"sender_id": map[string]interface{}{"open_id": "ou_sender"},
		},
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["message_id"] != "om_123" || out["id"] != "om_123" {
		t.Fatalf("message identifiers = %v", out)
	}
	if out["chat_id"] != "oc_456" {
		t.Fatalf("chat_id = %v", out["chat_id"])
	}
	if out["sender_id"] != "ou_sender" {
		t.Fatalf("sender_id = %v", out["sender_id"])
	}
	if out["content"] != "hello" {
		t.Fatalf("content = %v", out["content"])
	}
}

// --- im.message.message_read_v1 ---

func TestIMMessageReadHandler_Handle(t *testing.T) {
	h := NewIMMessageReadHandler()
	evt := makeNormalizedEvent("im.message.message_read_v1", map[string]interface{}{
		"reader": map[string]interface{}{
			"reader_id": map[string]interface{}{"open_id": "ou_reader"},
			"read_time": "1700000001",
		},
		"message_id_list": []interface{}{"msg_001", "msg_002"},
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["type"] != "im.message.message_read_v1" {
		t.Errorf("type = %v", out["type"])
	}
	if out["reader_id"] != "ou_reader" {
		t.Errorf("reader_id = %v", out["reader_id"])
	}
	if out["read_time"] != "1700000001" {
		t.Errorf("read_time = %v", out["read_time"])
	}
	ids, ok := out["message_ids"].([]string)
	if !ok || len(ids) != 2 {
		t.Errorf("message_ids = %v", out["message_ids"])
	}
}

func TestImMessageReadProcessor_Raw(t *testing.T) {
	p := &ImMessageReadProcessor{}
	raw := makeRawEvent("im.message.message_read_v1", `{}`)
	result, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent)
	if !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
	if result.Header.EventType != "im.message.message_read_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

func TestImMessageReadProcessor_UnmarshalError(t *testing.T) {
	p := &ImMessageReadProcessor{}
	raw := makeRawEvent("im.message.message_read_v1", `not json`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent)
	if !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
	if result.Header.EventType != "im.message.message_read_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

func TestImMessageReadProcessor_Dedup(t *testing.T) {
	p := &ImMessageReadProcessor{}
	raw := makeRawEvent("im.message.message_read_v1", `{}`)
	if k := p.DeduplicateKey(raw); k != "ev_test" {
		t.Errorf("DeduplicateKey = %q", k)
	}
}

// --- im.message.reaction.created_v1 / deleted_v1 ---

func TestIMReactionCreatedHandler_Handle(t *testing.T) {
	h := NewIMReactionCreatedHandler()
	evt := makeNormalizedEvent("im.message.reaction.created_v1", map[string]interface{}{
		"message_id":    "msg_react",
		"reaction_type": map[string]interface{}{"emoji_type": "THUMBSUP"},
		"operator_type": "user",
		"user_id":       map[string]interface{}{"open_id": "ou_reactor"},
		"action_time":   "1700000002",
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "added" {
		t.Errorf("action = %v, want added", out["action"])
	}
	if out["message_id"] != "msg_react" {
		t.Errorf("message_id = %v", out["message_id"])
	}
	if out["emoji_type"] != "THUMBSUP" {
		t.Errorf("emoji_type = %v", out["emoji_type"])
	}
	if out["operator_id"] != "ou_reactor" {
		t.Errorf("operator_id = %v", out["operator_id"])
	}
	if out["action_time"] != "1700000002" {
		t.Errorf("action_time = %v", out["action_time"])
	}
}

func TestIMReactionDeletedHandler_Handle(t *testing.T) {
	h := NewIMReactionDeletedHandler()
	evt := makeNormalizedEvent("im.message.reaction.deleted_v1", map[string]interface{}{
		"message_id":    "msg_unreact",
		"reaction_type": map[string]interface{}{"emoji_type": "THUMBSUP"},
		"user_id":       map[string]interface{}{"open_id": "ou_reactor"},
		"action_time":   "1700000003",
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "removed" {
		t.Errorf("action = %v, want removed", out["action"])
	}
}

func TestImReactionProcessor_Raw(t *testing.T) {
	p := NewImReactionCreatedProcessor()
	raw := makeRawEvent("im.message.reaction.created_v1", `{}`)
	if _, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent); !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
}

func TestImReactionProcessor_UnmarshalError(t *testing.T) {
	p := NewImReactionCreatedProcessor()
	raw := makeRawEvent("im.message.reaction.created_v1", `bad`)
	if _, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent); !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
}

// --- im.chat.member.bot.added_v1 / deleted_v1 ---

func TestIMChatMemberBotAddedHandler_Handle(t *testing.T) {
	h := NewIMChatMemberBotAddedHandler()
	evt := makeNormalizedEvent("im.chat.member.bot.added_v1", map[string]interface{}{
		"chat_id":     "oc_bot",
		"operator_id": map[string]interface{}{"open_id": "ou_operator"},
		"external":    false,
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "added" {
		t.Errorf("action = %v", out["action"])
	}
	if out["chat_id"] != "oc_bot" {
		t.Errorf("chat_id = %v", out["chat_id"])
	}
	if out["operator_id"] != "ou_operator" {
		t.Errorf("operator_id = %v", out["operator_id"])
	}
	if out["external"] != false {
		t.Errorf("external = %v", out["external"])
	}
}

func TestIMChatMemberBotDeletedHandler_Handle(t *testing.T) {
	h := NewIMChatMemberBotDeletedHandler()
	evt := makeNormalizedEvent("im.chat.member.bot.deleted_v1", map[string]interface{}{
		"chat_id":     "oc_bot2",
		"operator_id": map[string]interface{}{"open_id": "ou_op2"},
		"external":    true,
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "removed" {
		t.Errorf("action = %v, want removed", out["action"])
	}
	if out["external"] != true {
		t.Errorf("external = %v, want true", out["external"])
	}
}

func TestImChatBotProcessor_Raw(t *testing.T) {
	p := NewImChatBotAddedProcessor()
	raw := makeRawEvent("im.chat.member.bot.added_v1", `{}`)
	if _, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent); !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
}

func TestImChatBotProcessor_UnmarshalError(t *testing.T) {
	p := NewImChatBotAddedProcessor()
	raw := makeRawEvent("im.chat.member.bot.added_v1", `{bad}`)
	if _, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent); !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
}

// --- im.chat.member.user.added_v1 / withdrawn_v1 / deleted_v1 ---

func TestIMChatMemberUserAddedHandler_Handle(t *testing.T) {
	h := NewIMChatMemberUserAddedHandler()
	evt := makeNormalizedEvent("im.chat.member.user.added_v1", map[string]interface{}{
		"chat_id":     "oc_members",
		"operator_id": map[string]interface{}{"open_id": "ou_admin"},
		"external":    false,
		"users": []interface{}{
			map[string]interface{}{"user_id": map[string]interface{}{"open_id": "ou_user1"}, "name": "Alice"},
			map[string]interface{}{"user_id": map[string]interface{}{"open_id": "ou_user2"}, "name": "Bob"},
		},
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "added" {
		t.Errorf("action = %v", out["action"])
	}
	if out["chat_id"] != "oc_members" {
		t.Errorf("chat_id = %v", out["chat_id"])
	}
	if out["operator_id"] != "ou_admin" {
		t.Errorf("operator_id = %v", out["operator_id"])
	}
	userIDs, ok := out["user_ids"].([]string)
	if !ok || len(userIDs) != 2 {
		t.Fatalf("user_ids = %v", out["user_ids"])
	}
	if userIDs[0] != "ou_user1" || userIDs[1] != "ou_user2" {
		t.Errorf("user_ids = %v", userIDs)
	}
}

func TestIMChatMemberUserWithdrawnHandler_Handle(t *testing.T) {
	h := NewIMChatMemberUserWithdrawnHandler()
	evt := makeNormalizedEvent("im.chat.member.user.withdrawn_v1", map[string]interface{}{
		"chat_id":     "oc_w",
		"operator_id": map[string]interface{}{"open_id": "ou_self"},
		"external":    false,
		"users":       []interface{}{map[string]interface{}{"user_id": map[string]interface{}{"open_id": "ou_self"}, "name": "Self"}},
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "withdrawn" {
		t.Errorf("action = %v, want withdrawn", out["action"])
	}
}

func TestIMChatMemberUserDeletedHandler_Handle(t *testing.T) {
	h := NewIMChatMemberUserDeletedHandler()
	evt := makeNormalizedEvent("im.chat.member.user.deleted_v1", map[string]interface{}{
		"chat_id":     "oc_del",
		"operator_id": map[string]interface{}{"open_id": "ou_admin"},
		"users":       []interface{}{map[string]interface{}{"user_id": map[string]interface{}{"open_id": "ou_kicked"}}},
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["action"] != "removed" {
		t.Errorf("action = %v, want removed", out["action"])
	}
}

func TestImChatMemberUserProcessor_Raw(t *testing.T) {
	p := NewImChatMemberUserAddedProcessor()
	raw := makeRawEvent("im.chat.member.user.added_v1", `{}`)
	if _, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent); !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
}

func TestImChatMemberUserProcessor_UnmarshalError(t *testing.T) {
	p := NewImChatMemberUserAddedProcessor()
	raw := makeRawEvent("im.chat.member.user.added_v1", `bad json`)
	if _, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent); !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
}

// --- im.chat.updated_v1 ---

func TestIMChatUpdatedHandler_Handle(t *testing.T) {
	h := NewIMChatUpdatedHandler()
	evt := makeNormalizedEvent("im.chat.updated_v1", map[string]interface{}{
		"chat_id":       "oc_updated",
		"operator_id":   map[string]interface{}{"open_id": "ou_updater"},
		"external":      false,
		"after_change":  map[string]interface{}{"name": "New Name"},
		"before_change": map[string]interface{}{"name": "Old Name"},
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["type"] != "im.chat.updated_v1" {
		t.Errorf("type = %v", out["type"])
	}
	if out["chat_id"] != "oc_updated" {
		t.Errorf("chat_id = %v", out["chat_id"])
	}
	if out["operator_id"] != "ou_updater" {
		t.Errorf("operator_id = %v", out["operator_id"])
	}
	after, ok := out["after_change"].(map[string]interface{})
	if !ok {
		t.Fatal("after_change should be a map")
	}
	if after["name"] != "New Name" {
		t.Errorf("after_change.name = %v", after["name"])
	}
	before, ok := out["before_change"].(map[string]interface{})
	if !ok {
		t.Fatal("before_change should be a map")
	}
	if before["name"] != "Old Name" {
		t.Errorf("before_change.name = %v", before["name"])
	}
}

func TestImChatUpdatedProcessor_Raw(t *testing.T) {
	p := &ImChatUpdatedProcessor{}
	raw := makeRawEvent("im.chat.updated_v1", `{}`)
	if _, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent); !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
}

func TestImChatUpdatedProcessor_UnmarshalError(t *testing.T) {
	p := &ImChatUpdatedProcessor{}
	raw := makeRawEvent("im.chat.updated_v1", `???`)
	if _, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent); !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
}

// --- im.chat.disbanded_v1 ---

func TestIMChatDisbandedHandler_Handle(t *testing.T) {
	h := NewIMChatDisbandedHandler()
	evt := makeNormalizedEvent("im.chat.disbanded_v1", map[string]interface{}{
		"chat_id":     "oc_disbanded",
		"operator_id": map[string]interface{}{"open_id": "ou_disbander"},
		"external":    true,
	})

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be compact map")
	}
	if out["type"] != "im.chat.disbanded_v1" {
		t.Errorf("type = %v", out["type"])
	}
	if out["chat_id"] != "oc_disbanded" {
		t.Errorf("chat_id = %v", out["chat_id"])
	}
	if out["operator_id"] != "ou_disbander" {
		t.Errorf("operator_id = %v", out["operator_id"])
	}
	if out["external"] != true {
		t.Errorf("external = %v, want true", out["external"])
	}
}

func TestImChatDisbandedProcessor_Raw(t *testing.T) {
	p := &ImChatDisbandedProcessor{}
	raw := makeRawEvent("im.chat.disbanded_v1", `{}`)
	if _, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent); !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
}

func TestImChatDisbandedProcessor_UnmarshalError(t *testing.T) {
	p := &ImChatDisbandedProcessor{}
	raw := makeRawEvent("im.chat.disbanded_v1", `nope`)
	if _, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent); !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
}

// --- generic fallback handler ---

func TestGenericFallbackHandler_HandleUnknownEvent(t *testing.T) {
	h := NewGenericFallbackHandler()
	evt := &Event{
		Source:    SourceWebhook,
		EventType: "unknown.event",
		EventID:   "ev_unknown",
		Domain:    DomainUnknown,
		Payload: NormalizedPayload{
			Header: EventHeader{EventID: "ev_unknown", EventType: "unknown.event", CreateTime: "1700000009"},
			Data:   map[string]interface{}{"foo": "bar"},
		},
		RawPayload: []byte(`{"foo":"bar","nested":{"x":1}}`),
	}

	result := h.Handle(context.Background(), evt)
	if result.Status != HandlerStatusHandled {
		t.Fatalf("status = %q", result.Status)
	}
	out, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatal("output should be a map")
	}
	if out["type"] != "unknown.event" {
		t.Fatalf("type = %v", out["type"])
	}
	if out["event_id"] != "ev_unknown" {
		t.Fatalf("event_id = %v", out["event_id"])
	}
	if out["foo"] != "bar" {
		t.Fatalf("foo = %v", out["foo"])
	}
	if out["raw_payload"] != `{"foo":"bar","nested":{"x":1}}` {
		t.Fatalf("raw_payload = %v", out["raw_payload"])
	}
}

// --- Registry: all IM processors registered ---

func TestRegistryAllIMProcessors(t *testing.T) {
	r := DefaultRegistry()
	imTypes := []string{
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
	for _, et := range imTypes {
		p := r.Lookup(et)
		if p.EventType() != et {
			t.Errorf("Lookup(%q) returned processor with EventType=%q", et, p.EventType())
		}
	}
}

// --- Helper: openID ---

func TestOpenID(t *testing.T) {
	if id := openID(map[string]interface{}{"open_id": "ou_x"}); id != "ou_x" {
		t.Errorf("got %q", id)
	}
	if id := openID("not a map"); id != "" {
		t.Errorf("non-map should return empty, got %q", id)
	}
	if id := openID(nil); id != "" {
		t.Errorf("nil should return empty, got %q", id)
	}
}

// --- Helper: extractUserIDs ---

func TestExtractUserIDs(t *testing.T) {
	users := []interface{}{
		map[string]interface{}{
			"user_id": map[string]interface{}{"open_id": "ou_a"},
			"name":    "Alice",
		},
		map[string]interface{}{
			"user_id": map[string]interface{}{"open_id": "ou_b"},
		},
		"not a map",
		map[string]interface{}{
			"user_id": "not nested",
		},
	}
	ids := extractUserIDs(users)
	if len(ids) != 2 || ids[0] != "ou_a" || ids[1] != "ou_b" {
		t.Errorf("extractUserIDs = %v, want [ou_a, ou_b]", ids)
	}
}

func TestExtractUserIDs_Empty(t *testing.T) {
	ids := extractUserIDs(nil)
	if len(ids) != 0 {
		t.Errorf("nil input should return empty, got %v", ids)
	}
}

// --- WindowStrategy for all new processors ---

func TestWindowStrategy_IMProcessors(t *testing.T) {
	processors := []EventProcessor{
		&ImMessageReadProcessor{},
		NewImReactionCreatedProcessor(),
		NewImReactionDeletedProcessor(),
		NewImChatBotAddedProcessor(),
		NewImChatBotDeletedProcessor(),
		NewImChatMemberUserAddedProcessor(),
		NewImChatMemberUserWithdrawnProcessor(),
		NewImChatMemberUserDeletedProcessor(),
		&ImChatUpdatedProcessor{},
		&ImChatDisbandedProcessor{},
	}
	for _, p := range processors {
		if p.WindowStrategy() != (WindowConfig{}) {
			t.Errorf("%s: WindowStrategy should return zero WindowConfig", p.EventType())
		}
	}
}
