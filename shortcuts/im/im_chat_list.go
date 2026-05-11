// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// imChatListPath is the upstream HTTP path for the +chat-list shortcut.
const imChatListPath = "/open-apis/im/v1/chats"

// ImChatList is the +chat-list shortcut: wraps GET /open-apis/im/v1/chats to
// list groups the current user/bot is a member of. Supports sort order,
// pagination, and (user identity only) muted-chat filtering via --exclude-muted.
var ImChatList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-list",
	Description: "List groups the current user/bot is a member of; user/bot; supports sorting, pagination, and --exclude-muted (user identity only)",
	Risk:        "read",
	Scopes:      []string{"im:chat:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-id-type", Default: "open_id", Desc: "ID type for owner_id in response", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "sort-type", Default: "ByCreateTimeAsc", Desc: "sort order", Enum: []string{"ByCreateTimeAsc", "ByActiveTimeDesc"}},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "exclude-muted", Type: "bool", Desc: "(user identity only) drop chats the current user has muted (do-not-disturb); bot identity returns all chats unfiltered"},
	},
	// DryRun previews the GET /open-apis/im/v1/chats request without executing.
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(imChatListPath).
			Params(buildChatListParams(runtime))
	},
	// Validate enforces flag preconditions; only --page-size has bounds (1-100).
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if n := runtime.Int("page-size"); n < 1 || n > 100 {
			return output.ErrValidation("--page-size must be an integer between 1 and 100")
		}
		return nil
	},
	// Execute fetches one page of chats, optionally applies --exclude-muted
	// via MaybeApplyMuteFilter, and renders the result. outData["filter"] is
	// populated only when --exclude-muted is set (backward compatible).
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params := buildChatListParams(runtime)
		resData, err := runtime.CallAPI("GET", imChatListPath, params, nil)
		if err != nil {
			return err
		}

		rawItems, _ := resData["items"].([]interface{})
		hasMore, pageToken := common.PaginationMeta(resData)

		var items []map[string]interface{}
		for _, raw := range rawItems {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			items = append(items, item)
		}

		mfOut, err := MaybeApplyMuteFilter(runtime, MuteFilterInput{
			ExcludeMuted: runtime.Bool("exclude-muted"),
			IsBot:        runtime.IsBot(),
			Chats:        items,
			ChatIDKey:    "chat_id",
			HasMore:      hasMore,
		})
		if err != nil {
			return err
		}
		items = mfOut.Chats

		outData := map[string]interface{}{
			"chats":      items,
			"has_more":   hasMore,
			"page_token": pageToken,
		}
		if mfOut.Meta.Applied != "" {
			outData["filter"] = MuteFilterMetaToMap(mfOut.Meta)
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(items) == 0 {
				fmt.Fprintln(w, "No chats found.")
				if mfOut.Meta.Hint != "" {
					fmt.Fprintln(w, mfOut.Meta.Hint)
				}
				return
			}
			rows := make([]map[string]interface{}, 0, len(items))
			for _, m := range items {
				row := map[string]interface{}{
					"chat_id": m["chat_id"],
					"name":    m["name"],
				}
				if desc, _ := m["description"].(string); desc != "" {
					row["description"] = desc
				}
				if ownerID, _ := m["owner_id"].(string); ownerID != "" {
					row["owner_id"] = ownerID
				}
				if external, ok := m["external"].(bool); ok {
					row["external"] = external
				}
				if status, _ := m["chat_status"].(string); status != "" {
					row["chat_status"] = status
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d chat(s) listed", len(rows))
			if hasMore {
				fmt.Fprint(w, " (more available, use --page-token to fetch next page")
				if pageToken != "" {
					fmt.Fprintf(w, ", page_token: %s", pageToken)
				}
				fmt.Fprint(w, ")")
			}
			fmt.Fprintln(w)
			if mfOut.Meta.Hint != "" {
				fmt.Fprintln(w, mfOut.Meta.Hint)
			}
		})
		return nil
	},
}

// buildChatListParams builds the query parameters for the GET /im/v1/chats
// call from the runtime flag values. user_id_type and sort_type are always
// present (their flag defaults are non-empty); page_token is omitted when
// empty; page_size falls back to the API default of 20 when not provided.
func buildChatListParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{
		"user_id_type": runtime.Str("user-id-type"),
		"sort_type":    runtime.Str("sort-type"),
	}
	if n := runtime.Int("page-size"); n > 0 {
		params["page_size"] = n
	} else {
		params["page_size"] = 20
	}
	if pt := runtime.Str("page-token"); pt != "" {
		params["page_token"] = pt
	}
	return params
}
