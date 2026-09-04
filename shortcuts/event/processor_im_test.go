// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"testing"
)

func TestImMessageProcessor_CompactProjectsSyncToChatInfo(t *testing.T) {
	p := &ImMessageProcessor{}
	raw := makeRawEvent("im.message.receive_v1", `{
		"message": {
			"message_id": "om_current",
			"message_type": "text",
			"content": "{\"text\":\"hello\"}",
			"sync_to_chat_info": {
				"type": 1,
				"thread_id": "omt_origin",
				"related_message_id": "om_source",
				"future_relation": "kept"
			}
		}
	}`)

	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if _, loose := result["sync_to_chat_info"].(map[string]interface{}); loose {
		t.Fatalf("sync_to_chat_info = %#v, want typed relation projection", result["sync_to_chat_info"])
	}
	relationJSON, err := json.Marshal(result["sync_to_chat_info"])
	if err != nil {
		t.Fatalf("marshal sync_to_chat_info: %v", err)
	}
	var info map[string]interface{}
	if err := json.Unmarshal(relationJSON, &info); err != nil {
		t.Fatalf("unmarshal sync_to_chat_info: %v", err)
	}
	if info["type"] != float64(1) || info["thread_id"] != "omt_origin" || info["related_message_id"] != "om_source" {
		t.Errorf("sync_to_chat_info = %#v, want exact typed relation values", info)
	}
	if _, ok := info["future_relation"]; ok {
		t.Errorf("sync_to_chat_info.future_relation = %#v, want omitted", info["future_relation"])
	}
}

func TestImMessageProcessor_CompactOmitsUnusableSyncToChatInfo(t *testing.T) {
	for _, tt := range []struct {
		name string
		info string
	}{
		{name: "empty object", info: `{}`},
		{name: "wrong known field type", info: `{"type":"1","related_message_id":"om_source"}`},
		{name: "unsupported type", info: `{"type":3,"related_message_id":"om_source"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := &ImMessageProcessor{}
			raw := makeRawEvent("im.message.receive_v1", `{
				"message": {
					"message_id": "om_current",
					"message_type": "text",
					"content": "{\"text\":\"hello\"}",
					"sync_to_chat_info": `+tt.info+`
				}
			}`)
			result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
			if !ok {
				t.Fatalf("compact output = %T, want map", p.Transform(context.Background(), raw, TransformCompact))
			}
			if _, ok := result["sync_to_chat_info"]; ok {
				t.Fatalf("sync_to_chat_info = %#v, want omitted", result["sync_to_chat_info"])
			}
			if result["message_id"] != "om_current" {
				t.Fatalf("containing event was not preserved: %#v", result)
			}
		})
	}
}

// --- im.message.message_read_v1 ---

func TestImMessageReadProcessor_Compact(t *testing.T) {
	p := &ImMessageReadProcessor{}
	if p.EventType() != "im.message.message_read_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.message.message_read_v1", `{
		"reader": {
			"reader_id": {"open_id": "ou_reader"},
			"read_time": "1700000001"
		},
		"message_id_list": ["msg_001", "msg_002"]
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["type"] != "im.message.message_read_v1" {
		t.Errorf("type = %v", result["type"])
	}
	if result["reader_id"] != "ou_reader" {
		t.Errorf("reader_id = %v", result["reader_id"])
	}
	if result["read_time"] != "1700000001" {
		t.Errorf("read_time = %v", result["read_time"])
	}
	ids, ok := result["message_ids"].([]string)
	if !ok || len(ids) != 2 {
		t.Errorf("message_ids = %v", result["message_ids"])
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

func TestImReactionCreatedProcessor_Compact(t *testing.T) {
	p := NewImReactionCreatedProcessor()
	if p.EventType() != "im.message.reaction.created_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.message.reaction.created_v1", `{
		"message_id": "msg_react",
		"reaction_type": {"emoji_type": "THUMBSUP"},
		"operator_type": "user",
		"user_id": {"open_id": "ou_reactor"},
		"action_time": "1700000002"
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "added" {
		t.Errorf("action = %v, want added", result["action"])
	}
	if result["message_id"] != "msg_react" {
		t.Errorf("message_id = %v", result["message_id"])
	}
	if result["emoji_type"] != "THUMBSUP" {
		t.Errorf("emoji_type = %v", result["emoji_type"])
	}
	if result["operator_id"] != "ou_reactor" {
		t.Errorf("operator_id = %v", result["operator_id"])
	}
	if result["action_time"] != "1700000002" {
		t.Errorf("action_time = %v", result["action_time"])
	}
}

func TestImReactionDeletedProcessor_Compact(t *testing.T) {
	p := NewImReactionDeletedProcessor()
	if p.EventType() != "im.message.reaction.deleted_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.message.reaction.deleted_v1", `{
		"message_id": "msg_unreact",
		"reaction_type": {"emoji_type": "THUMBSUP"},
		"user_id": {"open_id": "ou_reactor"},
		"action_time": "1700000003"
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "removed" {
		t.Errorf("action = %v, want removed", result["action"])
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

func TestImChatBotAddedProcessor_Compact(t *testing.T) {
	p := NewImChatBotAddedProcessor()
	if p.EventType() != "im.chat.member.bot.added_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.member.bot.added_v1", `{
		"chat_id": "oc_bot",
		"operator_id": {"open_id": "ou_operator"},
		"external": false
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "added" {
		t.Errorf("action = %v", result["action"])
	}
	if result["chat_id"] != "oc_bot" {
		t.Errorf("chat_id = %v", result["chat_id"])
	}
	if result["operator_id"] != "ou_operator" {
		t.Errorf("operator_id = %v", result["operator_id"])
	}
	if result["external"] != false {
		t.Errorf("external = %v", result["external"])
	}
}

func TestImChatBotDeletedProcessor_Compact(t *testing.T) {
	p := NewImChatBotDeletedProcessor()
	if p.EventType() != "im.chat.member.bot.deleted_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.member.bot.deleted_v1", `{
		"chat_id": "oc_bot2",
		"operator_id": {"open_id": "ou_op2"},
		"external": true
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "removed" {
		t.Errorf("action = %v, want removed", result["action"])
	}
	if result["external"] != true {
		t.Errorf("external = %v, want true", result["external"])
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

func TestImChatMemberUserAddedProcessor_Compact(t *testing.T) {
	p := NewImChatMemberUserAddedProcessor()
	if p.EventType() != "im.chat.member.user.added_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.member.user.added_v1", `{
		"chat_id": "oc_members",
		"operator_id": {"open_id": "ou_admin"},
		"external": false,
		"users": [
			{"user_id": {"open_id": "ou_user1"}, "name": "Alice"},
			{"user_id": {"open_id": "ou_user2"}, "name": "Bob"}
		]
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "added" {
		t.Errorf("action = %v", result["action"])
	}
	if result["chat_id"] != "oc_members" {
		t.Errorf("chat_id = %v", result["chat_id"])
	}
	if result["operator_id"] != "ou_admin" {
		t.Errorf("operator_id = %v", result["operator_id"])
	}
	userIDs, ok := result["user_ids"].([]string)
	if !ok || len(userIDs) != 2 {
		t.Fatalf("user_ids = %v", result["user_ids"])
	}
	if userIDs[0] != "ou_user1" || userIDs[1] != "ou_user2" {
		t.Errorf("user_ids = %v", userIDs)
	}
}

func TestImChatMemberUserWithdrawnProcessor_Compact(t *testing.T) {
	p := NewImChatMemberUserWithdrawnProcessor()
	if p.EventType() != "im.chat.member.user.withdrawn_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.member.user.withdrawn_v1", `{
		"chat_id": "oc_w",
		"operator_id": {"open_id": "ou_self"},
		"external": false,
		"users": [{"user_id": {"open_id": "ou_self"}, "name": "Self"}]
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "withdrawn" {
		t.Errorf("action = %v, want withdrawn", result["action"])
	}
}

func TestImChatMemberUserDeletedProcessor_Compact(t *testing.T) {
	p := NewImChatMemberUserDeletedProcessor()
	if p.EventType() != "im.chat.member.user.deleted_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.member.user.deleted_v1", `{
		"chat_id": "oc_del",
		"operator_id": {"open_id": "ou_admin"},
		"users": [{"user_id": {"open_id": "ou_kicked"}}]
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["action"] != "removed" {
		t.Errorf("action = %v, want removed", result["action"])
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

func TestImChatUpdatedProcessor_Compact(t *testing.T) {
	p := &ImChatUpdatedProcessor{}
	if p.EventType() != "im.chat.updated_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.updated_v1", `{
		"chat_id": "oc_updated",
		"operator_id": {"open_id": "ou_updater"},
		"external": false,
		"after_change": {"name": "New Name"},
		"before_change": {"name": "Old Name"}
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["type"] != "im.chat.updated_v1" {
		t.Errorf("type = %v", result["type"])
	}
	if result["chat_id"] != "oc_updated" {
		t.Errorf("chat_id = %v", result["chat_id"])
	}
	if result["operator_id"] != "ou_updater" {
		t.Errorf("operator_id = %v", result["operator_id"])
	}
	after, ok := result["after_change"].(map[string]interface{})
	if !ok {
		t.Fatal("after_change should be a map")
	}
	if after["name"] != "New Name" {
		t.Errorf("after_change.name = %v", after["name"])
	}
	before, ok := result["before_change"].(map[string]interface{})
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

func TestImChatDisbandedProcessor_Compact(t *testing.T) {
	p := &ImChatDisbandedProcessor{}
	if p.EventType() != "im.chat.disbanded_v1" {
		t.Fatalf("EventType = %q", p.EventType())
	}
	raw := makeRawEvent("im.chat.disbanded_v1", `{
		"chat_id": "oc_disbanded",
		"operator_id": {"open_id": "ou_disbander"},
		"external": true
	}`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map")
	}
	if result["type"] != "im.chat.disbanded_v1" {
		t.Errorf("type = %v", result["type"])
	}
	if result["chat_id"] != "oc_disbanded" {
		t.Errorf("chat_id = %v", result["chat_id"])
	}
	if result["operator_id"] != "ou_disbander" {
		t.Errorf("operator_id = %v", result["operator_id"])
	}
	if result["external"] != true {
		t.Errorf("external = %v, want true", result["external"])
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
