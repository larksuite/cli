// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// ImFeedShortcutList provides the +feed-shortcut-list shortcut for listing
// the user's feed shortcuts. The server-controlled page size covers the full
// list in practice, but pagination is version-locked: when the list changes
// between calls the server rejects the stale token and the caller has to
// restart by omitting --page-token.
//
// The shortcut is a thin one-page wrapper — there is no automatic walking.
// Callers are expected to drive their own loop when they actually need to
// paginate, because the version-lock means each page is a real checkpoint
// that the caller must consciously decide what to do with on failure.
var ImFeedShortcutList = common.Shortcut{
	Service:               "im",
	Command:               "+feed-shortcut-list",
	Description:           "List one page of the user's feed shortcuts; user-only; first call omits --page-token, subsequent calls pass the previous response's page_token; each entry is auto-enriched with the full per-type info object attached as `detail` (pass --no-detail to skip)",
	Risk:                  "read",
	UserScopes:            []string{feedShortcutReadScope},
	ConditionalUserScopes: []string{chatBatchQueryScope},
	AuthTypes:             []string{"user"},
	HasFormat:             true,
	Flags: []common.Flag{
		{Name: "page-token",
			Desc: "opaque pagination token from the previous response; omit for the first page. If a token is rejected because the list changed, restart by omitting it."},
		{Name: "no-detail", Type: "bool",
			Desc: "skip fetching the full info object for each shortcut (default: enrichment enabled — CHAT-type entries call im.chats.batch_query, require im:chat:read, and attach the object under the detail field)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI().
			GET("/open-apis/im/v2/feed_shortcuts")
		if token := runtime.Str("page-token"); token != "" {
			d.Params(map[string]any{"page_token": token})
		}
		if !runtime.Bool("no-detail") {
			d.Desc("conditional enrichment: if CHAT-type entries exist, execution also calls POST /open-apis/im/v1/chats/batch_query and requires scope im:chat:read; pass --no-detail to skip this extra call and extra scope")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.DoAPIJSONTyped("GET", "/open-apis/im/v2/feed_shortcuts",
			feedShortcutListQuery(runtime.Str("page-token")), nil)
		if err != nil {
			return err
		}
		if !runtime.Bool("no-detail") {
			if err := enrichFeedShortcutDetail(runtime, data); err != nil {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: detail enrichment failed: %v\n", err)
				// Mirror the warning into the data payload so stdout-only
				// consumers can tell "enrichment skipped" from "nothing to
				// enrich" (same convention as mail's data-level _notice).
				if data != nil {
					data["_notice"] = fmt.Sprintf("detail enrichment skipped: %v", err)
				}
			}
		}
		runtime.Out(data, nil)
		return nil
	},
}

// feedShortcutListQuery omits the page_token key entirely when the token is
// empty, so the server treats the call as a first-page request.
func feedShortcutListQuery(token string) larkcore.QueryParams {
	if token == "" {
		return larkcore.QueryParams{}
	}
	return larkcore.QueryParams{"page_token": []string{token}}
}
