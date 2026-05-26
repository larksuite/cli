// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const feedGroupListPath = "/open-apis/im/v1/groups"

// ImFeedGroupList provides the +feed-group-list shortcut: it lists the caller's
// feed groups (tags) with auto-pagination that correctly merges BOTH the live
// (groups) and soft-deleted (deleted_groups) lists across pages.
//
// The raw `feed.groups list --page-all` goes through the generic paginator,
// which follows only one array field and silently drops the other list's later
// pages; this shortcut paginates the dual-list response itself.
var ImFeedGroupList = common.Shortcut{
	Service:     "im",
	Command:     "+feed-group-list",
	Description: "List the caller's feed groups (tags); user-only; supports `--page-all` auto-pagination",
	Risk:        "read",
	UserScopes:  []string{feedGroupReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: "50", Desc: "page size (1-50)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max pages when auto-pagination is enabled (default 20, max 1000)"},
		{Name: "start-time", Desc: "update-time window start (Unix milliseconds as a decimal string)"},
		{Name: "end-time", Desc: "update-time window end (Unix milliseconds as a decimal string)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFeedGroupListPageOptions(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if err := validateFeedGroupListPageOptions(runtime); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			GET(feedGroupListPath).
			Params(feedGroupListGroupsDryRunParams(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// When --page-token is explicitly provided, the user wants a specific
		// page — no auto-pagination regardless of --page-all.
		if runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token") {
			return executeFeedGroupListGroupsAllPages(runtime)
		}

		data, err := runtime.DoAPIJSON("GET", feedGroupListPath, feedGroupListGroupsQuery(runtime), nil)
		if err != nil {
			return err
		}

		hasMore, _ := data["has_more"].(bool)
		runtime.OutFormat(data, nil, func(w io.Writer) {
			renderFeedGroupsTable(w, data, hasMore)
		})
		return nil
	},
}

func validateFeedGroupListPageOptions(rt *common.RuntimeContext) error {
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

// feedGroupListGroupsQuery builds the query parameters. page_token is always
// sent (empty string = first page) because the groups endpoint rejects requests
// that omit it (HTTP 400 "Missing required parameter: page_token").
func feedGroupListGroupsQuery(rt *common.RuntimeContext) larkcore.QueryParams {
	params := larkcore.QueryParams{
		"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
		"page_token": []string{rt.Str("page-token")},
	}
	if start := rt.Str("start-time"); start != "" {
		params["start_time"] = []string{start}
	}
	if end := rt.Str("end-time"); end != "" {
		params["end_time"] = []string{end}
	}
	return params
}

// feedGroupListGroupsDryRunParams mirrors feedGroupListGroupsQuery for dry-run display.
func feedGroupListGroupsDryRunParams(rt *common.RuntimeContext) map[string]any {
	params := map[string]any{
		"page_size":  strconv.Itoa(rt.Int("page-size")),
		"page_token": rt.Str("page-token"),
	}
	if start := rt.Str("start-time"); start != "" {
		params["start_time"] = start
	}
	if end := rt.Str("end-time"); end != "" {
		params["end_time"] = end
	}
	return params
}

// executeFeedGroupListGroupsAllPages fetches all pages and merges both the live
// (groups) and soft-deleted (deleted_groups) lists into a single response. It
// merges each array independently so neither list loses its later pages.
func executeFeedGroupListGroupsAllPages(rt *common.RuntimeContext) error {
	maxPages := rt.Int("page-limit")
	if maxPages < 1 {
		maxPages = 20
	}
	if maxPages > 1000 {
		maxPages = 1000
	}

	// Use make([]any, 0) so empty arrays serialize as [] not null.
	allGroups := make([]any, 0)
	allDeletedGroups := make([]any, 0)
	var lastHasMore bool
	var lastPageToken string
	prevPageToken := "__START__"

	for page := 0; page < maxPages; page++ {
		// page_token is always sent (empty on the first page) — the groups
		// endpoint rejects requests that omit it.
		params := larkcore.QueryParams{
			"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
			"page_token": []string{""},
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

		data, err := rt.DoAPIJSON("GET", feedGroupListPath, params, nil)
		if err != nil {
			return err
		}

		if v, ok := data["groups"].([]any); ok {
			allGroups = append(allGroups, v...)
		}
		if v, ok := data["deleted_groups"].([]any); ok {
			allDeletedGroups = append(allDeletedGroups, v...)
		}

		lastHasMore, _ = data["has_more"].(bool)
		lastPageToken, _ = data["page_token"].(string)

		fmt.Fprintf(rt.IO().ErrOut, "page %d: %d groups, %d deleted\n",
			page+1, len(allGroups), len(allDeletedGroups))

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
		"groups":         allGroups,
		"deleted_groups": allDeletedGroups,
		"has_more":       lastHasMore,
		"page_token":     lastPageToken,
	}

	rt.OutFormat(merged, nil, func(w io.Writer) {
		renderFeedGroupsTable(w, merged, lastHasMore)
	})
	return nil
}

// renderFeedGroupsTable prints the active groups[] as a table (group_id / name /
// type), followed by a summary line. When hasMore is true a pagination hint is
// appended; when there are deleted groups their count is noted.
func renderFeedGroupsTable(w io.Writer, data map[string]any, hasMore bool) {
	groups, _ := data["groups"].([]any)
	rows := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		m, _ := g.(map[string]any)
		if m == nil {
			continue
		}
		id, _ := m["group_id"].(string)
		name, _ := m["name"].(string)
		typ, _ := m["type"].(string)
		rows = append(rows, map[string]interface{}{
			"group_id": id,
			"name":     name,
			"type":     typ,
		})
	}
	output.PrintTable(w, rows)

	moreHint := ""
	if hasMore {
		moreHint = " (more available, use --page-token to fetch next page)"
	}
	fmt.Fprintf(w, "\n%d group(s)%s\n", len(groups), moreHint)

	if deleted, _ := data["deleted_groups"].([]any); len(deleted) > 0 {
		fmt.Fprintf(w, "(%d deleted)\n", len(deleted))
	}
}
