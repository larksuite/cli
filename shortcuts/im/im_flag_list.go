// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// ImFlagList provides the +flag-list shortcut for listing bookmarks.
// Feed-type thread entries are auto-enriched with message content.
var ImFlagList = common.Shortcut{
	Service:     "im",
	Command:     "+flag-list",
	Description: "List bookmarks; user-only; auto-enriches feed-type thread entries with message content; supports `--page-all` auto-pagination",
	Risk:        "read",
	UserScopes:  []string{flagReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: "50", Desc: "page size (1-50)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max pages when auto-pagination is enabled (default 20, max 1000)"},
		{Name: "enrich-feed-thread", Type: "bool", Default: "true", Desc: "fetch message content for feed-type thread entries (default true; may call messages/mget and require im:message.group_msg:get_as_user/im:message.p2p_msg:get_as_user; use --enrich-feed-thread=false to avoid extra scopes)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateListOptions(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if err := validateListOptions(runtime); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		d := common.NewDryRunAPI().
			GET("/open-apis/im/v1/flags").
			Params(map[string]any{
				"page_size":  strconv.Itoa(runtime.Int("page-size")),
				"page_token": runtime.Str("page-token"),
			})
		if runtime.Bool("enrich-feed-thread") {
			d.Desc("conditional enrichment: if feed/thread flag items are missing message content, execution may also call GET /open-apis/im/v1/messages/mget and requires scopes im:message.group_msg:get_as_user im:message.p2p_msg:get_as_user; pass --enrich-feed-thread=false to skip this extra call and extra scopes")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// When --page-token is explicitly provided, the user wants a specific page —
		// no auto-pagination regardless of --page-all.
		if runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token") {
			return executeListAllPages(runtime)
		}

		data, err := runtime.DoAPIJSON("GET", "/open-apis/im/v1/flags", listQuery(runtime), nil)
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

func validateListOptions(rt *common.RuntimeContext) error {
	if n := rt.Int("page-size"); n < 1 || n > 50 {
		return output.ErrValidation("--page-size must be an integer between 1 and 50")
	}
	if n := rt.Int("page-limit"); n < 1 || n > 1000 {
		return output.ErrValidation("--page-limit must be an integer between 1 and 1000")
	}
	return nil
}

// listQuery builds the query parameters for the flag list API call.
// page_token is required by the server even on the first page — pass empty
// string when the user hasn't supplied one.
func listQuery(rt *common.RuntimeContext) larkcore.QueryParams {
	return larkcore.QueryParams{
		"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
		"page_token": []string{rt.Str("page-token")},
	}
}

// enrichFeedThreadItems attaches message body to feed-shape thread entries
// by calling messages/mget. The list API returns only IDs for feed-shape entries,
// so this enrichment is needed to provide full message content.
//
// NOTE: This function modifies data["flag_items"] in place by adding a "message" key
// to each feed-thread entry.
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
		if err := checkFlagRequiredScopes(rt.Ctx(), rt, flagMessageReadScopes); err != nil {
			return err
		}
		ids := make([]string, 0, len(need))
		for id := range need {
			ids = append(ids, id)
		}
		// /messages/mget accepts max 50 IDs per request — batch if needed.
		const mgetBatchSize = 50
		for i := 0; i < len(ids); i += mgetBatchSize {
			end := i + mgetBatchSize
			if end > len(ids) {
				end = len(ids)
			}
			batch := ids[i:end]
			got, err := rt.DoAPIJSON("GET", "/open-apis/im/v1/messages/mget",
				larkcore.QueryParams{"message_ids": batch}, nil)
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

// asString converts an arbitrary value to its string representation.
// Handles string, float64, int, int64, and json.Number types; returns empty string for other types.
func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return x.String()
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
	if maxPages > 1000 {
		maxPages = 1000
	}

	// Use make([]any, 0) to ensure empty arrays serialize as [] not null
	allFlagItems := make([]any, 0)
	allDeleteFlagItems := make([]any, 0)
	allMessages := make([]any, 0)
	var lastHasMore bool
	var lastPageToken string
	prevPageToken := "__START__" // Sentinel to detect unchanged token

	for page := 0; page < maxPages; page++ {
		token := ""
		if page > 0 {
			token = lastPageToken
		}
		data, err := rt.DoAPIJSON("GET", "/open-apis/im/v1/flags",
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

		// Progress output to stderr
		fmt.Fprintf(rt.IO().ErrOut, "page %d: %d flags, %d deleted\n",
			page+1, len(allFlagItems), len(allDeleteFlagItems))

		if !lastHasMore || lastPageToken == "" {
			break
		}
		// Detect server anomaly: same token returned twice means infinite loop
		if lastPageToken == prevPageToken {
			fmt.Fprintf(rt.IO().ErrOut, "warning: page_token did not change, stopping pagination to avoid infinite loop\n")
			break
		}
		prevPageToken = lastPageToken
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
