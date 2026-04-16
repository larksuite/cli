// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"context"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var FeedCreate = common.Shortcut{
	Service:     "feed",
	Command:     "+create",
	Description: "Create an app feed card for users; bot-only; sends a clickable card to one or more users' message feeds (requires Lark client v7.6+)",
	Risk:        "write",
	BotScopes:   []string{"im:app_feed_card:write"},
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		{Name: "user-ids", Type: "string_array", Required: true, Desc: "(required) user open_ids to receive the card (ou_xxx, 1-20 users; repeatable: --user-ids ou_aaa --user-ids ou_bbb)"},
		{Name: "link", Required: true, Desc: "(required) clickthrough URL for the card (HTTPS only, max 700 chars)"},
		{Name: "title", Required: true, Desc: "(required) card title shown in the message feed (max 60 chars)"},
		{Name: "preview", Desc: "preview text shown under the title in the feed (max 120 chars)"},
		{Name: "time-sensitive", Type: "bool", Desc: "temporarily pin the card at the top of each recipient's message feed (default false)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildFeedCreateBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/im/v2/app_feed_card").
			Params(map[string]any{"user_id_type": "open_id"}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userIDs := runtime.StrArray("user-ids")
		if len(userIDs) > 20 {
			return output.ErrValidation("--user-ids exceeds maximum of 20 users (got %d)", len(userIDs))
		}
		for _, id := range userIDs {
			if _, err := common.ValidateUserID(id); err != nil {
				return err
			}
		}

		link := runtime.Str("link")
		if !strings.HasPrefix(link, "https://") {
			return output.ErrValidation("--link must use HTTPS protocol (got %q); only https:// URLs are accepted", link)
		}
		if len(link) > 700 {
			return output.ErrValidation("--link exceeds maximum of 700 characters (got %d)", len(link))
		}

		if title := runtime.Str("title"); len([]rune(title)) > 60 {
			return output.ErrValidation("--title exceeds maximum of 60 characters (got %d)", len([]rune(title)))
		}
		if preview := runtime.Str("preview"); len([]rune(preview)) > 120 {
			return output.ErrValidation("--preview exceeds maximum of 120 characters (got %d)", len([]rune(preview)))
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildFeedCreateBody(runtime)
		resData, err := runtime.DoAPIJSON("POST", "/open-apis/im/v2/app_feed_card",
			larkcore.QueryParams{"user_id_type": []string{"open_id"}}, body)
		if err != nil {
			return err
		}

		failedCards, _ := resData["failed_cards"].([]any)
		if failedCards == nil {
			failedCards = []any{}
		}

		runtime.Out(map[string]any{
			"biz_id":       resData["biz_id"],
			"failed_cards": failedCards,
		}, nil)
		return nil
	},
}

func buildFeedCreateBody(runtime *common.RuntimeContext) map[string]any {
	// Normalize user IDs by trimming whitespace
	userIDs := runtime.StrArray("user-ids")
	normalizedIDs := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		normalizedIDs = append(normalizedIDs, strings.TrimSpace(id))
	}

	card := map[string]any{
		"title": runtime.Str("title"),
		"link":  map[string]any{"link": runtime.Str("link")},
	}
	if preview := runtime.Str("preview"); preview != "" {
		card["preview"] = preview
	}
	if runtime.Bool("time-sensitive") {
		card["time_sensitive"] = true
	}
	return map[string]any{
		"app_feed_card": card,
		"user_ids":      normalizedIDs,
	}
}
