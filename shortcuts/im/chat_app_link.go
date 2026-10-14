// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
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
	return convertlib.ChatAppLink(chatID, brand)
}
