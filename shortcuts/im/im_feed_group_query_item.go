// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"io"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// ImFeedGroupQueryItem provides the +feed-group-query-item shortcut: it looks up
// specific feed cards in a feed group by ID and enriches each item with chat_name
// resolved from its feed_id.
var ImFeedGroupQueryItem = common.Shortcut{
	Service:     "im",
	Command:     "+feed-group-query-item",
	Description: "Look up specific feed cards in a feed group (tag) by ID; user-only; enriches each item with chat_name resolved from feed_id",
	Risk:        "read",
	UserScopes:  []string{feedGroupReadScope, chatReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "feed-group-id", Desc: "feed group ID (ofg_xxx); path parameter (required)"},
		{Name: "feed-id", Desc: "comma-separated chat IDs (oc_xxx); feed_type is fixed to chat (required)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildFeedGroupQueryItemBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildFeedGroupQueryItemBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(feedGroupQueryItemPath(runtime)).
			Body(body).
			Desc("will also POST /open-apis/im/v1/chats/batch_query to resolve chat_name from feed_id; requires im:chat:read")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildFeedGroupQueryItemBody(runtime)
		if err != nil {
			return err
		}

		data, err := runtime.DoAPIJSONTyped("POST", feedGroupQueryItemPath(runtime), nil, body)
		if err != nil {
			return err
		}
		enrichFeedGroupItemsChatName(runtime, data)

		runtime.OutFormat(data, nil, func(w io.Writer) {
			renderFeedGroupItemsTable(w, data, false)
		})
		return nil
	},
}

// feedGroupQueryItemPath builds the batch_query_item endpoint path with the
// feed_group_id segment safely encoded.
func feedGroupQueryItemPath(rt *common.RuntimeContext) string {
	return "/open-apis/im/v1/groups/" + validate.EncodePathSegment(rt.Str("feed-group-id")) + "/batch_query_item"
}

// buildFeedGroupQueryItemBody validates the flags and constructs the request body
// {"items":[{"feed_id":"<tok>","feed_type":"chat"}, ...]}.
func buildFeedGroupQueryItemBody(rt *common.RuntimeContext) (map[string]any, error) {
	if rt.Str("feed-group-id") == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--feed-group-id is required").WithParam("--feed-group-id")
	}
	tokens := common.SplitCSV(rt.Str("feed-id"))
	items := make([]any, 0, len(tokens))
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		items = append(items, map[string]any{
			"feed_id":   tok,
			"feed_type": "chat",
		})
	}
	if len(items) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--feed-id is required (comma-separated chat IDs)").WithParam("--feed-id")
	}
	return map[string]any{"items": items}, nil
}
