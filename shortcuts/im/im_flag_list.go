// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	flagListDefaultPageSize = 50
	// GET /open-apis/im/v1/flags accepts page_size up to 50.
	flagListMaxPageSize = 50
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
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", flagListDefaultPageSize), Desc: fmt.Sprintf("page size (1-%d)", flagListMaxPageSize)},
		{Name: "page-token", Desc: "starting pagination cursor"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate, capped by --page-limit"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: fmt.Sprintf("max pages with --page-all (default 20, max %d; 0 = unlimited)", imReadMaxPageLimit)},
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
		return executeListAllPages(runtime)
	},
}

func validateListOptions(rt *common.RuntimeContext) error {
	if _, err := common.ValidatePageSizeTyped(rt, "page-size", flagListDefaultPageSize, 1, flagListMaxPageSize); err != nil {
		return err
	}
	return validateIMPagination(rt)
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
			got, err := rt.DoAPIJSONTyped("GET", "/open-apis/im/v1/messages/mget",
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
	pages, status, pageErr := collectIMPages(rt, rt.Bool("page-all"), func(pageToken string) (map[string]any, error) {
		return rt.DoAPIJSONTyped("GET", "/open-apis/im/v1/flags",
			larkcore.QueryParams{
				"page_size":  []string{strconv.Itoa(rt.Int("page-size"))},
				"page_token": []string{pageToken},
			}, nil)
	})
	if len(pages) == 0 {
		return pageErr
	}

	rt.RecordPagination(status)
	merged := mergeIMPageArrays(pages, "flag_items", "delete_flag_items", "messages")
	if rt.Bool("enrich-feed-thread") {
		if err := enrichFeedThreadItems(rt, merged); err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: feed-thread enrichment failed: %v\n", err)
		}
	}

	presentation := any(merged)
	if rt.JqExpr == "" && rt.Format != "" && rt.Format != "json" && rt.Format != "pretty" {
		presentation = flagListFormatRows(merged)
	}
	rt.OutFormat(presentation, nil, func(w io.Writer) {
		renderFlagListPretty(w, merged)
	})
	return nil
}

func flagListFormatRows(data map[string]any) []any {
	rows := make([]any, 0)
	appendRows := func(raw any, state string) {
		items, _ := raw.([]any)
		for _, item := range items {
			source, _ := item.(map[string]any)
			if source == nil {
				continue
			}
			row := make(map[string]any, len(source)+1)
			for key, value := range source {
				row[key] = value
			}
			row["list_state"] = state
			rows = append(rows, row)
		}
	}
	appendRows(data["flag_items"], "active")
	appendRows(data["delete_flag_items"], "deleted")
	return rows
}

func renderFlagListPretty(w io.Writer, data map[string]any) {
	rows := flagListFormatRows(data)
	if len(rows) == 0 {
		fmt.Fprintln(w, "No bookmarks found.")
		return
	}
	output.FormatValue(w, rows, output.FormatTable)
	active, _ := data["flag_items"].([]any)
	deleted, _ := data["delete_flag_items"].([]any)
	fmt.Fprintf(w, "\n%d active bookmark(s), %d deleted bookmark(s)\n", len(active), len(deleted))
}
