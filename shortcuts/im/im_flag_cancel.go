// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// ImFlagCancel provides the +flag-cancel shortcut for removing a bookmark.
// When no --flag-type is given, it performs double-cancel: removes both message and feed layers.
var ImFlagCancel = common.Shortcut{
	Service: "im",
	Command: "+flag-cancel",
	Description: "Cancel (remove) a bookmark. When no --flag-type is given, " +
		"performs double-cancel: removes both message and feed layers",
	Risk:       "write",
	UserScopes: flagWriteLookupScopes,
	AuthTypes:  []string{"user"},
	HasFormat:  true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx)"},
		{Name: "item-type", Desc: "item type override: default|thread|msg_thread"},
		{Name: "flag-type", Desc: "flag type override: message|feed; omit to double-cancel both layers"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, err := buildCancelItemsForPreview(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		items, _, err := buildCancelItemsForPreview(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		d := common.NewDryRunAPI().
			POST("/open-apis/im/v1/flags/cancel").
			Body(map[string]any{"flag_items": items})
		if len(items) > 1 {
			d.Desc("double-cancel: tries both message and feed layers (best-effort); feed-layer skipped if chat_type undeterminable")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		items, err := buildCancelItems(runtime)
		if err != nil {
			return err
		}

		// Make separate API calls for each item so they are independent.
		// If one fails, the other can still succeed.
		results := make([]map[string]any, 0, len(items))
		var lastErr error
		for _, item := range items {
			itemType := itemTypeString(parseItemTypeFromRaw(item.ItemType))
			flagType := flagTypeString(parseFlagTypeFromRaw(item.FlagType))
			result := map[string]any{
				"item_id":   item.ItemID,
				"item_type": itemType,
				"flag_type": flagType,
			}
			data, err := runtime.DoAPIJSON("POST", "/open-apis/im/v1/flags/cancel", nil,
				map[string]any{"flag_items": []flagItem{item}})
			if err != nil {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: cancel failed for %s/%s: %v\n",
					itemType, flagType, err)
				result["status"] = "failed"
				result["error"] = err.Error()
				lastErr = err
			} else {
				result["status"] = "ok"
				result["response"] = data
			}
			results = append(results, result)
		}

		runtime.Out(map[string]any{"results": results}, nil)
		return lastErr
	},
}

// buildCancelItemsForPreview builds cancel items without API calls.
// It shows double-cancel when no explicit flags are provided.
// DryRun cannot query chat_mode, so feed-layer item_type is represented with
// the same auto-detect placeholder used by +flag-create.
func buildCancelItemsForPreview(rt *common.RuntimeContext) ([]any, bool, error) {
	id, err := flagMessageID(rt)
	if err != nil {
		return nil, false, err
	}

	itOverride := strings.TrimSpace(rt.Str("item-type"))
	ftOverride := strings.TrimSpace(rt.Str("flag-type"))

	// Explicit override provided → single targeted delete
	if itOverride != "" || ftOverride != "" {
		item, err := buildSingleCancelItem(id, itOverride, ftOverride)
		if err != nil {
			return nil, false, err
		}
		return []any{item}, false, nil
	}

	// No override: show double-cancel (message + feed layers)
	// Dry-run shows both layers; actual execution is best-effort.
	return []any{
		newFlagItem(id, ItemTypeDefault, FlagTypeMessage),
		map[string]string{
			"item_id":   id,
			"item_type": "<auto:thread|msg_thread>",
			"flag_type": fmt.Sprintf("%d", int(FlagTypeFeed)),
		},
	}, true, nil
}

// buildCancelItems picks the (item_type, flag_type) pairs to cancel.
//
// Logic:
//  1. If --flag-type is explicitly provided, do a single targeted delete.
//  2. Otherwise, perform double-cancel: remove both message layer and feed layer.
//     - Message layer is always included (uses known message_id with ItemTypeDefault)
//     - Feed layer is best-effort: if chat_type cannot be determined, skip with warning
//     - Each layer is independent; failure to cancel one doesn't block the other
func buildCancelItems(rt *common.RuntimeContext) ([]flagItem, error) {
	id, err := flagMessageID(rt)
	if err != nil {
		return nil, err
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

	// Double-cancel: message layer + feed layer (best effort)
	// Message layer is always included - we have the message_id and know the combo is valid.
	items := []flagItem{newFlagItem(id, ItemTypeDefault, FlagTypeMessage)}

	// Feed layer: try to determine chat_type, but don't fail if we can't.
	// Most messages only have one layer flagged, so this is best-effort cleanup.
	chatID, err := getMessageChatID(rt, id)
	if err != nil {
		// Can't get chat_id, warn and skip feed layer
		fmt.Fprintf(rt.IO().ErrOut, "warning: cannot determine feed-layer item_type: %v; skipping feed-layer cancel\n", err)
		return items, nil
	}

	feedIT, err := resolveThreadFeedItemType(rt, chatID)
	if err != nil {
		// Can't determine chat_type, warn and skip feed layer
		fmt.Fprintf(rt.IO().ErrOut, "warning: cannot determine feed-layer item_type: %v; skipping feed-layer cancel\n", err)
		return items, nil
	}

	// Include feed layer
	items = append(items, newFlagItem(id, feedIT, FlagTypeFeed))
	return items, nil
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
	if !isValidCombo(itemType, flagType) {
		// Provide more specific hints for common mistakes
		if itOverride != "" && ftOverride == "" {
			if itemType == ItemTypeThread || itemType == ItemTypeMsgThread {
				return flagItem{}, output.ErrValidation(
					"invalid combination: --item-type=%s requires --flag-type=feed (feed-layer flags are the only valid type for threads)",
					itOverride)
			}
			return flagItem{}, output.ErrValidation(
				"invalid combination: --item-type=%s with inferred --flag-type=%s; specify --flag-type explicitly to override",
				itOverride, flagTypeString(flagType))
		}
		if itOverride == "" && ftOverride != "" {
			return flagItem{}, output.ErrValidation(
				"invalid combination: --flag-type=%s with inferred --item-type=%s; specify --item-type explicitly to override",
				ftOverride, itemTypeString(itemType))
		}
		return flagItem{}, output.ErrValidation(
			"invalid --item-type/--flag-type combination: supported pairs are default+message, thread+feed, and msg_thread+feed")
	}
	return newFlagItem(id, itemType, flagType), nil
}

// itemTypeString converts ItemType to a user-facing string.
func itemTypeString(it ItemType) string {
	switch it {
	case ItemTypeDefault:
		return "default"
	case ItemTypeThread:
		return "thread"
	case ItemTypeMsgThread:
		return "msg_thread"
	}
	return "unknown"
}

// flagTypeString converts FlagType to a user-facing string.
func flagTypeString(ft FlagType) string {
	switch ft {
	case FlagTypeFeed:
		return "feed"
	case FlagTypeMessage:
		return "message"
	}
	return "unknown"
}
