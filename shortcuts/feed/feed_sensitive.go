// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"context"

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
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return nil // TODO
	},
}
