// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/core"
)

const testMessageAppLink = "https://applink.feishu.cn/client/message/link/open?token=abc"

func textMessage() map[string]interface{} {
	return map[string]interface{}{
		"message_id":       "om_1",
		"msg_type":         "text",
		"content":          "hello world",
		"create_time":      "2026-07-27 21:26",
		"chat_id":          "oc_1",
		"message_app_link": testMessageAppLink,
	}
}

func TestMessageCitationPrefersMessageAppLink(t *testing.T) {
	c, ok := messageCitation(core.BrandFeishu, textMessage(), "")
	if !ok {
		t.Fatal("text message must be citable")
	}
	if c.SourceType != citation.SourceMessage {
		t.Errorf("source_type = %d", c.SourceType)
	}
	if c.URL != testMessageAppLink {
		t.Errorf("url = %q, want the message-level applink", c.URL)
	}
	if c.Title != "hello world" {
		t.Errorf("title = %q, want the message text verbatim", c.Title)
	}
	if c.Snippet != "" {
		t.Errorf("snippet = %q, want empty for messages", c.Snippet)
	}
	if c.PublishTime != citation.Time("2026-07-27 21:26") {
		t.Errorf("publish_time = %q", c.PublishTime)
	}
}

func TestMessageCitationFallsBackToChatLink(t *testing.T) {
	msg := textMessage()
	delete(msg, "message_app_link")
	c, ok := messageCitation(core.BrandFeishu, msg, "")
	if !ok {
		t.Fatal("text message must be citable")
	}
	if c.URL != "https://applink.feishu.cn/client/chat/open?openChatId=oc_1" {
		t.Errorf("url = %q, want the chat-level fallback", c.URL)
	}

	msg["message_app_link"] = "   "
	if c, _ := messageCitation(core.BrandFeishu, msg, ""); !strings.Contains(c.URL, "/client/chat/open") {
		t.Errorf("blank applink must fall back, got %q", c.URL)
	}
}

func TestMessageCitationUsesFallbackChatID(t *testing.T) {
	msg := textMessage()
	delete(msg, "chat_id")
	delete(msg, "message_app_link")
	c, _ := messageCitation(core.BrandFeishu, msg, "oc_outer")
	if c.URL != "https://applink.feishu.cn/client/chat/open?openChatId=oc_outer" {
		t.Errorf("url = %q, want the fallback chat id", c.URL)
	}
}

func TestMessageCitationWithoutMessageID(t *testing.T) {
	msg := textMessage()
	delete(msg, "message_id")
	c, ok := messageCitation(core.BrandFeishu, msg, "")
	if !ok {
		t.Fatal("a missing message_id must not make the message uncitable")
	}
	if c.URL == "" {
		t.Error("URL must still be present")
	}
}

func TestMessageCitationSkipsNonTextMessages(t *testing.T) {
	for _, msgType := range []string{"image", "file", "post", "merge_forward", "audio", ""} {
		msg := textMessage()
		msg["msg_type"] = msgType
		if _, ok := messageCitation(core.BrandFeishu, msg, ""); ok {
			t.Errorf("msg_type %q must not be citable this round", msgType)
		}
	}
}

func TestMessageCitationTitleIsNotTruncated(t *testing.T) {
	// The protocol defines a message's title as the message itself: a long
	// body is carried whole, with no truncation and no ellipsis.
	long := strings.Repeat("字", 600)
	msg := textMessage()
	msg["content"] = long
	c, _ := messageCitation(core.BrandFeishu, msg, "")
	if c.Title != long {
		t.Errorf("title = %d runes, want %d", len([]rune(c.Title)), len([]rune(long)))
	}
	if strings.Contains(c.Title, "…") {
		t.Error("title must not be truncated")
	}
}

func TestChatCitation(t *testing.T) {
	c := chatCitation(core.BrandFeishu, map[string]interface{}{
		"chat_id":     "oc_2",
		"name":        "CLI 引用需求群",
		"description": "对齐 citation 协议",
		"create_time": "1753622760",
	})
	if c.SourceType != citation.SourceMessage {
		t.Errorf("source_type = %d", c.SourceType)
	}
	if c.URL != "https://applink.feishu.cn/client/chat/open?openChatId=oc_2" {
		t.Errorf("url = %q", c.URL)
	}
	if c.Title != "CLI 引用需求群" {
		t.Errorf("title = %q, want the group name verbatim", c.Title)
	}
	if c.Snippet != "对齐 citation 协议" {
		t.Errorf("snippet = %q, want the group description", c.Snippet)
	}
	if c.PublishTime != citation.Time("1753622760") {
		t.Errorf("publish_time = %q", c.PublishTime)
	}
}

func TestChatCitationLongNameIsNotTruncated(t *testing.T) {
	long := strings.Repeat("群", 300)
	c := chatCitation(core.BrandFeishu, map[string]interface{}{"chat_id": "oc_3", "name": long})
	if c.Title != long {
		t.Errorf("title = %d runes, want %d", len([]rune(c.Title)), len([]rune(long)))
	}
}

func TestChatCitationWithoutChatIDHasNoURL(t *testing.T) {
	// Normalize drops URL-less entries; the builder must not invent one.
	if c := chatCitation(core.BrandFeishu, map[string]interface{}{"name": "x"}); c.URL != "" {
		t.Errorf("url = %q, want empty", c.URL)
	}
}

func TestChatMessagesListCitationsBuilder(t *testing.T) {
	data := map[string]interface{}{
		"chat_id": "oc_outer",
		"messages": []map[string]interface{}{
			textMessage(),
			{"message_id": "om_2", "msg_type": "image", "content": "[image]", "create_time": ""},
			{"message_id": "om_3", "msg_type": "text", "content": "no chat id", "create_time": ""},
		},
	}
	got := chatMessagesListCitations(nil, data)
	if len(got) != 2 {
		t.Fatalf("builder = %#v, want 2 entries (image skipped)", got)
	}
	if got[0].URL != testMessageAppLink {
		t.Errorf("first url = %q", got[0].URL)
	}
	if got[1].Title != "no chat id" {
		t.Errorf("second title = %q", got[1].Title)
	}
}

func TestMessagesSearchCitationsBuilder(t *testing.T) {
	data := map[string]interface{}{
		"messages": []interface{}{
			textMessage(),
			map[string]interface{}{"message_id": "om_9", "msg_type": "file", "content": "x"},
		},
	}
	got := messagesSearchCitations(nil, data)
	if len(got) != 1 {
		t.Fatalf("builder = %#v, want 1 entry (file skipped)", got)
	}
	if got[0].Title != "hello world" {
		t.Errorf("title = %q, want the message text", got[0].Title)
	}
}

func TestChatSearchCitationsBuilder(t *testing.T) {
	data := map[string]interface{}{
		"chats": []map[string]interface{}{
			{"chat_id": "oc_a", "name": "A"},
			{"chat_id": "oc_b", "name": "B"},
		},
	}
	got := chatSearchCitations(nil, data)
	if len(got) != 2 {
		t.Fatalf("builder = %#v, want 2 entries", got)
	}
	if got[1].Title != "B" {
		t.Errorf("title = %q", got[1].Title)
	}
}

func TestCitationBuildersTolerateBadShapes(t *testing.T) {
	if chatMessagesListCitations(nil, "not a map") != nil {
		t.Error("non-map data must yield nil")
	}
	if chatMessagesListCitations(nil, map[string]interface{}{"messages": "wrong"}) != nil {
		t.Error("wrong messages shape must yield nil")
	}
	if messagesSearchCitations(nil, map[string]interface{}{"message_ids": []string{"om_1"}}) != nil {
		t.Error("mget-fallback payload must yield nil")
	}
	if chatSearchCitations(nil, map[string]interface{}{"chats": 42}) != nil {
		t.Error("wrong chats shape must yield nil")
	}
}
