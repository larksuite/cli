// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestRenderMessagesConciseChatConversation(t *testing.T) {
	longContent := strings.Repeat("full message content ", 8)
	messages := []map[string]interface{}{
		{
			"message_id":       "om_root",
			"reply_to":         "om_parent",
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
			"resources": []map[string]interface{}{
				{"local_path": "lark-im-resources/root-image.png", "key": "must-not-render"},
				{"key": "download-failed"},
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
					"resources": []interface{}{
						map[string]interface{}{"local_path": "lark-im-resources/reply-file.pdf"},
					},
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
		"reply_to: `om_parent`",
		"thread_id: `omt_thread`",
		"**Reply**",
		"message_id: `om_reply`",
		"> reply line one",
		"> reply line two",
		"reactions: `THUMBSUP x2`",
		"resources: `lark-im-resources/root-image.png`",
		"resources: `lark-im-resources/reply-file.pdf`",
		"thread_has_more: true (thread replies incomplete)",
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
	for _, forbidden := range []string{
		"duplicate root",
		"reaction-detail-is-omitted",
		"must-not-render",
		"download-failed",
		"app_link:",
		"https://applink.feishu.cn/client/thread/open?open_thread_id=omt_thread",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("concise output contains %q:\n%s", forbidden, got)
		}
	}
	if count := strings.Count(got, "reply_to:"); count != 1 {
		t.Fatalf("concise output reply_to count = %d, want 1:\n%s", count, got)
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
	for _, want := range []string{"[deleted]", "reactions: unavailable", "thread_replies_error: true (thread replies unavailable)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("concise output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "must not be rendered") {
		t.Fatalf("deleted message leaked original content:\n%s", got)
	}
}

func TestRenderMessagesConciseOmitsTokenWhenPaginationIsComplete(t *testing.T) {
	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Title:     "Messages",
		HasMore:   false,
		NextToken: "final-server-token",
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "- has_more: false") {
		t.Fatalf("concise output missing completed pagination state:\n%s", got)
	}
	if strings.Contains(got, "next_token:") || strings.Contains(got, "final-server-token") {
		t.Fatalf("concise output exposed a non-resumable final token:\n%s", got)
	}
}

func TestRenderMessagesConciseEscapesUntrustedMetadata(t *testing.T) {
	const forged = "\n## forged"
	messages := []map[string]interface{}{
		{
			"message_id":       "om_bad`" + forged,
			"reply_to":         "om_parent`" + forged,
			"thread_id":        "omt_bad`" + forged,
			"msg_type":         "text`" + forged,
			"create_time":      "2026-08-26`" + forged,
			"content":          "normal body",
			"message_app_link": "https://safe.example>" + forged,
			"resources": []map[string]interface{}{
				{"local_path": "lark-im-resources/file`" + forged},
			},
			"sender": map[string]interface{}{
				"id":          "ou_bad`" + forged,
				"name":        "\x1b[31m**Mallory**" + forged + "\u202e",
				"sender_type": "user" + forged,
				"open_bot_id": "ou_bot`" + forged,
			},
			"reactions": map[string]interface{}{
				"counts": []interface{}{
					map[string]interface{}{"reaction_type": "SMILE`" + forged, "count": float64(1)},
				},
			},
		},
	}

	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Title:     "Messages" + forged,
		ChatID:    "oc_bad`" + forged,
		Messages:  messages,
		HasMore:   true,
		NextToken: "next`" + forged,
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, forged) {
		t.Fatalf("untrusted metadata injected a Markdown heading:\n%s", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\u202e") {
		t.Fatalf("untrusted metadata retained terminal control characters: %q", got)
	}
	for _, want := range []string{
		"# Messages \\#\\# forged",
		"chat_id: ``oc_bad` ## forged``",
		"reply_to: ``om_parent` ## forged``",
		"next_token: ``next` ## forged``",
		"resources: ``lark-im-resources/file` ## forged``",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("escaped output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "safe.example") || strings.Contains(got, "app_link:") {
		t.Fatalf("concise output included message_app_link:\n%s", got)
	}
}

func TestMessageListConciseFormatIsCommandScoped(t *testing.T) {
	for _, shortcut := range []*common.Shortcut{&ImChatMessageList, &ImThreadsMessagesList} {
		runtime, _ := newMountedIMRuntime(t, shortcut)
		flag := runtime.Cmd.Flags().Lookup("format")
		if flag == nil || !strings.Contains(flag.Usage, "concise") {
			t.Fatalf("%s --format usage = %#v, want concise", shortcut.Command, flag)
		}
		if findDeclaredFormatFlag(*shortcut) != nil {
			t.Fatalf("%s must use the framework-owned --format flag", shortcut.Command)
		}
		if runtime.Cmd.Flags().Lookup("json") == nil {
			t.Fatalf("%s lost --json shorthand", shortcut.Command)
		}
	}

	runtime, _ := newMountedIMRuntime(t, &ImChatList)
	flag := runtime.Cmd.Flags().Lookup("format")
	if flag == nil || strings.Contains(flag.Usage, "concise") {
		t.Fatalf("%s unexpectedly advertises concise: %#v", ImChatList.Command, flag)
	}
}

func findDeclaredFormatFlag(shortcut common.Shortcut) *common.Flag {
	for i := range shortcut.Flags {
		flag := &shortcut.Flags[i]
		if flag.Name == "format" {
			return flag
		}
	}
	return nil
}
