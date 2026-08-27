// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestRenderMessagesConciseChatConversation(t *testing.T) {
	longContent := strings.Repeat("full message content ", 8)
	messages := []map[string]interface{}{
		{
			"message_id":       "om_root",
			"thread_id":        "omt_thread",
			"msg_type":         "text",
			"create_time":      "2026-08-26 06:17",
			"content":          longContent,
			"message_app_link": "https://applink.feishu.cn/client/thread/open?open_thread_id=omt_thread",
			"sender": map[string]interface{}{
				"id":          "ou_sender",
				"name":        "Alice",
				"sender_type": "user",
			},
			"reactions": map[string]interface{}{
				"counts": []interface{}{
					map[string]interface{}{"reaction_type": "THUMBSUP", "count": float64(2)},
				},
				"details": []interface{}{map[string]interface{}{"reaction_id": "reaction-detail-is-omitted"}},
			},
			"thread_replies": []map[string]interface{}{
				{
					"message_id": "om_root",
					"thread_id":  "omt_thread",
					"msg_type":   "text",
					"content":    "duplicate root",
				},
				{
					"message_id":  "om_reply",
					"thread_id":   "omt_thread",
					"msg_type":    "post",
					"create_time": "2026-08-26 06:18",
					"content":     "reply line one\nreply line two",
					"sender": map[string]interface{}{
						"id":          "cli_bot",
						"name":        "Helper Bot",
						"sender_type": "app",
						"open_bot_id": "ou_bot",
					},
				},
			},
			"thread_has_more": true,
		},
	}

	var out bytes.Buffer
	err := renderMessagesConcise(&out, conciseMessageView{
		Title:     "Chat messages",
		ChatID:    "oc_chat",
		Messages:  messages,
		HasMore:   true,
		NextToken: "next-page",
	})
	if err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"# Chat messages",
		"- chat_id: `oc_chat`",
		"- Alice (`ou_sender`, user)",
		"- Helper Bot (`cli_bot`, app, bot_open_id: `ou_bot`)",
		longContent,
		"message_id: `om_root`",
		"thread_id: `omt_thread`",
		"app_link: <https://applink.feishu.cn/client/thread/open?open_thread_id=omt_thread>",
		"**Reply**",
		"message_id: `om_reply`",
		"> reply line one",
		"> reply line two",
		"reactions: `THUMBSUP x2`",
		"thread_replies: incomplete",
		"- messages: 1",
		"- thread_replies: 1",
		"- threads: 1",
		"- has_more: true",
		"- next_token: `next-page`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("concise output missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"duplicate root", "reaction-detail-is-omitted"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("concise output contains %q:\n%s", forbidden, got)
		}
	}
}

func TestRenderMessagesConciseMarksUnavailableAndDeletedContent(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"message_id":           "om_deleted",
			"msg_type":             "text",
			"content":              "must not be rendered",
			"deleted":              true,
			"reactions_error":      true,
			"thread_replies_error": true,
		},
	}

	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Title:    "Messages",
		Messages: messages,
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"[deleted]", "reactions: unavailable", "thread_replies: unavailable"} {
		if !strings.Contains(got, want) {
			t.Fatalf("concise output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "must not be rendered") {
		t.Fatalf("deleted message leaked original content:\n%s", got)
	}
}

func TestMessageListConciseFormatIsCommandScoped(t *testing.T) {
	for _, shortcut := range []struct {
		name  string
		flags []string
	}{
		{name: ImChatMessageList.Command, flags: formatEnum(ImChatMessageList)},
		{name: ImThreadsMessagesList.Command, flags: formatEnum(ImThreadsMessagesList)},
	} {
		if !slices.Contains(shortcut.flags, "concise") {
			t.Fatalf("%s format enum = %v, want concise", shortcut.name, shortcut.flags)
		}
	}
	if formats := formatEnum(ImChatList); slices.Contains(formats, "concise") {
		t.Fatalf("%s unexpectedly declares concise: %v", ImChatList.Command, formats)
	}
}

func formatEnum(shortcut common.Shortcut) []string {
	for _, flag := range shortcut.Flags {
		if flag.Name == "format" {
			return flag.Enum
		}
	}
	return nil
}
