// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

func addChatAppLinks(chats []map[string]interface{}, runtime *common.RuntimeContext) {
	if runtime == nil || runtime.Config == nil {
		return
	}
	for _, chat := range chats {
		if link := assembleChatAppLink(chat["chat_id"], runtime.Config.Brand); link != "" {
			chat["chat_app_link"] = link
		}
	}
}

func assembleChatAppLink(rawChatID interface{}, brand core.LarkBrand) string {
	chatID, _ := rawChatID.(string)
	chatID = strings.TrimSpace(chatID)
	if !strings.HasPrefix(chatID, "oc_") {
		return ""
	}
	domain := resolveChatAppLinkDomain(brand)
	if domain == "" {
		return ""
	}

	u := &url.URL{Scheme: "https", Host: domain, Path: "/client/chat/open"}
	q := url.Values{}
	q.Set("openChatId", chatID)
	u.RawQuery = q.Encode()
	return u.String()
}

func resolveChatAppLinkDomain(brand core.LarkBrand) string {
	appLink := core.ResolveEndpoints(brand).AppLink
	u, err := url.Parse(appLink)
	if err != nil {
		return ""
	}
	return u.Host
}
