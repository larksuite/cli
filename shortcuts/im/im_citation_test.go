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
		"chat_name":        "Example group",
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
	if c.Title != "Example group" {
		t.Errorf("title = %q, want the conversation name", c.Title)
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

func TestMessageCitationIncludesAllMessageTypes(t *testing.T) {
	for _, msgType := range []string{"text", "image", "file", "post", "merge_forward", "audio", "media", "video", "sticker", "interactive", "system", "share_chat", "share_user", "future_type", ""} {
		msg := textMessage()
		msg["msg_type"] = msgType
		if c, ok := messageCitation(core.BrandFeishu, msg, ""); !ok || c.Title != "Example group" {
			t.Errorf("msg_type %q citation = %#v, %v", msgType, c, ok)
		}
	}
}

func TestMessageCitationTitleIsNotTruncated(t *testing.T) {
	// Conversation names are not truncated or replaced by message bodies.
	long := strings.Repeat("字", 600)
	msg := textMessage()
	msg["chat_name"] = long
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
		"chat_id":   "oc_outer",
		"chat_name": "Outer group",
		"messages": []map[string]interface{}{
			textMessage(),
			{"message_id": "om_2", "msg_type": "image", "content": "[image]", "create_time": ""},
			{"message_id": "om_3", "msg_type": "text", "content": "no chat id", "create_time": ""},
		},
	}
	got := chatMessagesListCitations(nil, data)
	if len(got) != 3 {
		t.Fatalf("builder = %#v, want all 3 entries", got)
	}
	if got[0].URL != testMessageAppLink {
		t.Errorf("first url = %q", got[0].URL)
	}
	if got[1].Title != "Outer group" {
		t.Errorf("second title = %q", got[1].Title)
	}
}

func TestChatMessagesListCitationsIncludesAllThreadReplies(t *testing.T) {
	parent := textMessage()
	parent["thread_replies"] = []map[string]interface{}{
		{
			"message_id":       "om_reply_1",
			"msg_type":         "text",
			"content":          "first reply",
			"create_time":      "2026-08-31 19:31",
			"message_app_link": "https://applink.feishu.cn/client/thread/open?thread_position=0",
		},
		{"message_id": "om_reply_image", "msg_type": "image", "content": "[image]"},
	}
	nonTextParent := map[string]interface{}{
		"message_id": "om_parent_image",
		"msg_type":   "image",
		"content":    "[image]",
		"thread_replies": []interface{}{
			map[string]interface{}{
				"message_id":       "om_reply_2",
				"msg_type":         "text",
				"content":          "second reply",
				"create_time":      "2026-08-31 19:32",
				"message_app_link": "https://applink.feishu.cn/client/thread/open?thread_position=1",
			},
		},
	}

	got := chatMessagesListCitations(nil, map[string]interface{}{
		"chat_id":   "oc_outer",
		"chat_name": "Outer group",
		"messages":  []map[string]interface{}{parent, nonTextParent},
	})
	if len(got) != 5 {
		t.Fatalf("builder = %#v, want both parents and all 3 replies", got)
	}
	for _, c := range got {
		if c.Title != "Outer group" {
			t.Errorf("citation = %#v, want owning conversation title", c)
		}
	}
	if got[1].URL != "https://applink.feishu.cn/client/thread/open?thread_position=0" {
		t.Errorf("first reply url = %q", got[1].URL)
	}
	if got[1].PublishTime != citation.Time("2026-08-31 19:31") {
		t.Errorf("first reply publish_time = %q", got[1].PublishTime)
	}
	if got[4].URL != "https://applink.feishu.cn/client/thread/open?thread_position=1" {
		t.Errorf("second reply citation = %#v", got[4])
	}
}

func TestMessagesSearchCitationsBuilder(t *testing.T) {
	data := map[string]interface{}{
		"messages": []interface{}{
			textMessage(),
			map[string]interface{}{"message_id": "om_9", "msg_type": "file", "content": "x", "chat_id": "oc_other", "chat_name": "A和B的会话"},
		},
	}
	got := messagesSearchCitations(nil, data)
	if len(got) != 2 {
		t.Fatalf("builder = %#v, want both entries", got)
	}
	if got[0].Title != "Example group" || got[1].Title != "A和B的会话" {
		t.Errorf("titles = %#v", got)
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
