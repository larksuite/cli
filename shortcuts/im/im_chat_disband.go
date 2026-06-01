// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImChatDisband = common.Shortcut{
	Service:     "im",
	Command:     "+chat-disband",
	Description: "Disband a group chat; user/bot; high-risk, permanently dissolves the chat",
	Risk:        "high-risk-write",
	Scopes:      []string{"im:chat:delete"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "chat-id", Desc: "group chat ID to disband (oc_xxx)", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		chatID := runtime.Str("chat-id")
		return common.NewDryRunAPI().
			DELETE("/open-apis/im/v1/chats/:chat_id").
			Set("chat_id", chatID).
			Desc("high-risk: disbands the entire group chat; use --yes to execute")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidateChatID(runtime.Str("chat-id"))
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chatID := runtime.Str("chat-id")
		if _, err := runtime.DoAPIJSON(http.MethodDelete,
			fmt.Sprintf("/open-apis/im/v1/chats/%s", validate.EncodePathSegment(chatID)),
			nil,
			nil); err != nil {
			return err
		}

		runtime.OutFormat(map[string]interface{}{
			"chat_id":   chatID,
			"disbanded": true,
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Group disbanded successfully (chat_id: %s)\n", chatID)
		})
		return nil
	},
}
