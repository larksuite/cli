// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ImFeedShortcutCreate provides the +feed-shortcut-create shortcut for adding
// chats to the user's feed shortcuts. Currently only CHAT-type shortcuts are
// exposed by the OpenAPI gateway; feed_card_id must be an open_chat_id
// (oc_xxx).
var ImFeedShortcutCreate = common.Shortcut{
	Service:     "im",
	Command:     "+feed-shortcut-create",
	Description: "Add chats to the user's feed shortcuts; user-only; batch up to 10 chat IDs per call; --head/--tail controls insertion order",
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
			Desc: "open_chat_id to add as a feed shortcut (oc_xxx); required; repeat the flag or pass comma-separated; max 10 per call"},
		{Name: "head", Type: "bool",
			Desc: "insert at the top of the shortcut list (default); mutually exclusive with --tail"},
		{Name: "tail", Type: "bool",
			Desc: "append at the bottom of the shortcut list; mutually exclusive with --head"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := collectChatIDs(runtime); err != nil {
			return err
		}
		_, err := resolveIsHeader(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ids, err := collectChatIDs(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		isHeader, err := resolveIsHeader(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/im/v2/feed_shortcuts").
			Body(map[string]any{
				"shortcuts": buildShortcutItems(ids),
				"is_header": isHeader,
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids, err := collectChatIDs(runtime)
		if err != nil {
			return err
		}
		isHeader, err := resolveIsHeader(runtime)
		if err != nil {
			return err
		}
		items := buildShortcutItems(ids)
		data, err := runtime.DoAPIJSONTyped("POST", "/open-apis/im/v2/feed_shortcuts", nil,
			map[string]any{
				"shortcuts": items,
				"is_header": isHeader,
			})
		if err != nil {
			return err
		}
		return emitFeedShortcutWriteResult(runtime, items, data)
	},
}

// resolveIsHeader determines the insertion position.
//   - default (neither flag set) → true (head)
//   - --head → true
//   - --tail → false
//   - both set → error
func resolveIsHeader(rt *common.RuntimeContext) (bool, error) {
	head := rt.Bool("head")
	tail := rt.Bool("tail")
	if head && tail {
		return false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--head and --tail are mutually exclusive")
	}
	if tail {
		return false, nil
	}
	return true, nil
}
