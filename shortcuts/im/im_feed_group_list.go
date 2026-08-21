// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	feedGroupListPath            = "/open-apis/im/v1/groups"
	feedGroupListDefaultPageSize = 50
	// GET /open-apis/im/v1/groups accepts page_size up to 50.
	feedGroupListMaxPageSize = 50
)

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
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", feedGroupListDefaultPageSize), Desc: fmt.Sprintf("page size (1-%d)", feedGroupListMaxPageSize)},
		{Name: "page-token", Desc: "starting pagination cursor"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: fmt.Sprintf("max pages when auto-pagination is enabled (default 20, max %d; 0 = unlimited)", imReadMaxPageLimit)},
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
		return executeFeedGroupListGroupsAllPages(runtime)
	},
}

func validateFeedGroupListPageOptions(rt *common.RuntimeContext) error {
	if _, err := common.ValidatePageSizeTyped(rt, "page-size", feedGroupListDefaultPageSize, 1, feedGroupListMaxPageSize); err != nil {
		return err
	}
	if v := rt.Str("start-time"); v != "" {
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--start-time must be Unix milliseconds (a decimal integer string)").WithParam("--start-time")
		}
	}
	if v := rt.Str("end-time"); v != "" {
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--end-time must be Unix milliseconds (a decimal integer string)").WithParam("--end-time")
		}
	}
	return validateIMPagination(rt)
}

// feedGroupListGroupsDryRunParams builds query parameters for dry-run display.
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
	pages, status, pageErr := collectIMPages(rt, rt.Bool("page-all"), func(pageToken string) (map[string]any, error) {
		params := larkcore.QueryParams{
			"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
			"page_token": []string{pageToken},
		}
		if start := rt.Str("start-time"); start != "" {
			params["start_time"] = []string{start}
		}
		if end := rt.Str("end-time"); end != "" {
			params["end_time"] = []string{end}
		}

		return rt.DoAPIJSONTyped("GET", feedGroupListPath, params, nil)
	})
	if len(pages) == 0 {
		return pageErr
	}

	rt.RecordPagination(status)
	merged := mergeIMPageArrays(pages, "groups", "deleted_groups")
	lastHasMore, _ := merged["has_more"].(bool)
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
