// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var ImFlagList = common.Shortcut{
	Service: "im",
	Command: "+flag-list",
	Description: "List bookmarks (标记). Feed-type thread entries are auto-enriched with message " +
		"content because the API returns only ids for that shape",
	Risk:       "read",
	UserScopes: []string{"im:feed.flag:read", "im:message:readonly"},
	AuthTypes:  []string{"user"},
	HasFormat:  true,
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: "50", Desc: "page size (1-50)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max pages when auto-pagination is enabled (default 20, max 40)"},
		{Name: "enrich-feed-thread", Type: "bool", Default: "true", Desc: "fetch message content for feed-type thread entries (default true); skipped when the server already returned `messages` in the response"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if n := runtime.Int("page-size"); n < 1 || n > 50 {
			return output.ErrValidation("--page-size must be an integer between 1 and 50")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/im/v1/flags").
			Params(map[string]any{
				"page_size":  strconv.Itoa(runtime.Int("page-size")),
				"page_token": runtime.Str("page-token"),
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// When --page-token is explicitly provided, the user wants a specific page —
		// no auto-pagination regardless of --page-all.
		if runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token") {
			return executeListAllPages(runtime)
		}

		data, err := runtime.DoAPIJSON(http.MethodGet, "/open-apis/im/v1/flags", listQuery(runtime), nil)
		if err != nil {
			return err
		}
		if runtime.Bool("enrich-feed-thread") {
			if err := enrichFeedThreadItems(runtime, data); err != nil {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: feed-thread enrichment failed: %v\n", err)
			}
		}
		runtime.Out(data, nil)
		return nil
	},
}

func listQuery(rt *common.RuntimeContext) larkcore.QueryParams {
	// page_token is required by the server even on the first page — pass empty
	// string when the user hasn't supplied one.
	return larkcore.QueryParams{
		"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
		"page_token": []string{rt.Str("page-token")},
	}
}

// enrichFeedThreadItems attaches message body to feed-shape thread entries
// by calling messages/mget. The list API returns only IDs for feed-shape entries,
// so this enrichment is needed to provide full message content.
func enrichFeedThreadItems(rt *common.RuntimeContext, data map[string]any) error {
	// Only enrich active flags (flag_items), not canceled flags (delete_flag_items).
	// Canceled message-type flags don't show message content, so thread-type flags don't need it either.
	items, _ := data["flag_items"].([]any)
	if len(items) == 0 {
		return nil
	}

	// Index any messages the server already returned — saves a mget round-trip
	// (ItemType=default+FlagType=Message responses already carry the message body).
	byID := make(map[string]map[string]any)
	if inline, ok := data["messages"].([]any); ok {
		for _, m := range inline {
			mm, _ := m.(map[string]any)
			if mm == nil {
				continue
			}
			if id := asString(mm["message_id"]); id != "" {
				byID[id] = mm
			}
		}
	}

	// Collect feed-thread ids whose message body wasn't inlined — dedup to cut mget calls.
	need := map[string]bool{}
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		ft := asString(m["flag_type"])
		itStr := asString(m["item_type"])
		if ft != strconv.Itoa(int(FlagTypeFeed)) {
			continue
		}
		if itStr != strconv.Itoa(int(ItemTypeThread)) && itStr != strconv.Itoa(int(ItemTypeMsgThread)) {
			continue
		}
		id := asString(m["item_id"])
		if id == "" {
			continue
		}
		if _, inlined := byID[id]; !inlined {
			need[id] = true
		}
	}

	if len(need) > 0 {
		ids := make([]string, 0, len(need))
		for id := range need {
			ids = append(ids, id)
		}
		// /messages/mget accepts repeated `message_ids` params — larkcore.QueryParams
		// serializes each slice value as a separate query pair.
		got, err := rt.DoAPIJSON(http.MethodGet, "/open-apis/im/v1/messages/mget",
			larkcore.QueryParams{"message_ids": ids}, nil)
		if err != nil {
			return err
		}
		fetched, _ := got["items"].([]any)
		for _, m := range fetched {
			mm, _ := m.(map[string]any)
			if mm == nil {
				continue
			}
			if id := asString(mm["message_id"]); id != "" {
				byID[id] = mm
			}
		}
	}

	if len(byID) == 0 {
		return nil
	}
	// Attach message payload to the matching list entries.
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		ft := asString(m["flag_type"])
		itType := asString(m["item_type"])
		if ft != strconv.Itoa(int(FlagTypeFeed)) {
			continue
		}
		if itType != strconv.Itoa(int(ItemTypeThread)) && itType != strconv.Itoa(int(ItemTypeMsgThread)) {
			continue
		}
		if msg, ok := byID[asString(m["item_id"])]; ok {
			m["message"] = msg
		}
	}
	return nil
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	}
	return ""
}

// executeListAllPages fetches all pages and merges the results into a single response.
// The flag list API returns items sorted by update_time ascending, so the last page
// contains the newest items.
func executeListAllPages(rt *common.RuntimeContext) error {
	maxPages := rt.Int("page-limit")
	if maxPages < 1 {
		maxPages = 20
	}
	if maxPages > 40 {
		maxPages = 40
	}

	var allFlagItems, allDeleteFlagItems, allMessages []any
	var lastHasMore bool
	var lastPageToken string

	for page := 0; page < maxPages; page++ {
		token := ""
		if page > 0 {
			token = lastPageToken
		}
		data, err := rt.DoAPIJSON(http.MethodGet, "/open-apis/im/v1/flags",
			larkcore.QueryParams{
				"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
				"page_token": []string{token},
			}, nil)
		if err != nil {
			return err
		}

		if v, ok := data["flag_items"].([]any); ok {
			allFlagItems = append(allFlagItems, v...)
		}
		if v, ok := data["delete_flag_items"].([]any); ok {
			allDeleteFlagItems = append(allDeleteFlagItems, v...)
		}
		if v, ok := data["messages"].([]any); ok {
			allMessages = append(allMessages, v...)
		}

		lastHasMore, _ = data["has_more"].(bool)
		lastPageToken, _ = data["page_token"].(string)

		if !lastHasMore || lastPageToken == "" {
			break
		}
	}

	merged := map[string]any{
		"flag_items":        allFlagItems,
		"delete_flag_items": allDeleteFlagItems,
		"messages":          allMessages,
		"has_more":          lastHasMore,
		"page_token":        lastPageToken,
	}

	if rt.Bool("enrich-feed-thread") {
		if err := enrichFeedThreadItems(rt, merged); err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: feed-thread enrichment failed: %v\n", err)
		}
	}

	rt.Out(merged, nil)
	return nil
}
