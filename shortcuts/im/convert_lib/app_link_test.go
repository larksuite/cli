// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	testurlrewrite "github.com/larksuite/cli/internal/testutil/urlrewrite"
)

func TestMessageLinkDataContainerLinks(t *testing.T) {
	for _, tt := range []struct {
		brand core.LarkBrand
		host  string
	}{
		{core.BrandFeishu, "applink.feishu.cn"},
		{core.BrandLark, "applink.larksuite.com"},
	} {
		t.Run(string(tt.brand), func(t *testing.T) {
			message := MessageLinkData{ChatID: "oc_a&b", ThreadID: "omt_a+b"}
			links := message.AppLinks(tt.brand)
			if links.MessageAppLink != "" {
				t.Fatalf("positionless thread has a message link: %s", links.MessageAppLink)
			}
			chat, err := url.Parse(links.ChatAppLink)
			if err != nil || chat.Host != tt.host || chat.Path != "/client/chat/open" || chat.Query().Get("openChatId") != message.ChatID {
				t.Fatalf("chat link = %q, err = %v", links.ChatAppLink, err)
			}
			thread, err := url.Parse(links.ThreadAppLink)
			if err != nil || thread.Host != tt.host || thread.Path != "/client/thread/open" {
				t.Fatalf("thread link = %q, err = %v", links.ThreadAppLink, err)
			}
			for key, want := range map[string]string{
				"openchatid": message.ChatID, "open_chat_id": message.ChatID,
				"openthreadid": message.ThreadID, "open_thread_id": message.ThreadID,
			} {
				if got := thread.Query().Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if thread.Query().Has("thread_position") || len(thread.Query()) != 4 {
				t.Fatalf("unexpected thread query: %v", thread.Query())
			}
		})
	}
}

func TestMessageLinkDataAvailableAnchors(t *testing.T) {
	for _, tt := range []struct {
		name                string
		message             MessageLinkData
		path, key, position string
	}{
		{"thread position", MessageLinkData{ChatID: "oc_x", ThreadID: "omt_x", MessagePosition: "10", ThreadMessagePosition: "2"}, "/client/thread/open", "thread_position", "2"},
		{"chat fallback", MessageLinkData{ChatID: "oc_x", ThreadID: "omt_x", MessagePosition: "10"}, "/client/chat/open", "position", "10"},
		{"missing chat", MessageLinkData{ThreadID: "omt_x", ThreadMessagePosition: "2"}, "", "", ""},
		{"missing positions", MessageLinkData{ChatID: "oc_x", ThreadID: "omt_x"}, "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			links := tt.message.AppLinks(core.BrandFeishu)
			if tt.path == "" {
				if links.MessageAppLink != "" {
					t.Fatalf("unexpected message link: %s", links.MessageAppLink)
				}
				return
			}
			u, err := url.Parse(links.MessageAppLink)
			if err != nil || u.Path != tt.path || u.Query().Get(tt.key) != tt.position {
				t.Fatalf("message link = %q, err = %v", links.MessageAppLink, err)
			}
		})
	}
}

func TestMessageLinkDataRewriteAndServerPreference(t *testing.T) {
	testurlrewrite.Register(t, func(raw string) string {
		return strings.Replace(raw, "applink.feishu.cn", "links.example.test", 1)
	})
	message := MessageLinkData{ChatID: "oc_x", ThreadID: "omt_x", ThreadMessagePosition: "2"}
	links := message.AppLinks(core.BrandFeishu)
	for _, link := range []string{links.ChatAppLink, links.ThreadAppLink, links.MessageAppLink} {
		if !strings.HasPrefix(link, "https://links.example.test/") {
			t.Errorf("rewrite missing: %s", link)
		}
	}
	message.MessageAppLink = "https://server.example.test/exact-message"
	if got := message.AppLinks(core.BrandFeishu).MessageAppLink; got != message.MessageAppLink {
		t.Fatalf("server link replaced with %q", got)
	}
}
