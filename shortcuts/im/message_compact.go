// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"reflect"

	"github.com/larksuite/cli/internal/output"
)

func messageListOutputData(
	jsonShape string,
	runtimeFormat string,
	jqExpr string,
	messages []map[string]interface{},
	chatID string,
	threadID string,
	hasMore bool,
	pageToken string,
) map[string]interface{} {
	legacy := map[string]interface{}{
		"messages":   messages,
		"total":      len(messages),
		"has_more":   hasMore,
		"page_token": pageToken,
	}
	if threadID != "" {
		legacy["thread_id"] = threadID
	}

	// Preserve the established message envelope unless the caller explicitly
	// opts into normalization. JQ filters whichever envelope the caller chose.
	if jsonShape != messageListJSONShapeNormalized {
		return legacy
	}

	// JQ always filters the JSON envelope. Unknown formats also fall back to
	// JSON in Emitter.Success. Record-oriented and human formats keep their
	// established per-message shape even when the JSON-only option is present.
	if jqExpr != "" {
		return compactMessageListData(messages, chatID, threadID, hasMore, pageToken)
	}
	if runtimeFormat == "pretty" {
		return legacy
	}
	format, known := output.ParseFormat(runtimeFormat)
	if !known || format == output.FormatJSON {
		return compactMessageListData(messages, chatID, threadID, hasMore, pageToken)
	}
	return legacy
}

const (
	messageListJSONShapeLegacy     = "legacy"
	messageListJSONShapeNormalized = "normalized"
)

// compactMessageListData normalizes repeated conversation metadata for JSON
// output. It never mutates messages: the enriched message tree remains the
// source for human and record-oriented renderers.
func compactMessageListData(
	messages []map[string]interface{},
	chatID string,
	threadID string,
	hasMore bool,
	pageToken string,
) map[string]interface{} {
	if chatID == "" {
		chatID = commonMessageString(messages, "chat_id")
	}

	participants, reusableSenders := compactParticipants(messages)
	projected := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		projected = append(projected, compactMessage(message, chatID, threadID, reusableSenders))
	}

	out := map[string]interface{}{
		"messages":   projected,
		"total":      len(projected),
		"has_more":   hasMore,
		"page_token": pageToken,
	}
	if chatID != "" {
		out["chat_id"] = chatID
	}
	if threadID != "" {
		out["thread_id"] = threadID
	}
	if len(participants) > 0 {
		out["participants"] = participants
	}
	return out
}

// compactParticipants returns sender records that can be referenced without
// losing information. A sender id is reusable only when every occurrence has
// identical metadata; conflicting occurrences stay inline in compactMessage.
func compactParticipants(messages []map[string]interface{}) (map[string]map[string]interface{}, map[string]bool) {
	originalByID := make(map[string]map[string]interface{})
	participants := make(map[string]map[string]interface{})
	reusable := make(map[string]bool)

	walkMessageTree(messages, func(message map[string]interface{}) {
		sender, ok := message["sender"].(map[string]interface{})
		if !ok {
			return
		}
		id, _ := sender["id"].(string)
		if id == "" {
			return
		}
		if existing, seen := originalByID[id]; seen {
			if !reflect.DeepEqual(existing, sender) {
				reusable[id] = false
			}
			return
		}
		originalByID[id] = cloneStringMap(sender)
		participant := cloneStringMap(sender)
		delete(participant, "id")
		participants[id] = participant
		reusable[id] = true
	})

	for id := range participants {
		if !reusable[id] {
			delete(participants, id)
		}
	}
	return participants, reusable
}

func compactMessage(message map[string]interface{}, chatID string, threadID string, reusableSenders map[string]bool) map[string]interface{} {
	out := cloneStringMap(message)
	if messageChatID, _ := out["chat_id"].(string); chatID != "" && messageChatID == chatID {
		delete(out, "chat_id")
	}
	if messageThreadID, _ := out["thread_id"].(string); threadID != "" && messageThreadID == threadID {
		delete(out, "thread_id")
	}
	if sender, ok := out["sender"].(map[string]interface{}); ok {
		if id, _ := sender["id"].(string); id != "" && reusableSenders[id] {
			delete(out, "sender")
			out["sender_id"] = id
		}
	}
	if replies, ok := compactThreadReplies(out["thread_replies"], chatID, threadID, reusableSenders); ok {
		out["thread_replies"] = replies
	}
	return out
}

func compactThreadReplies(value interface{}, chatID string, threadID string, reusableSenders map[string]bool) (interface{}, bool) {
	switch replies := value.(type) {
	case []map[string]interface{}:
		projected := make([]map[string]interface{}, 0, len(replies))
		for _, reply := range replies {
			projected = append(projected, compactMessage(reply, chatID, threadID, reusableSenders))
		}
		return projected, true
	case []interface{}:
		projected := make([]interface{}, 0, len(replies))
		for _, reply := range replies {
			if message, ok := reply.(map[string]interface{}); ok {
				projected = append(projected, compactMessage(message, chatID, threadID, reusableSenders))
				continue
			}
			projected = append(projected, reply)
		}
		return projected, true
	default:
		return nil, false
	}
}

func commonMessageString(messages []map[string]interface{}, key string) string {
	common := ""
	conflict := false
	walkMessageTree(messages, func(message map[string]interface{}) {
		if conflict {
			return
		}
		value, ok := message[key].(string)
		if !ok || value == "" {
			common = ""
			conflict = true
			return
		}
		if common == "" {
			common = value
			return
		}
		if common != value {
			common = ""
			conflict = true
		}
	})
	return common
}

func walkMessageTree(messages []map[string]interface{}, visit func(map[string]interface{})) {
	for _, message := range messages {
		visit(message)
		walkMessageTree(messageSlice(message["thread_replies"]), visit)
	}
}

func messageSlice(value interface{}) []map[string]interface{} {
	switch items := value.(type) {
	case []map[string]interface{}:
		return items
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			if message, ok := item.(map[string]interface{}); ok {
				out = append(out, message)
			}
		}
		return out
	default:
		return nil
	}
}

func cloneStringMap(source map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
