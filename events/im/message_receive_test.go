// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/processing"
)

func TestIMKeys_ProcessedReceiveRegistered(t *testing.T) {
	def, ok := lookupCompiledDef(t, "im.message.receive_v1")
	if !ok {
		t.Fatal("im.message.receive_v1 should be registered via Keys()")
	}
	if def.Schema.Custom == nil {
		t.Error("Processed key must set Schema.Custom")
	}
	if def.Schema.Native != nil {
		t.Error("Processed key must not set Schema.Native")
	}
	if def.Process == nil {
		t.Error("Process must not be nil for Processed key")
	}
	if len(def.Scopes) == 0 {
		t.Error("Scopes must not be empty — preflightScopes would bypass validation")
	}
}

func TestIMKeys_NativeEventsRegistered(t *testing.T) {
	want := []string{
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
	for _, k := range want {
		def, ok := lookupCompiledDef(t, k)
		if !ok {
			t.Errorf("%s should be registered via Keys()", k)
			continue
		}
		if def.Schema.Native == nil {
			t.Errorf("%s: Schema.Native must be set for native key", k)
		}
		if def.Schema.Custom != nil {
			t.Errorf("%s: Native key must not set Schema.Custom", k)
		}
		if def.Process != nil {
			t.Errorf("%s: Native key must not set Process", k)
		}
		if def.Schema.Native != nil && def.Schema.Native.Type == nil {
			t.Errorf("%s: Schema.Native.Type must reference an SDK type", k)
		}
	}
}

func TestProcessImMessageReceive_Text(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_test_text",
			"event_type": "im.message.receive_v1",
			"create_time": "1776409469273",
			"app_id": "cli_test"
		},
		"event": {
			"sender": {
				"sender_type": "user",
				"sender_id": {"open_id": "ou_sender"}
			},
			"message": {
				"message_id":   "om_text_001",
				"root_id":      "om_root_001",
				"parent_id":    "om_parent_001",
				"thread_id":    "omt_thread_001",
				"chat_id":      "oc_chat",
				"chat_type":    "p2p",
				"message_type": "text",
				"create_time":  "1776409468987",
				"update_time":  "1776409469999",
				"content":      "{\"text\":\"hello @_user_1\"}",
				"mentions": [
					{
						"key": "@_user_1",
						"id": {"open_id": "ou_mentioned"},
						"name": "Alice"
					}
				]
			}
		}
	}`
	out := runReceive(t, payload)
	outMap := runReceiveMap(t, payload)

	if out.Type != "im.message.receive_v1" {
		t.Errorf("Type = %q", out.Type)
	}
	if out.MessageID != "om_text_001" || out.ID != "om_text_001" {
		t.Errorf("MessageID/ID = %q/%q", out.MessageID, out.ID)
	}
	if out.ChatType != "p2p" || out.ChatID != "oc_chat" {
		t.Errorf("chat_id/chat_type = %q/%q", out.ChatID, out.ChatType)
	}
	if out.SenderID != "ou_sender" {
		t.Errorf("SenderID = %q", out.SenderID)
	}
	if out.Content != "hello @Alice" {
		t.Errorf("Content = %q, want \"hello @Alice\"", out.Content)
	}
	if out.Timestamp != "1776409469273" {
		t.Errorf("Timestamp = %q", out.Timestamp)
	}
	for field, want := range map[string]string{
		"sender_type": "user",
		"root_id":     "om_root_001",
		"thread_id":   "omt_thread_001",
		"reply_to":    "om_parent_001",
		"update_time": "1776409469999",
	} {
		if got, _ := outMap[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	mentions, _ := outMap["mentions"].([]interface{})
	if len(mentions) != 1 {
		t.Fatalf("mentions length = %d, want 1: %#v", len(mentions), outMap["mentions"])
	}
	mention, _ := mentions[0].(map[string]interface{})
	for field, want := range map[string]string{
		"key":  "@_user_1",
		"id":   "ou_mentioned",
		"name": "Alice",
	} {
		if got, _ := mention[field].(string); got != want {
			t.Errorf("mentions[0].%s = %q, want %q", field, got, want)
		}
	}
}

func TestProcessImMessageReceive_OmitsUnchangedUpdateTime(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_test_text",
			"event_type": "im.message.receive_v1",
			"create_time": "1776409469273",
			"app_id": "cli_test"
		},
		"event": {
			"sender": {
				"sender_type": "user",
				"sender_id": {"open_id": "ou_sender"}
			},
			"message": {
				"message_id":   "om_text_001",
				"chat_id":      "oc_chat",
				"chat_type":    "p2p",
				"message_type": "text",
				"create_time":  "1776409468987",
				"update_time":  "1776409468987",
				"content":      "{\"text\":\"hello there\"}"
			}
		}
	}`
	outMap := runReceiveMap(t, payload)

	if _, ok := outMap["update_time"]; ok {
		t.Errorf("update_time should be omitted when it equals create_time: %#v", outMap)
	}
}

func TestProcessImMessageReceive_SyncToChatInfo(t *testing.T) {
	for _, tt := range []struct {
		name             string
		info             string
		typeValue        float64
		relatedMessageID string
		threadID         string
	}{
		{name: "target", info: `{"type":1,"thread_id":"omt_origin","related_message_id":"om_source","future_relation":"kept"}`, typeValue: 1, relatedMessageID: "om_source", threadID: "omt_origin"},
		{name: "source", info: `{"type":2,"related_message_id":"om_target"}`, typeValue: 2, relatedMessageID: "om_target"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{
				"schema":"2.0",
				"header":{"event_id":"ev_relation","event_type":"im.message.receive_v1"},
				"event":{"message":{"message_id":"om_current","sync_to_chat_info":` + tt.info + `}}
			}`
			out := runReceiveMap(t, payload)
			info, ok := out["sync_to_chat_info"].(map[string]interface{})
			if !ok {
				t.Fatalf("sync_to_chat_info = %#v, want object", out["sync_to_chat_info"])
			}
			if info["type"] != tt.typeValue {
				t.Errorf("sync_to_chat_info.type = %#v, want %#v", info["type"], tt.typeValue)
			}
			if info["related_message_id"] != tt.relatedMessageID {
				t.Errorf("sync_to_chat_info.related_message_id = %#v, want %q", info["related_message_id"], tt.relatedMessageID)
			}
			if tt.threadID == "" {
				if _, ok := info["thread_id"]; ok {
					t.Errorf("sync_to_chat_info.thread_id = %#v, want omitted", info["thread_id"])
				}
			} else if info["thread_id"] != tt.threadID {
				t.Errorf("sync_to_chat_info.thread_id = %#v, want %q", info["thread_id"], tt.threadID)
			}
			if _, ok := info["future_relation"]; ok {
				t.Errorf("sync_to_chat_info.future_relation = %#v, want omitted", info["future_relation"])
			}
		})
	}
}

func TestProcessImMessageReceive_OmitsUnusableSyncToChatInfo(t *testing.T) {
	for _, tt := range []struct {
		name string
		info string
	}{
		{name: "empty object", info: `{}`},
		{name: "wrong known field type", info: `{"type":"1","related_message_id":"om_source"}`},
		{name: "unsupported type", info: `{"type":3,"related_message_id":"om_source"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{
				"schema":"2.0",
				"header":{"event_id":"ev_relation","event_type":"im.message.receive_v1"},
				"event":{"message":{"message_id":"om_current","sync_to_chat_info":` + tt.info + `}}
			}`
			out := runReceiveMap(t, payload)
			if _, ok := out["sync_to_chat_info"]; ok {
				t.Fatalf("sync_to_chat_info = %#v, want omitted", out["sync_to_chat_info"])
			}
			if out["message_id"] != "om_current" {
				t.Fatalf("containing event was not preserved: %#v", out)
			}
		})
	}
}

func TestProcessImMessageReceive_OmitsMissingSyncToChatInfo(t *testing.T) {
	payload := `{"schema":"2.0","header":{"event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_legacy"}}}`
	out := runReceiveMap(t, payload)
	if _, ok := out["sync_to_chat_info"]; ok {
		t.Fatalf("sync_to_chat_info = %#v, want omitted", out["sync_to_chat_info"])
	}
}

func TestProcessImMessageReceive_Interactive(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_test_card",
			"event_type": "im.message.receive_v1",
			"create_time": "1776409469274",
			"app_id": "cli_test"
		},
		"event": {
			"sender": {
				"sender_id": {"open_id": "ou_sender"}
			},
			"message": {
				"message_id":   "om_card_001",
				"chat_id":      "oc_chat",
				"chat_type":    "group",
				"message_type": "interactive",
				"create_time":  "1776409468987",
				"content":      "{\"header\":{\"title\":{\"tag\":\"plain_text\",\"content\":\"A card\"}}}"
			}
		}
	}`
	out := runReceive(t, payload)

	if out.Type != "im.message.receive_v1" {
		t.Errorf("Type = %q", out.Type)
	}
	if out.MessageType != "interactive" {
		t.Errorf("MessageType = %q", out.MessageType)
	}
	if out.ChatType != "group" {
		t.Errorf("ChatType = %q", out.ChatType)
	}
}

func TestProcessImMessageReceive_MalformedPayload(t *testing.T) {
	raw := &event.RawEvent{
		EventID:   "ev_bad",
		EventType: "im.message.receive_v1",
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processImMessageReceive(context.Background(), nil, raw, nil)
	if !processing.IsDropMalformed(err) {
		t.Fatalf("malformed payload must be dropped with a malformed marker, got err=%v", err)
	}
	if got != nil {
		t.Errorf("malformed payload must be dropped without output, got %q", string(got))
	}
}

func runReceive(t *testing.T, payload string) ImMessageReceiveOutput {
	t.Helper()
	raw := &event.RawEvent{
		EventID:   "ev_test",
		EventType: "im.message.receive_v1",
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	fillCanonicalFromHeader(t, raw)
	got, err := processImMessageReceive(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var out ImMessageReceiveOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid ImMessageReceiveOutput JSON: %v\nraw=%s", err, string(got))
	}
	return out
}

func runReceiveMap(t *testing.T, payload string) map[string]interface{} {
	t.Helper()
	raw := &event.RawEvent{
		EventID:   "ev_test",
		EventType: "im.message.receive_v1",
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	fillCanonicalFromHeader(t, raw)
	got, err := processImMessageReceive(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid JSON: %v\nraw=%s", err, string(got))
	}
	return out
}
