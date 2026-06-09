// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// ImFeedShortcutRemove provides the +feed-shortcut-remove shortcut for
// removing chats from the user's feed shortcuts. Per-item failures are kept
// in stdout and returned as a partial-failure exit.
var ImFeedShortcutRemove = common.Shortcut{
	Service:     "im",
	Command:     "+feed-shortcut-remove",
	Description: "Remove chats from the user's feed shortcuts; user-only; batch up to 10 chat IDs per call; per-item failures return ok:false with failed_shortcuts",
	Risk:        "write",
	UserScopes:  []string{feedShortcutWriteScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		// chat-id is mandatory but intentionally not cobra-Required: the
		// requiredness check lives in collectChatIDs so a missing flag is
		// reported through the structured validation envelope (exit 2)
		// instead of cobra's plain-text error.
		{Name: "chat-id", Type: "string_slice",
			Desc: "open_chat_id to remove from feed shortcuts (oc_xxx); required; repeat the flag or pass comma-separated; max 10 per call"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := collectChatIDs(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ids, err := collectChatIDs(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/im/v2/feed_shortcuts/remove").
			Body(map[string]any{"shortcuts": buildShortcutItems(ids)})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids, err := collectChatIDs(runtime)
		if err != nil {
			return err
		}
		items := buildShortcutItems(ids)
		data, err := runtime.DoAPIJSONTyped("POST", "/open-apis/im/v2/feed_shortcuts/remove", nil,
			map[string]any{"shortcuts": items})
		if err != nil {
			return err
		}
		return emitFeedShortcutWriteResult(runtime, items, data)
	},
}
