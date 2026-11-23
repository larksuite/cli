// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestAssembleChatAppLink(t *testing.T) {
	tests := []struct {
		name   string
		chatID interface{}
		brand  core.LarkBrand
		want   string
	}{
		{
			name:   "feishu open chat id",
			chatID: "oc_a0553eda9014c201e6969b478895c230",
			brand:  core.BrandFeishu,
			want:   "https://applink.feishu.cn/client/chat/open?openChatId=oc_a0553eda9014c201e6969b478895c230",
		},
		{
			name:   "lark open chat id",
			chatID: "oc_a0553eda9014c201e6969b478895c230",
			brand:  core.BrandLark,
			want:   "https://applink.larksuite.com/client/chat/open?openChatId=oc_a0553eda9014c201e6969b478895c230",
		},
		{
			name:   "numeric internal chat id omitted",
			chatID: "7670440925243608339",
			brand:  core.BrandFeishu,
		},
		{
			name:   "non string chat id omitted",
			chatID: 123,
			brand:  core.BrandFeishu,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assembleChatAppLink(tt.chatID, tt.brand); got != tt.want {
				t.Fatalf("assembleChatAppLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImChatListExecuteAddsChatAppLink(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"msg":"ok","data":{"items":[{"chat_id":"oc_g","name":"G","chat_mode":"group"}],"has_more":false,"page_token":""}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	attachChatListCmd(t, rt, map[string]string{"types": "group"}, nil)

	if err := ImChatList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	var envelope struct {
		Data struct {
			Chats []map[string]interface{} `json:"chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chatListOutBuf(t, rt).Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if len(envelope.Data.Chats) != 1 {
		t.Fatalf("chats length = %d, want 1", len(envelope.Data.Chats))
	}
	want := "https://applink.feishu.cn/client/chat/open?openChatId=oc_g"
	if got, _ := envelope.Data.Chats[0]["chat_app_link"].(string); got != want {
		t.Fatalf("chat_app_link = %q, want %q", got, want)
	}
}

func TestImChatSearchExecuteOmitsChatAppLink(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"msg":"ok","data":{"items":[{"meta_data":{"chat_id":"oc_visible","name":"Visible","chat_mode":"group"}}],"has_more":false,"page_token":""}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	rt.Cmd = newChatSearchNoticeTestCommand(t, "Joined")
	rt.Format = "json"

	if err := ImChatSearch.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	var envelope struct {
		Data struct {
			Chats []map[string]interface{} `json:"chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chatListOutBuf(t, rt).Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if _, ok := envelope.Data.Chats[0]["chat_app_link"]; ok {
		t.Fatalf("chat_app_link must be omitted for search results: %#v", envelope.Data.Chats[0])
	}
}

func TestImChatSearchExecuteOmitsChatAppLinkForNonMemberResults(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"msg":"ok","data":{"items":[{"meta_data":{"chat_id":"oc_not_joined","name":"Not Joined","chat_mode":"group"}}],"has_more":false,"page_token":""}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	rt.Cmd = newChatSearchNoticeTestCommand(t, "Not Joined")
	if err := rt.Cmd.Flags().Set("search-types", "public_not_joined"); err != nil {
		t.Fatalf("Flags().Set(search-types) error = %v", err)
	}
	rt.Format = "json"

	if err := ImChatSearch.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	var envelope struct {
		Data struct {
			Chats []map[string]interface{} `json:"chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chatListOutBuf(t, rt).Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if _, ok := envelope.Data.Chats[0]["chat_app_link"]; ok {
		t.Fatalf("chat_app_link must be omitted for public_not_joined search results: %#v", envelope.Data.Chats[0])
	}
}
