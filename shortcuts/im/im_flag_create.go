// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImFlagCreate = common.Shortcut{
	Service:     "im",
	Command:     "+flag-create",
	Description: "Create a bookmark (标记) on a message or thread",
	Risk:        "write",
	UserScopes:  []string{"im:feed.flag:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx); mutually exclusive with --thread-id"},
		{Name: "thread-id", Desc: "thread ID (omt_xxx); mutually exclusive with --message-id"},
		{Name: "item-type", Desc: "item type override: default|chat|doc|thread|box|open_app|subscription|msg_thread|my_ai|app_feed|knowledge_ai"},
		{Name: "flag-type", Desc: "flag type override: message|feed"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := common.MutuallyExclusive(runtime, "message-id", "thread-id"); err != nil {
			return err
		}
		if err := common.AtLeastOne(runtime, "message-id", "thread-id"); err != nil {
			return err
		}
		item, err := buildCreateItem(runtime)
		if err != nil {
			return err
		}
		// Validate (item_type, flag_type) combination before DryRun/Execute.
		if !isValidCombo(mustParseItemType(item.ItemType), mustParseFlagType(item.FlagType)) {
			return output.ErrValidation(
				"invalid (item_type=%s, flag_type=%s) combination; the server only accepts "+
					"(default, message), (thread, feed), or (msg_thread, feed)",
				item.ItemType, item.FlagType)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		item, _ := buildCreateItem(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/im/v1/flags").
			Body(map[string]any{"flag_items": []flagItem{item}})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		item, err := buildCreateItem(runtime)
		if err != nil {
			return err
		}
		// Combo validation already done in Validate, but double-check as a safety net.
		if !isValidCombo(mustParseItemType(item.ItemType), mustParseFlagType(item.FlagType)) {
			return output.ErrValidation(
				"invalid (item_type=%s, flag_type=%s) combination; the server only accepts "+
					"(default, message), (thread, feed), or (msg_thread, feed)",
				item.ItemType, item.FlagType)
		}
		data, err := runtime.DoAPIJSON(http.MethodPost, "/open-apis/im/v1/flags", nil,
			map[string]any{"flag_items": []flagItem{item}})
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// buildCreateItem derives a flagItem for the create path.
//
// Product rule: when the caller does not explicitly say whether to flag a
// message or a feed, default to flagging the message — i.e. (ItemTypeDefault,
// FlagTypeMessage). This applies uniformly:
//   - regular message ids (om_xxx)     → (default, message)
//   - thread ids (omt_xxx)             → (default, message)
//   - thread root messages (om_xxx anchoring a thread, which can be flagged
//     either as message or as feed) → (default, message)
//
// Feed-layer flagging — (thread, feed) for 话题群 or (msg_thread, feed) for
// 普通群 — is an opt-in and must be requested explicitly via both --item-type
// and --flag-type.
//
// Resolution order:
//  1. User passed --item-type and/or --flag-type → both are required, honor verbatim.
//  2. Otherwise → (default, message).
func buildCreateItem(rt *common.RuntimeContext) (flagItem, error) {
	id := rt.Str("message-id")
	if id == "" {
		id = rt.Str("thread-id")
	}
	if strings.TrimSpace(id) == "" {
		return flagItem{}, output.ErrValidation("--message-id or --thread-id is required")
	}

	itOverride := strings.TrimSpace(rt.Str("item-type"))
	ftOverride := strings.TrimSpace(rt.Str("flag-type"))

	// Explicit override path — honor both.
	if itOverride != "" || ftOverride != "" {
		if itOverride == "" || ftOverride == "" {
			return flagItem{}, output.ErrValidation(
				"--item-type and --flag-type must be provided together when overriding the default message route")
		}
		it, err := parseItemType(itOverride)
		if err != nil {
			return flagItem{}, err
		}
		ft, err := parseFlagType(ftOverride)
		if err != nil {
			return flagItem{}, err
		}
		return newFlagItem(id, it, ft), nil
	}

	// Default path — always (default, message), regardless of id prefix.
	return newFlagItem(id, ItemTypeDefault, FlagTypeMessage), nil
}

// helpers used inside Execute to re-parse the stringified enum back to int for
// the combo-validity check.
func mustParseItemType(s string) ItemType {
	switch s {
	case "0":
		return ItemTypeDefault
	case "4":
		return ItemTypeThread
	case "11":
		return ItemTypeMsgThread
	}
	return ItemTypeDefault
}

func mustParseFlagType(s string) FlagType {
	switch s {
	case "1":
		return FlagTypeFeed
	case "2":
		return FlagTypeMessage
	}
	return FlagTypeUnknown
}
