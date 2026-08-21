// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// ImFeedShortcutList provides the +feed-shortcut-list shortcut for listing
// the user's feed shortcuts. Pagination tokens are version-locked: automatic
// pagination forwards each server-issued token exactly once and reports an
// incomplete read if the list changes or the token cannot advance.
var ImFeedShortcutList = common.Shortcut{
	Service:               "im",
	Command:               "+feed-shortcut-list",
	Description:           "List the user's feed shortcuts; user-only; supports explicit full pagination and auto-enriches each entry with the full per-type info object under `detail` (pass --no-detail to skip)",
	Risk:                  "read",
	UserScopes:            []string{feedShortcutReadScope},
	ConditionalUserScopes: []string{chatBatchQueryScope},
	AuthTypes:             []string{"user"},
	HasFormat:             true,
	Flags: append([]common.Flag{
		{Name: "page-token",
			Desc: "opaque pagination token from the previous response; omit for the first page. If a token is rejected because the list changed, restart by omitting it."},
		{Name: "no-detail", Type: "bool",
			Desc: "skip fetching the full info object for each shortcut (default: enrichment enabled — CHAT-type entries call im.chats.batch_query, require im:chat:read, and attach the object under the detail field)"},
	}, imPaginationFlags(imReadDefaultPageLimit)...),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateIMPagination(runtime)
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
		pages, status, pageErr := collectIMPages(runtime, runtime.Bool("page-all"), func(pageToken string) (map[string]any, error) {
			return runtime.DoAPIJSONTyped("GET", "/open-apis/im/v2/feed_shortcuts",
				feedShortcutListQuery(pageToken), nil)
		})
		if len(pages) == 0 {
			return pageErr
		}

		runtime.RecordPagination(status)
		data := mergeIMPageArrays(pages, "shortcuts")
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
		presentation := any(data)
		if runtime.JqExpr == "" && runtime.Format != "" &&
			runtime.Format != "json" && runtime.Format != "pretty" {
			presentation = data["shortcuts"]
		}
		runtime.OutFormat(presentation, nil, func(w io.Writer) {
			renderFeedShortcutListPretty(w, data)
		})
		return nil
	},
}

func renderFeedShortcutListPretty(w io.Writer, data map[string]any) {
	items, _ := data["shortcuts"].([]any)
	if len(items) == 0 {
		fmt.Fprintln(w, "No feed shortcuts found.")
		return
	}
	output.FormatValue(w, items, output.FormatTable)
	hasMore, _ := data["has_more"].(bool)
	fmt.Fprintf(w, "\n%d feed shortcut(s)", len(items))
	if hasMore {
		fmt.Fprint(w, " (more available)")
	}
	fmt.Fprintln(w)
}

// feedShortcutListQuery omits the page_token key entirely when the token is
// empty, so the server treats the call as a first-page request.
func feedShortcutListQuery(token string) larkcore.QueryParams {
	if token == "" {
		return larkcore.QueryParams{}
	}
	return larkcore.QueryParams{"page_token": []string{token}}
}
