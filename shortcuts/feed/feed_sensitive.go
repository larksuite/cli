// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var FeedSensitive = common.Shortcut{
	Service:     "feed",
	Command:     "+sensitive",
	Description: "Set or unset time-sensitive (即时提醒) status for a feed card (group chat) for specified users; bot only",
	Risk:        "write",
	BotScopes:   []string{"im:datasync.feed_card.time_sensitive:write"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "feed-card-id", Desc: "feed card ID (oc_xxx); group chats only", Required: true},
		{Name: "enable", Type: "bool", Desc: "enable time-sensitive (pin card to top for specified users)"},
		{Name: "disable", Type: "bool", Desc: "disable time-sensitive"},
		{Name: "user-ids", Type: "string_slice", Desc: "user ID list (comma-separated or repeatable); must be members of the feed card chat", Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type", Enum: []string{"open_id", "union_id", "user_id"}},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		feedCardID := runtime.Str("feed-card-id")
		userIDs := runtime.StrSlice("user-ids")
		userIDType := runtime.Str("user-id-type")
		timeSensitive := runtime.Changed("enable")
		return common.NewDryRunAPI().
			PATCH(fmt.Sprintf("/open-apis/im/v2/feed_cards/%s", validate.EncodePathSegment(feedCardID))).
			Params(map[string]any{"user_id_type": userIDType}).
			Body(map[string]any{
				"time_sensitive": timeSensitive,
				"user_ids":       userIDs,
			})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		feedCardID := runtime.Str("feed-card-id")
		if _, err := common.ValidateChatID(feedCardID); err != nil {
			return err
		}

		enableChanged := runtime.Changed("enable")
		disableChanged := runtime.Changed("disable")
		if enableChanged && disableChanged {
			return common.FlagErrorf("--enable and --disable are mutually exclusive")
		}
		if !enableChanged && !disableChanged {
			return common.FlagErrorf("specify exactly one of --enable or --disable")
		}

		userIDs := runtime.StrSlice("user-ids")
		if len(userIDs) == 0 {
			return output.ErrValidation("--user-ids is required and must not be empty")
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		feedCardID := runtime.Str("feed-card-id")
		userIDs := runtime.StrSlice("user-ids")
		userIDType := runtime.Str("user-id-type")
		timeSensitive := runtime.Changed("enable")

		resp, err := runtime.CallAPI("PATCH",
			fmt.Sprintf("/open-apis/im/v2/feed_cards/%s", validate.EncodePathSegment(feedCardID)),
			map[string]any{"user_id_type": userIDType},
			map[string]any{"time_sensitive": timeSensitive, "user_ids": userIDs},
		)
		if err != nil {
			return err
		}

		failedReasons := make([]map[string]any, 0)
		if raw, ok := resp["failed_user_reasons"]; ok && raw != nil {
			if slice, ok := raw.([]any); ok {
				for _, item := range slice {
					if m, ok := item.(map[string]any); ok {
						failedReasons = append(failedReasons, m)
					}
				}
			}
		}

		outData := map[string]any{
			"feed_card_id":        feedCardID,
			"time_sensitive":      timeSensitive,
			"failed_user_reasons": failedReasons,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			successCount := len(userIDs) - len(failedReasons)
			if successCount > 0 {
				fmt.Fprintf(w, "Time-sensitive updated for %d user(s) (feed_card_id: %s)\n", successCount, feedCardID)
			}
			if len(failedReasons) > 0 {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: %d user(s) failed:\n", len(failedReasons))
				for _, r := range failedReasons {
					fmt.Fprintf(runtime.IO().ErrOut, "  %v: %v\n", r["user_id"], r["error_message"])
				}
			}
		})

		if len(failedReasons) > 0 {
			if len(failedReasons) == len(userIDs) {
				return output.Errorf(output.ExitAPI, "partial_failure", "all %d user(s) failed to update time-sensitive status", len(userIDs))
			}
			return output.Errorf(output.ExitAPI, "partial_failure", "%d user(s) failed to update time-sensitive status", len(failedReasons))
		}
		return nil
	},
}
