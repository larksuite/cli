// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// ImFeedGroupListItem provides the +feed-group-list-item shortcut: it lists the
// feed cards inside one feed group and enriches each item with chat_name resolved
// from its feed_id.
var ImFeedGroupListItem = common.Shortcut{
	Service:     "im",
	Command:     "+feed-group-list-item",
	Description: "List feed cards in a feed group (tag); user-only; enriches each item with chat_name resolved from feed_id; supports --page-all auto-pagination",
	Risk:        "read",
	UserScopes:  []string{feedGroupReadScope, chatReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "feed-group-id", Desc: "feed group ID (ofg_xxx); path parameter (required)"},
		{Name: "page-size", Type: "int", Default: "50", Desc: "page size (1-50)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max pages when auto-pagination is enabled (default 20, max 1000)"},
		{Name: "start-time", Desc: "update-time window start (Unix milliseconds as a decimal string)"},
		{Name: "end-time", Desc: "update-time window end (Unix milliseconds as a decimal string)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFeedGroupListOptions(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if err := validateFeedGroupListOptions(runtime); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			GET(feedGroupListItemPath(runtime)).
			Params(feedGroupListDryRunParams(runtime)).
			Desc("will also POST /open-apis/im/v1/chats/batch_query to resolve chat_name from feed_id; requires im:chat:read")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// When --page-token is explicitly provided, the user wants a specific page —
		// no auto-pagination regardless of --page-all.
		if runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token") {
			return executeFeedGroupListAllPages(runtime)
		}

		data, err := runtime.DoAPIJSON("GET", feedGroupListItemPath(runtime), feedGroupListQuery(runtime), nil)
		if err != nil {
			return err
		}
		enrichFeedGroupItemsChatName(runtime, data)

		hasMore, _ := data["has_more"].(bool)
		runtime.OutFormat(data, nil, func(w io.Writer) {
			renderFeedGroupItemsTable(w, data, hasMore)
		})
		return nil
	},
}

func validateFeedGroupListOptions(rt *common.RuntimeContext) error {
	if rt.Str("feed-group-id") == "" {
		return output.ErrValidation("--feed-group-id is required")
	}
	if n := rt.Int("page-size"); n < 1 || n > 50 {
		return output.ErrValidation("--page-size must be an integer between 1 and 50")
	}
	if n := rt.Int("page-limit"); n < 1 || n > 1000 {
		return output.ErrValidation("--page-limit must be an integer between 1 and 1000")
	}
	if v := rt.Str("start-time"); v != "" {
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return output.ErrValidation("--start-time must be Unix milliseconds (a decimal integer string)")
		}
	}
	if v := rt.Str("end-time"); v != "" {
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return output.ErrValidation("--end-time must be Unix milliseconds (a decimal integer string)")
		}
	}
	return nil
}

// feedGroupListItemPath builds the list_item endpoint path with the feed_group_id
// segment safely encoded.
func feedGroupListItemPath(rt *common.RuntimeContext) string {
	return "/open-apis/im/v1/groups/" + validate.EncodePathSegment(rt.Str("feed-group-id")) + "/list_item"
}

// feedGroupListQuery builds the query parameters, sending only non-empty values.
func feedGroupListQuery(rt *common.RuntimeContext) larkcore.QueryParams {
	params := larkcore.QueryParams{
		"page_size": []string{strconv.Itoa(rt.Int("page-size"))},
	}
	if token := rt.Str("page-token"); token != "" {
		params["page_token"] = []string{token}
	}
	if start := rt.Str("start-time"); start != "" {
		params["start_time"] = []string{start}
	}
	if end := rt.Str("end-time"); end != "" {
		params["end_time"] = []string{end}
	}
	return params
}

// feedGroupListDryRunParams mirrors feedGroupListQuery for dry-run display.
func feedGroupListDryRunParams(rt *common.RuntimeContext) map[string]any {
	params := map[string]any{
		"page_size": strconv.Itoa(rt.Int("page-size")),
	}
	if token := rt.Str("page-token"); token != "" {
		params["page_token"] = token
	}
	if start := rt.Str("start-time"); start != "" {
		params["start_time"] = start
	}
	if end := rt.Str("end-time"); end != "" {
		params["end_time"] = end
	}
	return params
}

// executeFeedGroupListAllPages fetches all pages and merges items/deleted_items
// into a single response, then enriches the merged result.
func executeFeedGroupListAllPages(rt *common.RuntimeContext) error {
	maxPages := rt.Int("page-limit")
	if maxPages < 1 {
		maxPages = 20
	}
	if maxPages > 1000 {
		maxPages = 1000
	}

	// Use make([]any, 0) so empty arrays serialize as [] not null.
	allItems := make([]any, 0)
	allDeletedItems := make([]any, 0)
	var lastHasMore bool
	var lastPageToken string
	prevPageToken := "__START__"

	for page := 0; page < maxPages; page++ {
		params := larkcore.QueryParams{
			"page_size": []string{strconv.Itoa(rt.Int("page-size"))},
		}
		if page > 0 {
			params["page_token"] = []string{lastPageToken}
		}
		if start := rt.Str("start-time"); start != "" {
			params["start_time"] = []string{start}
		}
		if end := rt.Str("end-time"); end != "" {
			params["end_time"] = []string{end}
		}

		data, err := rt.DoAPIJSON("GET", feedGroupListItemPath(rt), params, nil)
		if err != nil {
			return err
		}

		if v, ok := data["items"].([]any); ok {
			allItems = append(allItems, v...)
		}
		if v, ok := data["deleted_items"].([]any); ok {
			allDeletedItems = append(allDeletedItems, v...)
		}

		lastHasMore, _ = data["has_more"].(bool)
		lastPageToken, _ = data["page_token"].(string)

		fmt.Fprintf(rt.IO().ErrOut, "page %d: %d items, %d deleted\n",
			page+1, len(allItems), len(allDeletedItems))

		if !lastHasMore || lastPageToken == "" {
			break
		}
		if lastPageToken == prevPageToken {
			fmt.Fprintf(rt.IO().ErrOut, "warning: page_token did not change, stopping pagination to avoid infinite loop\n")
			break
		}
		prevPageToken = lastPageToken
	}

	merged := map[string]any{
		"items":         allItems,
		"deleted_items": allDeletedItems,
		"has_more":      lastHasMore,
		"page_token":    lastPageToken,
	}

	enrichFeedGroupItemsChatName(rt, merged)

	rt.OutFormat(merged, nil, func(w io.Writer) {
		renderFeedGroupItemsTable(w, merged, lastHasMore)
	})
	return nil
}
