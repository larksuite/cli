// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestMain(m *testing.M) {
	for _, k := range Keys() {
		event.RegisterKey(k)
	}
	os.Exit(m.Run())
}

func TestIMKeys_ProcessedReceiveRegistered(t *testing.T) {
	def, ok := event.Lookup("im.message.receive_v1")
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
		def, ok := event.Lookup(k)
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
				"sender_id": {"open_id": "ou_sender"}
			},
			"message": {
				"message_id":   "om_text_001",
				"chat_id":      "oc_chat",
				"chat_type":    "p2p",
				"message_type": "text",
				"create_time":  "1776409468987",
				"content":      "{\"text\":\"hello there\"}"
			}
		}
	}`
	out := runReceive(t, payload)

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
	if out.Content != "hello there" {
		t.Errorf("Content = %q, want \"hello there\"", out.Content)
	}
	if out.Timestamp != "1776409469273" {
		t.Errorf("Timestamp = %q", out.Timestamp)
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
	if err != nil {
		t.Fatalf("Process should swallow parse errors, got %v", err)
	}
	if string(got) != "not json" {
		t.Errorf("malformed fallback output = %q, want original bytes", string(got))
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
