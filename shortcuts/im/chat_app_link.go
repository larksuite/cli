// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/urlrewrite"
	"github.com/larksuite/cli/shortcuts/common"
)

func addChatAppLinks(chats []map[string]interface{}, runtime *common.RuntimeContext) error {
	if runtime == nil || runtime.Config == nil {
		return nil
	}
	for _, chat := range chats {
		if link, err := assembleChatAppLink(runtime.Ctx(), chat["chat_id"], runtime.Config.Brand); err != nil {
			return err
		} else if link != "" {
			chat["chat_app_link"] = link
		}
	}
	return nil
}

func assembleChatAppLink(ctx context.Context, rawChatID interface{}, brand core.LarkBrand) (string, error) {
	chatID, _ := rawChatID.(string)
	chatID = strings.TrimSpace(chatID)
	if !strings.HasPrefix(chatID, "oc_") {
		return "", nil
	}
	domain := resolveChatAppLinkDomain(brand)
	if domain == "" {
		return "", nil
	}

	u := &url.URL{Scheme: "https", Host: domain, Path: "/client/chat/open"}
	q := url.Values{}
	q.Set("openChatId", chatID)
	u.RawQuery = q.Encode()
	return urlrewrite.Rewrite(ctx, u.String())
}

func resolveChatAppLinkDomain(brand core.LarkBrand) string {
	appLink := core.ResolveEndpoints(brand).AppLink
	u, err := url.Parse(appLink)
	if err != nil {
		return ""
	}
	return u.Host
}
