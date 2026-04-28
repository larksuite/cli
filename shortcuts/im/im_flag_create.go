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

// ImFlagCreate provides the +flag-create shortcut for creating a bookmark on a message.
var ImFlagCreate = common.Shortcut{
	Service:     "im",
	Command:     "+flag-create",
	Description: "Create a bookmark on a message; user-only; defaults to message-layer flag; use --flag-type feed to create feed-layer flag (auto-detects chat type)",
	Risk:        "write",
	UserScopes:  flagWriteLookupScopes,
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx)"},
		{Name: "item-type", Desc: "item type override: default|thread|msg_thread (rarely needed)"},
		{Name: "flag-type", Desc: "flag type: message (default) or feed"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildCreateItemForPreview(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		item, err := buildCreateItemForPreview(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		d := common.NewDryRunAPI().
			POST("/open-apis/im/v1/flags").
			Body(map[string]any{"flag_items": []any{item}})
		if m, ok := item.(map[string]string); ok && m["item_type"] == "<auto:thread|msg_thread>" {
			d.Desc("feed-layer item_type is auto-detected at execution time by reading the message chat and chat_mode")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		item, err := buildCreateItem(runtime)
		if err != nil {
			return err
		}
		// Combo validation already done in Validate, but double-check as a safety net.
		if !isValidCombo(parseItemTypeFromRaw(item.ItemType), parseFlagTypeFromRaw(item.FlagType)) {
			return output.ErrValidation(
				"invalid (item_type=%s, flag_type=%s) combination; the server only accepts "+
					"(default, message), (thread, feed), or (msg_thread, feed)",
				item.ItemType, item.FlagType)
		}
		data, err := runtime.DoAPIJSON("POST", "/open-apis/im/v1/flags", nil,
			map[string]any{"flag_items": []flagItem{item}})
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// buildCreateItemForPreview derives a preview payload without making network calls.
// Feed-layer execution auto-detects item_type from chat_mode, but dry-run must
// not query the message or chat APIs, so it uses an explicit placeholder.
func buildCreateItemForPreview(rt *common.RuntimeContext) (any, error) {
	id, err := flagMessageID(rt)
	if err != nil {
		return nil, err
	}

	itOverride := strings.TrimSpace(rt.Str("item-type"))
	ftOverride := strings.TrimSpace(rt.Str("flag-type"))
	combo, err := parseExplicitFlagCombo(itOverride, ftOverride)
	if err != nil {
		return nil, err
	}

	flagType := FlagTypeMessage
	if combo.FlagTypeSet {
		flagType = combo.FlagType
	}
	if flagType == FlagTypeMessage {
		return newFlagItem(id, ItemTypeDefault, FlagTypeMessage), nil
	}

	if combo.ItemTypeSet {
		return newFlagItem(id, combo.ItemType, FlagTypeFeed), nil
	}

	return map[string]string{
		"item_id":   id,
		"item_type": "<auto:thread|msg_thread>",
		"flag_type": fmt.Sprintf("%d", int(FlagTypeFeed)),
	}, nil
}

// buildCreateItem derives a flagItem for the create path.
//
// Resolution logic:
//  1. No --flag-type or --flag-type=message → (default, message)
//  2. --flag-type=feed (no --item-type) → query message to get chat_id,
//     then query chat_mode to determine: topic-style → (thread, feed), regular → (msg_thread, feed)
//  3. Both --item-type and --flag-type provided → honor verbatim (for edge cases)
func buildCreateItem(rt *common.RuntimeContext) (flagItem, error) {
	id, err := flagMessageID(rt)
	if err != nil {
		return flagItem{}, err
	}

	itOverride := strings.TrimSpace(rt.Str("item-type"))
	ftOverride := strings.TrimSpace(rt.Str("flag-type"))
	combo, err := parseExplicitFlagCombo(itOverride, ftOverride)
	if err != nil {
		return flagItem{}, err
	}

	flagType := FlagTypeMessage
	if combo.FlagTypeSet {
		flagType = combo.FlagType
	}

	// Message-layer flag: always (default, message)
	if flagType == FlagTypeMessage {
		return newFlagItem(id, ItemTypeDefault, FlagTypeMessage), nil
	}

	// Feed-layer flag: need to determine item_type from chat_mode
	if combo.ItemTypeSet {
		// User explicitly specified item-type, honor it
		return newFlagItem(id, combo.ItemType, FlagTypeFeed), nil
	}

	chatID, err := getMessageChatID(rt, id)
	if err != nil {
		return flagItem{}, output.ErrValidation(
			"failed to query message for feed-layer flag: %v; if you know the chat type, specify --item-type explicitly", err)
	}
	if chatID == "" {
		return flagItem{}, output.ErrValidation(
			"message does not belong to a chat; feed-layer flags are only for messages in chats")
	}

	feedIT, err := resolveThreadFeedItemType(rt, chatID)
	if err != nil {
		return flagItem{}, output.ErrValidation(
			"failed to determine chat type: %v; if you know the chat type, specify --item-type explicitly", err)
	}
	return newFlagItem(id, feedIT, FlagTypeFeed), nil
}

type explicitFlagCombo struct {
	ItemType    ItemType
	FlagType    FlagType
	ItemTypeSet bool
	FlagTypeSet bool
}

func parseExplicitFlagCombo(itOverride, ftOverride string) (explicitFlagCombo, error) {
	itOverride = strings.TrimSpace(itOverride)
	ftOverride = strings.TrimSpace(ftOverride)

	var combo explicitFlagCombo
	if itOverride != "" {
		it, err := parseItemType(itOverride)
		if err != nil {
			return explicitFlagCombo{}, err
		}
		combo.ItemType = it
		combo.ItemTypeSet = true
	}
	if ftOverride != "" {
		ft, err := parseFlagType(ftOverride)
		if err != nil {
			return explicitFlagCombo{}, err
		}
		combo.FlagType = ft
		combo.FlagTypeSet = true
	}

	if combo.ItemTypeSet && !combo.FlagTypeSet {
		switch combo.ItemType {
		case ItemTypeThread, ItemTypeMsgThread:
			return explicitFlagCombo{}, output.ErrValidation(
				"--item-type=%s requires --flag-type=feed; message-layer flags always use item-type=default", itOverride)
		case ItemTypeDefault:
			return explicitFlagCombo{}, output.ErrValidation(
				"--item-type=default requires --flag-type=message; or omit both to use default behavior")
		}
	}

	if combo.ItemTypeSet && combo.FlagTypeSet && !isValidCombo(combo.ItemType, combo.FlagType) {
		return explicitFlagCombo{}, output.ErrValidation(
			"invalid --item-type=%s --flag-type=%s combination; supported pairs are default+message, thread+feed, and msg_thread+feed",
			itOverride, ftOverride)
	}

	return combo, nil
}

// validateExplicitCombo validates the (item_type, flag_type) combination when
// the user explicitly provides flags. It does not make API calls - it only
// validates the logic for what the user explicitly specified.
func validateExplicitCombo(itOverride, ftOverride string) error {
	_, err := parseExplicitFlagCombo(itOverride, ftOverride)
	return err
}
