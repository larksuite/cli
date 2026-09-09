// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
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

	// Projection must not mutate the enriched source used by other formats,
	// including nested thread replies.
	if messages[0]["chat_id"] != "oc_chat" || messages[0]["sender"] == nil {
		t.Fatalf("source message mutated: %#v", messages[0])
	}
	sourceReply := messages[0]["thread_replies"].([]map[string]interface{})[0]
	if sourceReply["chat_id"] != "oc_chat" || sourceReply["sender"] == nil {
		t.Fatalf("source thread reply mutated: %#v", sourceReply)
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

func TestCompactMessageListDataDoesNotHoistPartiallyPresentChatID(t *testing.T) {
	for _, missingValue := range []interface{}{nil, 42} {
		messages := []map[string]interface{}{
			{"message_id": "om_1", "chat_id": "oc_a"},
			{"message_id": "om_2", "chat_id": missingValue},
		}
		if missingValue == nil {
			delete(messages[1], "chat_id")
		}

		got := compactMessageListData(messages, "", "omt_thread", false, "")
		if _, exists := got["chat_id"]; exists {
			t.Fatalf("partially present chat_id was hoisted for %#v: %#v", missingValue, got)
		}
		projected := got["messages"].([]map[string]interface{})
		if projected[0]["chat_id"] != "oc_a" {
			t.Fatalf("known chat_id was lost for %#v: %#v", missingValue, projected)
		}
	}
}

func TestCompactMessageListDataPreservesMismatchedContextAndReplyValues(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"message_id": "om_root", "chat_id": "oc_expected", "thread_id": "omt_expected",
			"thread_replies": []interface{}{
				map[string]interface{}{"message_id": "om_reply", "chat_id": "oc_other", "thread_id": "omt_other"},
				"opaque-reply-value",
			},
		},
	}

	got := compactMessageListData(messages, "oc_expected", "omt_expected", false, "")
	root := got["messages"].([]map[string]interface{})[0]
	if _, exists := root["chat_id"]; exists {
		t.Fatalf("root retained matching chat_id: %#v", root)
	}
	if _, exists := root["thread_id"]; exists {
		t.Fatalf("root retained matching thread_id: %#v", root)
	}
	replies := root["thread_replies"].([]interface{})
	reply := replies[0].(map[string]interface{})
	if reply["chat_id"] != "oc_other" || reply["thread_id"] != "omt_other" {
		t.Fatalf("mismatched reply context was lost: %#v", reply)
	}
	if replies[1] != "opaque-reply-value" {
		t.Fatalf("non-message reply value was lost: %#v", replies)
	}
}

func TestCompactMessageListDataEmptyMessages(t *testing.T) {
	got := compactMessageListData(nil, "oc_chat", "", false, "")
	if got["chat_id"] != "oc_chat" || got["total"] != 0 {
		t.Fatalf("empty normalized output = %#v", got)
	}
	if messages := got["messages"].([]map[string]interface{}); len(messages) != 0 {
		t.Fatalf("empty normalized messages = %#v", messages)
	}
	if _, exists := got["participants"]; exists {
		t.Fatalf("empty output gained participants: %#v", got)
	}
}

func TestCompactMessageListDataDoesNotMutateInput(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"message_id": "om_root", "chat_id": "oc_chat",
			"sender": map[string]interface{}{
				"id": "ou_alice", "name": "Alice",
				"sender_i18n_names": map[string]interface{}{"en_us": "Alice"},
			},
			"thread_replies": []interface{}{map[string]interface{}{
				"message_id": "om_reply", "chat_id": "oc_chat",
				"sender": map[string]interface{}{"id": "ou_alice", "name": "Alice", "sender_i18n_names": map[string]interface{}{"en_us": "Alice"}},
			}},
		},
	}
	before, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}

	_ = compactMessageListData(messages, "oc_chat", "", false, "")

	after, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("projection mutated input\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestMessageListOutputDataRequiresExplicitNormalizedJSON(t *testing.T) {
	messages := []map[string]interface{}{{
		"message_id": "om_1", "chat_id": "oc_chat",
		"sender": map[string]interface{}{"id": "ou_alice", "name": "Alice"},
	}}

	for _, tc := range []struct {
		name, shape, format, jq string
		wantCompact             bool
	}{
		{name: "omitted shape keeps default JSON legacy", format: "json"},
		{name: "explicit legacy keeps JSON legacy", shape: "legacy", format: "json"},
		{name: "legacy jq filters legacy envelope", shape: "legacy", format: "table", jq: ".data"},
		{name: "legacy unknown format fallback stays legacy", shape: "legacy", format: "other"},
		{name: "normalized JSON", shape: "normalized", format: "json", wantCompact: true},
		{name: "case-insensitive normalized JSON", shape: "normalized", format: "JSON", wantCompact: true},
		{name: "normalized jq filters normalized envelope", shape: "normalized", format: "table", jq: ".data", wantCompact: true},
		{name: "normalized unknown format uses JSON fallback", shape: "normalized", format: "other", wantCompact: true},
		{name: "normalized uppercase pretty uses JSON fallback", shape: "normalized", format: "Pretty", wantCompact: true},
		{name: "normalized pretty keeps legacy projection", shape: "normalized", format: "pretty"},
		{name: "normalized table keeps legacy projection", shape: "normalized", format: "table"},
		{name: "normalized csv keeps legacy projection", shape: "normalized", format: "csv"},
		{name: "normalized ndjson keeps legacy projection", shape: "normalized", format: "ndjson"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := messageListOutputData(tc.shape, tc.format, tc.jq, messages, "oc_chat", "", false, "")
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

func TestMessageListJSONShapeFlagContract(t *testing.T) {
	for _, shortcut := range []struct {
		name  string
		flags []common.Flag
	}{
		{name: "chat messages", flags: ImChatMessageList.Flags},
		{name: "thread messages", flags: ImThreadsMessagesList.Flags},
	} {
		t.Run(shortcut.name, func(t *testing.T) {
			var shape *common.Flag
			for index := range shortcut.flags {
				if shortcut.flags[index].Name == "json-shape" {
					shape = &shortcut.flags[index]
					break
				}
			}
			if shape == nil {
				t.Fatal("missing --json-shape flag")
			}
			if shape.Default != "legacy" || !reflect.DeepEqual(shape.Enum, []string{"legacy", "normalized"}) {
				t.Fatalf("--json-shape contract = %#v", shape)
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

func TestChatMessagesListPreservesLegacyJSONByDefault(t *testing.T) {
	testMessageListCommandJSONShape(t, "chat-messages-list", nil, false)
}

func TestChatMessagesListEmitsNormalizedJSONWhenRequested(t *testing.T) {
	testMessageListCommandJSONShape(t, "chat-messages-list", map[string]string{"json-shape": "normalized"}, true)
}

func TestThreadsMessagesListPreservesLegacyJSONByDefault(t *testing.T) {
	testMessageListCommandJSONShape(t, "threads-messages-list", nil, false)
}

func TestThreadsMessagesListEmitsNormalizedJSONWhenRequested(t *testing.T) {
	testMessageListCommandJSONShape(t, "threads-messages-list", map[string]string{"json-shape": "normalized"}, true)
}

func TestChatMessagesListJQFiltersSelectedJSONShape(t *testing.T) {
	var tc listPageAllCase
	for _, candidate := range listPageAllCases() {
		if candidate.name == "chat-messages-list" {
			tc = candidate
			break
		}
	}
	for _, test := range []struct {
		name       string
		flags      map[string]string
		normalized bool
	}{
		{name: "legacy default"},
		{name: "normalized opt-in", flags: map[string]string{"json-shape": "normalized"}, normalized: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, _ := newListPageAllRuntime(t, tc, test.flags, func(_ *http.Request, _ int) map[string]interface{} {
				return map[string]interface{}{
					"items": []interface{}{map[string]interface{}{
						"message_id": "om_1", "chat_id": "oc_test", "msg_type": "text",
						"sender": map[string]interface{}{"id": "ou_alice", "sender_type": "user", "sender_name": "Alice"},
						"body":   map[string]interface{}{"content": `{"text":"hello"}`}, "create_time": "0",
					}},
					"has_more": false, "page_token": "",
				}
			})
			runtime.Format = "table"
			runtime.JqExpr = ".data.messages[0]"

			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			var message map[string]interface{}
			if err := json.Unmarshal(runtime.IO().Out.(*bytes.Buffer).Bytes(), &message); err != nil {
				t.Fatalf("jq output is not a JSON object: %v", err)
			}
			_, hasSenderID := message["sender_id"]
			if hasSenderID != test.normalized {
				t.Fatalf("sender_id present = %t, want %t: %#v", hasSenderID, test.normalized, message)
			}
			if test.normalized {
				if _, exists := message["sender"]; exists {
					t.Fatalf("normalized jq output retained sender: %#v", message)
				}
			} else if message["sender"] == nil || message["chat_id"] != "oc_test" {
				t.Fatalf("legacy jq output lost inline context: %#v", message)
			}
		})
	}
}

func testMessageListCommandJSONShape(t *testing.T, caseName string, flags map[string]string, normalized bool) {
	t.Helper()
	var tc listPageAllCase
	for _, candidate := range listPageAllCases() {
		if candidate.name == caseName {
			tc = candidate
			break
		}
	}
	containerID := "oc_test"
	contextKey := "chat_id"
	senderID := "ou_alice"
	senderName := "Alice"
	content := "full content"
	if caseName == "threads-messages-list" {
		containerID = "omt_test"
		contextKey = "thread_id"
		senderID = "ou_bob"
		senderName = "Bob"
		content = "reply"
	}
	runtime, calls := newListPageAllRuntime(t, tc, flags, func(req *http.Request, _ int) map[string]interface{} {
		if got := req.URL.Query().Get("container_id"); got != containerID {
			t.Fatalf("container_id = %q, want %s", got, containerID)
		}
		return map[string]interface{}{
			"items": []interface{}{map[string]interface{}{
				"message_id": "om_1", "msg_type": "text", contextKey: containerID,
				"sender": map[string]interface{}{"id": senderID, "sender_type": "user", "sender_name": senderName},
				"body":   map[string]interface{}{"content": fmt.Sprintf(`{"text":%q}`, content)}, "create_time": "0",
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
	if data["page_token"] != "final" {
		t.Fatalf("page_token = %#v, want final", data["page_token"])
	}
	message := data["messages"].([]interface{})[0].(map[string]interface{})
	if message["content"] != content {
		t.Fatalf("message = %#v", message)
	}
	if normalized {
		if data[contextKey] != containerID {
			t.Fatalf("top-level %s = %#v, want %s", contextKey, data[contextKey], containerID)
		}
		participants := data["participants"].(map[string]interface{})
		participant := participants[senderID].(map[string]interface{})
		if participant["name"] != senderName || participant["sender_type"] != "user" {
			t.Fatalf("participant = %#v", participant)
		}
		if message["sender_id"] != senderID {
			t.Fatalf("sender_id = %#v, want %s", message["sender_id"], senderID)
		}
		if _, exists := message["sender"]; exists {
			t.Fatalf("normalized message retained sender: %#v", message)
		}
		if _, exists := message[contextKey]; exists {
			t.Fatalf("normalized message retained %s: %#v", contextKey, message)
		}
		return
	}
	if caseName == "chat-messages-list" {
		if _, exists := data[contextKey]; exists {
			t.Fatalf("legacy chat output gained top-level %s: %#v", contextKey, data)
		}
	} else if data[contextKey] != containerID {
		t.Fatalf("legacy thread output lost top-level %s: %#v", contextKey, data)
	}
	if message[contextKey] != containerID || message["sender"] == nil {
		t.Fatalf("legacy message lost inline context: %#v", message)
	}
	if _, exists := message["sender_id"]; exists {
		t.Fatalf("legacy message gained sender_id: %#v", message)
	}
	if _, exists := data["participants"]; exists {
		t.Fatalf("legacy output gained participants: %#v", data)
	}
}
