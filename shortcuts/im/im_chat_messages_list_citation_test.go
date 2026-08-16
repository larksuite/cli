// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/core"
)

func TestChatMessageCitation(t *testing.T) {
	c := chatMessageCitation(core.BrandFeishu, "oc_1", "om_1", "hello world", "2026-07-27 21:26")
	if c.SourceType != citation.SourceMessage {
		t.Errorf("source_type = %d", c.SourceType)
	}
	if c.URL != "https://applink.feishu.cn/client/chat/open?openChatId=oc_1" {
		t.Errorf("url = %q", c.URL)
	}
	if c.Title != "hello world" || c.Snippet != "hello world" {
		t.Errorf("title/snippet = %q %q", c.Title, c.Snippet)
	}
	if c.ResourceID != "oc_1/om_1" {
		t.Errorf("resource_id = %q", c.ResourceID)
	}
	if c.PublishTime != citation.Time("2026-07-27 21:26") {
		t.Errorf("publish_time = %q", c.PublishTime)
	}
}

func TestChatMessageCitationTitleTruncation(t *testing.T) {
	long := strings.Repeat("字", 60)
	c := chatMessageCitation(core.BrandFeishu, "oc_1", "om_1", long, "")
	wantTitle := strings.Repeat("字", 50) + "…"
	if c.Title != wantTitle {
		t.Errorf("title = %q (len %d), want 50 runes + ellipsis", c.Title, len([]rune(c.Title)))
	}
	if c.Snippet != long {
		t.Error("snippet must keep full text")
	}
}

func TestChatMessagesListCitationsBuilder(t *testing.T) {
	data := map[string]interface{}{
		"chat_id": "oc_1",
		"messages": []map[string]interface{}{
			{"message_id": "om_1", "content": "hello", "create_time": "2026-07-27 21:26"},
			{"message_id": "", "content": "no-id-still-builds", "create_time": ""},
		},
	}
	got := chatMessagesListCitations(nil, data)
	if len(got) != 2 {
		t.Fatalf("builder = %#v, want 2 entries", got)
	}
	if got[0].ResourceID != "oc_1/om_1" {
		t.Errorf("resource_id = %q", got[0].ResourceID)
	}
}

func TestChatMessagesListCitationsBuilderBadShape(t *testing.T) {
	if chatMessagesListCitations(nil, "not a map") != nil {
		t.Error("non-map data must yield nil")
	}
	if chatMessagesListCitations(nil, map[string]interface{}{"messages": "wrong"}) != nil {
		t.Error("wrong messages shape must yield nil")
	}
}
