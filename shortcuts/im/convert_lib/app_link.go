// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/urlrewrite"
)

// MessageLinkData is the navigation metadata returned by the message APIs.
type MessageLinkData struct {
	MessageID             string      `json:"message_id"`
	ChatID                string      `json:"chat_id"`
	ThreadID              string      `json:"thread_id"`
	MessageAppLink        string      `json:"message_app_link"`
	MessagePosition       interface{} `json:"message_position"`
	ThreadMessagePosition interface{} `json:"thread_message_position"`
}

// MessageLinkDataFromMap projects the fields used for navigation before assembly.
func MessageLinkDataFromMap(message map[string]interface{}) MessageLinkData {
	messageID, _ := message["message_id"].(string)
	chatID, _ := message["chat_id"].(string)
	threadID, _ := message["thread_id"].(string)
	messageAppLink, _ := message["message_app_link"].(string)
	return MessageLinkData{
		MessageID: messageID, ChatID: chatID, ThreadID: threadID,
		MessageAppLink:        messageAppLink,
		MessagePosition:       message["message_position"],
		ThreadMessagePosition: message["thread_message_position"],
	}
}

// AppLinks distinguishes a container link from a link to a specific message.
type AppLinks struct {
	ChatAppLink    string `json:"chat_app_link,omitempty"`
	ThreadAppLink  string `json:"thread_app_link,omitempty"`
	MessageAppLink string `json:"message_app_link,omitempty"`
}

// AppLinks returns available links, preferring the server's message link.
// A thread container needs no position; a specific message still does.
func (message MessageLinkData) AppLinks(brand core.LarkBrand) AppLinks {
	links := AppLinks{
		ChatAppLink:    ChatAppLink(message.ChatID, brand),
		MessageAppLink: strings.TrimSpace(message.MessageAppLink),
	}
	if u := threadAppLinkURL(message.ChatID, message.ThreadID, brand); u != nil {
		links.ThreadAppLink = urlrewrite.Rewrite(u.String())
	}
	if links.MessageAppLink == "" {
		links.MessageAppLink = message.assembleMessageAppLink(brand)
	}
	return links
}

// ChatAppLink opens a conversation the caller has already joined.
func ChatAppLink(chatID string, brand core.LarkBrand) string {
	chatID = strings.TrimSpace(chatID)
	if !strings.HasPrefix(chatID, "oc_") {
		return ""
	}
	u := chatAppLinkURL(chatID, brand)
	if u == nil {
		return ""
	}
	return urlrewrite.Rewrite(u.String())
}

func chatAppLinkURL(chatID string, brand core.LarkBrand) *url.URL {
	domain := resolveAppLinkDomain(brand)
	if chatID == "" || domain == "" {
		return nil
	}
	return &url.URL{Scheme: "https", Host: domain, Path: "/client/chat/open",
		RawQuery: url.Values{"openChatId": {chatID}}.Encode()}
}

func threadAppLinkURL(chatID, threadID string, brand core.LarkBrand) *url.URL {
	domain := resolveAppLinkDomain(brand)
	if chatID == "" || threadID == "" || domain == "" {
		return nil
	}
	// Desktop and mobile clients use different spellings of these keys.
	return &url.URL{Scheme: "https", Host: domain, Path: "/client/thread/open",
		RawQuery: url.Values{
			"openthreadid": {threadID}, "openchatid": {chatID},
			"open_thread_id": {threadID}, "open_chat_id": {chatID},
		}.Encode()}
}

func (message MessageLinkData) assembleMessageAppLink(brand core.LarkBrand) string {
	var u *url.URL
	if position, ok := normalizeMessagePosition(message.ThreadMessagePosition); ok {
		u = threadAppLinkURL(message.ChatID, message.ThreadID, brand)
		if u != nil {
			query := u.Query()
			query.Set("thread_position", position)
			u.RawQuery = query.Encode()
		}
	}
	if u == nil {
		if position, ok := normalizeMessagePosition(message.MessagePosition); ok {
			u = chatAppLinkURL(message.ChatID, brand)
			if u != nil {
				query := u.Query()
				query.Set("position", position)
				u.RawQuery = query.Encode()
			}
		}
	}
	if u == nil {
		return ""
	}
	return urlrewrite.Rewrite(u.String())
}
