// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailMessagesExecuteChunksMoreThanTwentyIDs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	ids := make([]string, 21)
	for i := range ids {
		ids[i] = base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("biz-%03d", i)))
	}

	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/user_mailboxes/me/messages/batch_get",
		BodyFilter: requestMessageIDsEqual(ids[:20]),
		Body:       batchGetMessagesResponse(ids[:20], true),
	})
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/user_mailboxes/me/messages/batch_get",
		BodyFilter: requestMessageIDsEqual(ids[20:]),
		Body:       batchGetMessagesResponse(ids[20:], false),
	})

	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--message-ids", strings.Join(ids, ","),
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := decodeShortcutEnvelopeData(t, stdout)
	if got := int(out["total"].(float64)); got != len(ids) {
		t.Fatalf("total = %d, want %d; stdout=%s", got, len(ids), stdout.String())
	}
	messages, ok := out["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages has unexpected type %T", out["messages"])
	}
	if len(messages) != len(ids) {
		t.Fatalf("messages length = %d, want %d", len(messages), len(ids))
	}
	for i, item := range messages {
		msg, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("messages[%d] has unexpected type %T", i, item)
		}
		if got := msg["message_id"]; got != ids[i] {
			t.Fatalf("messages[%d].message_id = %v, want %s", i, got, ids[i])
		}
		if i == 0 {
			if got := msg["reply_to_message_id"]; got != "parent-"+ids[i] {
				t.Fatalf("messages[%d].reply_to_message_id = %v, want %s", i, got, "parent-"+ids[i])
			}
		} else if _, ok := msg["reply_to_message_id"]; ok {
			t.Fatalf("messages[%d].reply_to_message_id should be omitted when absent", i)
		}
	}
}

func requestMessageIDsEqual(want []string) func([]byte) bool {
	return func(body []byte) bool {
		var payload struct {
			MessageIDs []string `json:"message_ids"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return false
		}
		return reflect.DeepEqual(payload.MessageIDs, want)
	}
}

func batchGetMessagesResponse(ids []string, includeReplyToMessageID bool) map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		message := map[string]interface{}{
			"message_id": id,
			"subject":    id,
		}
		if includeReplyToMessageID && len(messages) == 0 {
			message["reply_to_message_id"] = "parent-" + id
		}
		messages = append(messages, message)
	}
	return map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"messages": messages,
		},
	}
}
