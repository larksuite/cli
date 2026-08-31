// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestCompactMessageListDataHoistsRepeatedContext(t *testing.T) {
	sender := map[string]interface{}{
		"id":                "ou_alice",
		"id_type":           "open_id",
		"sender_type":       "user",
		"name":              "Alice",
		"sender_i18n_names": map[string]interface{}{"en_us": "Alice"},
	}
	messages := []map[string]interface{}{
		{
			"message_id": "om_root", "chat_id": "oc_chat", "sender": sender,
			"content": "root", "reactions": map[string]interface{}{"counts": []interface{}{map[string]interface{}{"reaction_type": "OK", "count": 1}}},
			"thread_replies": []map[string]interface{}{{
				"message_id": "om_reply", "chat_id": "oc_chat", "sender": sender, "content": "reply",
			}},
		},
		{"message_id": "om_second", "chat_id": "oc_chat", "sender": sender, "content": "second"},
	}

	got := compactMessageListData(messages, "oc_chat", "", true, "next")
	if got["chat_id"] != "oc_chat" || got["has_more"] != true || got["page_token"] != "next" {
		t.Fatalf("top-level context = %#v", got)
	}
	participants, ok := got["participants"].(map[string]map[string]interface{})
	wantParticipant := cloneStringMap(sender)
	delete(wantParticipant, "id")
	if !ok || len(participants) != 1 || !reflect.DeepEqual(participants["ou_alice"], wantParticipant) {
		t.Fatalf("participants = %#v, want sender metadata keyed by id", got["participants"])
	}
	projected := got["messages"].([]map[string]interface{})
	for index, message := range projected {
		if message["sender_id"] != "ou_alice" {
			t.Fatalf("message %d sender_id = %#v", index, message["sender_id"])
		}
		if _, exists := message["sender"]; exists {
			t.Fatalf("message %d retained repeated sender: %#v", index, message)
		}
		if _, exists := message["chat_id"]; exists {
			t.Fatalf("message %d retained repeated chat_id: %#v", index, message)
		}
	}
	reply := projected[0]["thread_replies"].([]map[string]interface{})[0]
	if reply["sender_id"] != "ou_alice" || reply["content"] != "reply" {
		t.Fatalf("projected reply = %#v", reply)
	}
	if _, ok := projected[0]["reactions"]; !ok {
		t.Fatalf("reactions were lost: %#v", projected[0])
	}

	// Projection must not mutate the enriched source used by other formats.
	if messages[0]["chat_id"] != "oc_chat" || messages[0]["sender"] == nil {
		t.Fatalf("source message mutated: %#v", messages[0])
	}
}

func TestCompactMessageListDataKeepsUnsafeSenderInline(t *testing.T) {
	messages := []map[string]interface{}{
		{"message_id": "om_1", "sender": map[string]interface{}{"id": "ou_same", "name": "Old Name"}},
		{"message_id": "om_2", "sender": map[string]interface{}{"id": "ou_same", "name": "New Name"}},
		{"message_id": "om_system", "sender": map[string]interface{}{"sender_type": "system"}},
	}

	got := compactMessageListData(messages, "", "omt_thread", false, "")
	if _, exists := got["participants"]; exists {
		t.Fatalf("conflicting sender must not be hoisted: %#v", got["participants"])
	}
	projected := got["messages"].([]map[string]interface{})
	for index, message := range projected {
		if _, exists := message["sender"]; !exists {
			t.Fatalf("message %d lost inline sender: %#v", index, message)
		}
		if _, exists := message["sender_id"]; exists {
			t.Fatalf("message %d has unsafe sender reference: %#v", index, message)
		}
	}
}

func TestCompactMessageListDataHoistsThreadID(t *testing.T) {
	messages := []map[string]interface{}{
		{"message_id": "om_1", "thread_id": "omt_thread"},
		{"message_id": "om_2", "thread_id": "omt_thread"},
	}
	got := compactMessageListData(messages, "", "omt_thread", false, "")
	for index, message := range got["messages"].([]map[string]interface{}) {
		if _, exists := message["thread_id"]; exists {
			t.Fatalf("message %d retained repeated thread_id: %#v", index, message)
		}
	}
}

func TestCompactMessageListDataDoesNotHoistMixedChatIDs(t *testing.T) {
	messages := []map[string]interface{}{
		{"message_id": "om_1", "chat_id": "oc_a"},
		{"message_id": "om_2", "chat_id": "oc_b"},
	}
	got := compactMessageListData(messages, "", "omt_thread", false, "")
	if _, exists := got["chat_id"]; exists {
		t.Fatalf("mixed chat_id was hoisted: %#v", got)
	}
	for index, message := range got["messages"].([]map[string]interface{}) {
		if _, exists := message["chat_id"]; !exists {
			t.Fatalf("message %d lost its chat_id: %#v", index, message)
		}
	}
}

func TestMessageListOutputDataOnlyCompactsJSON(t *testing.T) {
	messages := []map[string]interface{}{{
		"message_id": "om_1", "chat_id": "oc_chat",
		"sender": map[string]interface{}{"id": "ou_alice", "name": "Alice"},
	}}

	for _, tc := range []struct {
		name, format, jq string
		wantCompact      bool
	}{
		{name: "default JSON", format: "json", wantCompact: true},
		{name: "case-insensitive JSON", format: "JSON", wantCompact: true},
		{name: "jq envelope", format: "table", jq: ".data", wantCompact: true},
		{name: "unknown falls back to JSON", format: "other", wantCompact: true},
		{name: "uppercase pretty falls back to JSON", format: "Pretty", wantCompact: true},
		{name: "pretty", format: "pretty"},
		{name: "table", format: "table"},
		{name: "csv", format: "csv"},
		{name: "ndjson", format: "ndjson"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := messageListOutputData(tc.format, tc.jq, messages, "oc_chat", "", false, "")
			message := got["messages"].([]map[string]interface{})[0]
			_, compact := message["sender_id"]
			if compact != tc.wantCompact {
				t.Fatalf("sender_id present = %t, want %t; output = %#v", compact, tc.wantCompact, got)
			}
			if !tc.wantCompact {
				if message["chat_id"] != "oc_chat" || message["sender"] == nil {
					t.Fatalf("legacy format lost inline context: %#v", got)
				}
				if _, exists := got["participants"]; exists {
					t.Fatalf("legacy format gained participants: %#v", got)
				}
			}
		})
	}
}

func TestCompactMessageListDataReducesRepeatedJSON(t *testing.T) {
	sender := map[string]interface{}{
		"id": "ou_alice", "id_type": "open_id", "sender_type": "user",
		"name": "Alice Example",
		"sender_i18n_names": map[string]interface{}{
			"en_us": "Alice Example", "zh_cn": "Alice Example",
		},
	}
	messages := make([]map[string]interface{}, 20)
	for index := range messages {
		messages[index] = map[string]interface{}{
			"message_id": "om_repeated", "chat_id": "oc_chat", "sender": sender,
			"msg_type": "text", "content": "same-sized message body",
		}
	}
	legacy, err := json.Marshal(map[string]interface{}{"messages": messages})
	if err != nil {
		t.Fatal(err)
	}
	compact, err := json.Marshal(compactMessageListData(messages, "oc_chat", "", false, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(compact)*4 >= len(legacy)*3 {
		t.Fatalf("compact JSON size = %d, legacy = %d; want at least 25%% reduction", len(compact), len(legacy))
	}
}

func TestChatMessagesListEmitsCompactJSONContract(t *testing.T) {
	var tc listPageAllCase
	for _, candidate := range listPageAllCases() {
		if candidate.name == "chat-messages-list" {
			tc = candidate
			break
		}
	}
	runtime, calls := newListPageAllRuntime(t, tc, nil, func(req *http.Request, _ int) map[string]interface{} {
		if got := req.URL.Query().Get("container_id"); got != "oc_test" {
			t.Fatalf("container_id = %q, want oc_test", got)
		}
		return map[string]interface{}{
			"items": []interface{}{map[string]interface{}{
				"message_id": "om_1", "msg_type": "text", "chat_id": "oc_test",
				"sender": map[string]interface{}{"id": "ou_alice", "sender_type": "user", "sender_name": "Alice"},
				"body":   map[string]interface{}{"content": `{"text":"full content"}`}, "create_time": "0",
			}},
			"has_more": false, "page_token": "final",
		}
	})
	if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if *calls != 1 {
		t.Fatalf("API calls = %d, want 1", *calls)
	}
	data := listPageAllOutputData(t, runtime)
	if data["chat_id"] != "oc_test" || data["page_token"] != "final" {
		t.Fatalf("top-level data = %#v", data)
	}
	participants := data["participants"].(map[string]interface{})
	alice := participants["ou_alice"].(map[string]interface{})
	if alice["name"] != "Alice" || alice["sender_type"] != "user" {
		t.Fatalf("participant = %#v", alice)
	}
	message := data["messages"].([]interface{})[0].(map[string]interface{})
	if message["sender_id"] != "ou_alice" || message["content"] != "full content" {
		t.Fatalf("message = %#v", message)
	}
	if _, exists := message["sender"]; exists {
		t.Fatalf("message retained sender: %#v", message)
	}
	if _, exists := message["chat_id"]; exists {
		t.Fatalf("message retained chat_id: %#v", message)
	}
}

func TestThreadsMessagesListEmitsCompactJSONContract(t *testing.T) {
	var tc listPageAllCase
	for _, candidate := range listPageAllCases() {
		if candidate.name == "threads-messages-list" {
			tc = candidate
			break
		}
	}
	runtime, _ := newListPageAllRuntime(t, tc, nil, func(req *http.Request, _ int) map[string]interface{} {
		if got := req.URL.Query().Get("container_id"); got != "omt_test" {
			t.Fatalf("container_id = %q, want omt_test", got)
		}
		return map[string]interface{}{
			"items": []interface{}{map[string]interface{}{
				"message_id": "om_reply", "thread_id": "omt_test", "msg_type": "text",
				"sender": map[string]interface{}{"id": "ou_bob", "sender_type": "user", "sender_name": "Bob"},
				"body":   map[string]interface{}{"content": `{"text":"reply"}`}, "create_time": "0",
			}},
			"has_more": true, "page_token": "next",
		}
	})
	if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	data := listPageAllOutputData(t, runtime)
	if data["thread_id"] != "omt_test" || data["has_more"] != true || data["page_token"] != "next" {
		t.Fatalf("top-level data = %#v", data)
	}
	message := data["messages"].([]interface{})[0].(map[string]interface{})
	if message["sender_id"] != "ou_bob" || message["content"] != "reply" {
		t.Fatalf("message = %#v", message)
	}
	if _, exists := message["thread_id"]; exists {
		t.Fatalf("message retained repeated thread_id: %#v", message)
	}
}
