// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImFlagCancel = common.Shortcut{
	Service: "im",
	Command: "+flag-cancel",
	Description: "Cancel (remove) a bookmark (标记). When no --flag-type is given, " +
		"checks if the message is a thread root message; if so, cancels both message and feed layers",
	Risk:       "write",
	UserScopes: []string{"im:feed.flag:write"},
	AuthTypes:  []string{"user"},
	HasFormat:  true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx); mutually exclusive with --thread-id"},
		{Name: "thread-id", Desc: "thread ID (omt_xxx); mutually exclusive with --message-id"},
		{Name: "item-type", Desc: "item type override: default|chat|doc|thread|box|open_app|subscription|msg_thread|my_ai|app_feed|knowledge_ai"},
		{Name: "flag-type", Desc: "flag type override: message|feed; omit to auto-detect based on message type"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := common.MutuallyExclusive(runtime, "message-id", "thread-id"); err != nil {
			return err
		}
		if err := common.AtLeastOne(runtime, "message-id", "thread-id"); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		items, _, _ := buildCancelItemsDryRun(runtime)
		d := common.NewDryRunAPI().
			POST("/open-apis/im/v1/flags/cancel").
			Body(map[string]any{"flag_items": items})
		if len(items) > 1 {
			d.Desc("double-cancel: message is a thread root, so both message and feed layers are removed")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		items, err := buildCancelItems(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.DoAPIJSON(http.MethodPost, "/open-apis/im/v1/flags/cancel", nil,
			map[string]any{"flag_items": items})
		if err != nil {
			// Fallback: if the feed item type is wrong, retry with the alternate type.
			// This can happen when chat_mode lookup failed or returned an unexpected value.
			altItems := alternateFeedItemType(items)
			if altItems != nil {
				if altData, altErr := runtime.DoAPIJSON(http.MethodPost, "/open-apis/im/v1/flags/cancel", nil,
					map[string]any{"flag_items": altItems}); altErr == nil {
					runtime.Out(altData, nil)
					return nil
				}
			}
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// buildCancelItemsDryRun is used by DryRun to preview without API calls.
// It assumes worst-case (double-cancel) for om_xxx since we can't query the message.
func buildCancelItemsDryRun(rt *common.RuntimeContext) ([]flagItem, bool, error) {
	id := rt.Str("message-id")
	if id == "" {
		id = rt.Str("thread-id")
	}
	if strings.TrimSpace(id) == "" {
		return nil, false, output.ErrValidation("--message-id / --thread-id is required")
	}

	itOverride := strings.TrimSpace(rt.Str("item-type"))
	ftOverride := strings.TrimSpace(rt.Str("flag-type"))

	// Explicit override provided → single targeted delete
	if itOverride != "" || ftOverride != "" {
		item, err := buildSingleCancelItem(id, itOverride, ftOverride)
		if err != nil {
			return nil, false, err
		}
		return []flagItem{item}, false, nil
	}

	// No override: for dry-run, assume double-cancel for om_xxx
	if strings.HasPrefix(id, "omt_") {
		return []flagItem{
			newFlagItem(id, ItemTypeThread, FlagTypeFeed),
			newFlagItem(id, ItemTypeDefault, FlagTypeMessage),
		}, true, nil
	}
	// om_xxx: assume could be thread root for dry-run preview
	return []flagItem{
		newFlagItem(id, ItemTypeDefault, FlagTypeMessage),
		newFlagItem(id, ItemTypeMsgThread, FlagTypeFeed),
	}, true, nil
}

// buildCancelItems picks the (item_type, flag_type) pairs to cancel.
//
// Logic:
//  1. If --flag-type is explicitly provided, do a single targeted delete.
//  2. For omt_xxx (thread ID), query the chat API to determine chat_mode,
//     then double-cancel with the correct feed-layer item_type:
//     - 话题群 (chat_mode=topic) → (thread, feed) + (default, message)
//     - 普通群 (chat_mode=group) → (msg_thread, feed) + (default, message)
//  3. For om_xxx (message ID), query the message to check if it's a thread root:
//     - If thread_id is present, query the chat API to determine chat_mode:
//     - 话题群 (chat_mode=topic) → (thread, feed) + (default, message)
//     - 普通群 (chat_mode=group) → (msg_thread, feed) + (default, message)
//     - If thread_id is absent → single cancel (default, message)
func buildCancelItems(rt *common.RuntimeContext) ([]flagItem, error) {
	id := rt.Str("message-id")
	if id == "" {
		id = rt.Str("thread-id")
	}
	if strings.TrimSpace(id) == "" {
		return nil, output.ErrValidation("--message-id / --thread-id is required")
	}

	itOverride := strings.TrimSpace(rt.Str("item-type"))
	ftOverride := strings.TrimSpace(rt.Str("flag-type"))

	// Explicit override provided → single targeted delete
	if itOverride != "" || ftOverride != "" {
		item, err := buildSingleCancelItem(id, itOverride, ftOverride)
		if err != nil {
			return nil, err
		}
		return []flagItem{item}, nil
	}

	// omt_xxx (thread ID) → need chat_mode to choose between thread vs msg_thread
	if strings.HasPrefix(id, "omt_") {
		feedIT := resolveThreadFeedItemTypeFromThread(rt, id)
		return []flagItem{
			newFlagItem(id, feedIT, FlagTypeFeed),
			newFlagItem(id, ItemTypeDefault, FlagTypeMessage),
		}, nil
	}

	// om_xxx: query message to check if it's a thread root
	isThreadRoot, chatID, err := checkIsThreadRoot(rt, id)
	if err != nil {
		// If query fails, fall back to single delete to avoid permission errors
		return []flagItem{newFlagItem(id, ItemTypeDefault, FlagTypeMessage)}, nil
	}

	if isThreadRoot {
		// Thread root message: determine feed-layer item_type from chat_mode
		feedIT := resolveThreadFeedItemType(rt, chatID)
		return []flagItem{
			newFlagItem(id, ItemTypeDefault, FlagTypeMessage),
			newFlagItem(id, feedIT, FlagTypeFeed),
		}, nil
	}

	// Regular message: single cancel
	return []flagItem{newFlagItem(id, ItemTypeDefault, FlagTypeMessage)}, nil
}

// buildSingleCancelItem builds a single cancel item when user provides explicit flags.
func buildSingleCancelItem(id, itOverride, ftOverride string) (flagItem, error) {
	var itemType ItemType
	var flagType FlagType

	if itOverride != "" {
		it, err := parseItemType(itOverride)
		if err != nil {
			return flagItem{}, err
		}
		itemType = it
	}
	if ftOverride != "" {
		ft, err := parseFlagType(ftOverride)
		if err != nil {
			return flagItem{}, err
		}
		flagType = ft
	}
	if itOverride == "" || ftOverride == "" {
		inferIT, inferFT, err := parseItemID(id)
		if err != nil {
			return flagItem{}, err
		}
		if itOverride == "" {
			itemType = inferIT
		}
		if ftOverride == "" {
			flagType = inferFT
		}
	}
	return newFlagItem(id, itemType, flagType), nil
}

// alternateFeedItemType swaps any feed-layer item between ItemTypeThread and ItemTypeMsgThread.
// Returns nil if no feed item is found (nothing to swap).
// Used as a fallback when the first cancel attempt fails due to item_type mismatch.
func alternateFeedItemType(items []flagItem) []flagItem {
	var result []flagItem
	swapped := false
	for _, it := range items {
		if it.FlagType == fmt.Sprintf("%d", int(FlagTypeFeed)) && !swapped {
			parsedIT := parseItemTypeFromRaw(it.ItemType)
			var altIT ItemType
			switch parsedIT {
			case ItemTypeThread:
				altIT = ItemTypeMsgThread
			case ItemTypeMsgThread:
				altIT = ItemTypeThread
			default:
				result = append(result, it)
				continue
			}
			result = append(result, newFlagItem(it.ItemID, altIT, FlagTypeFeed))
			swapped = true
		} else {
			result = append(result, it)
		}
	}
	if !swapped {
		return nil
	}
	return result
}

// parseItemTypeFromRaw parses a stringified numeric item_type back to ItemType.
func parseItemTypeFromRaw(s string) ItemType {
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
