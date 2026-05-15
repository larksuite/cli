// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	wikiSpaceListAPIPath         = "/open-apis/wiki/v2/spaces"
	wikiSpaceListDefaultPageSize = 50
	wikiSpaceListMaxPageSize     = 50
)

// WikiSpaceList lists all wiki spaces the caller has access to.
var WikiSpaceList = common.Shortcut{
	Service:     "wiki",
	Command:     "+space-list",
	Description: "List wiki spaces accessible to the caller",
	Risk:        "read",
	// Declare the narrowest valid scope: the upstream API accepts any of
	// wiki:wiki / wiki:wiki:readonly / wiki:space:retrieve, but the
	// framework's preflight does exact-string scope matching (see
	// internal/auth/scope.go), so picking the broad readonly form would
	// wrongly reject tokens that only carry the narrow retrieve scope and
	// hand them a misleading missing-scope hint.
	Scopes:    []string{"wiki:space:retrieve"},
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: strconv.Itoa(wikiSpaceListDefaultPageSize), Desc: fmt.Sprintf("page size, 1-%d", wikiSpaceListMaxPageSize)},
		{Name: "page-token", Desc: "page token; implies single-page fetch (no auto-pagination)"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (capped by --page-limit)"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages to fetch with --page-all (default 10, 0 = unlimited)"},
	},
	Tips: []string{
		"Default fetches a single page (matches other list shortcuts in this CLI); pass --page-all to pull every page.",
		"The underlying API never returns the my_library personal library; resolve it via `wiki spaces get --params '{\"space_id\":\"my_library\"}'`.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateWikiListPagination(runtime, wikiSpaceListMaxPageSize)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{"page_size": runtime.Int("page-size")}
		if pt := strings.TrimSpace(runtime.Str("page-token")); pt != "" {
			params["page_token"] = pt
		}
		dry := common.NewDryRunAPI()
		// Auto-pagination is the default — make it explicit in the dry-run so
		// callers can see whether the loop will fire.
		if wikiListShouldAutoPaginate(runtime) {
			dry.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
		}
		return dry.GET(wikiSpaceListAPIPath).Params(params)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		warnIfConflictingPagingFlags(runtime)
		spaces, hasMore, nextToken, err := fetchWikiSpaces(runtime)
		if err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Found %d wiki space(s)\n", len(spaces))
		outData := map[string]interface{}{
			"spaces":     spaces,
			"has_more":   hasMore,
			"page_token": nextToken,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(spaces)}, func(w io.Writer) {
			renderWikiSpacesPretty(w, spaces, hasMore, nextToken)
		})
		return nil
	},
}

// fetchWikiSpaces honours the four pagination flags:
//   - default (no --page-all, no --page-token): fetch a single page from the start
//   - --page-token X: fetch a single page starting at X (auto-pagination disabled)
//   - --page-all: pull subsequent pages, capped by --page-limit (default 10; 0 = unlimited)
//
// The returned slice is always non-nil so json output stays as `[]` instead of `null`.
func fetchWikiSpaces(runtime *common.RuntimeContext) ([]map[string]interface{}, bool, string, error) {
	pageSize := runtime.Int("page-size")
	startToken := strings.TrimSpace(runtime.Str("page-token"))
	auto := wikiListShouldAutoPaginate(runtime)
	pageLimit := runtime.Int("page-limit")

	var (
		spaces        = make([]map[string]interface{}, 0)
		pageToken     = startToken
		lastHasMore   bool
		lastPageToken string
	)
	for page := 0; ; page++ {
		params := map[string]interface{}{"page_size": pageSize}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPI("GET", wikiSpaceListAPIPath, params, nil)
		if err != nil {
			return nil, false, "", err
		}
		items, _ := data["items"].([]interface{})
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				spaces = append(spaces, parseWikiSpaceItem(m))
			}
		}
		lastHasMore, _ = data["has_more"].(bool)
		lastPageToken, _ = data["page_token"].(string)
		if !auto {
			break
		}
		if !lastHasMore || lastPageToken == "" {
			break
		}
		if pageLimit > 0 && page+1 >= pageLimit {
			break
		}
		pageToken = lastPageToken
	}
	return spaces, lastHasMore, lastPageToken, nil
}

func parseWikiSpaceItem(m map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"space_id":     common.GetString(m, "space_id"),
		"name":         common.GetString(m, "name"),
		"description":  common.GetString(m, "description"),
		"space_type":   common.GetString(m, "space_type"),
		"visibility":   common.GetString(m, "visibility"),
		"open_sharing": common.GetString(m, "open_sharing"),
	}
}

func renderWikiSpacesPretty(w io.Writer, spaces []map[string]interface{}, hasMore bool, pageToken string) {
	if len(spaces) == 0 {
		// Distinguish "nothing here" from "current page empty but server says
		// more pages follow" — the latter is a hint to keep paginating instead
		// of giving up.
		if hasMore && pageToken != "" {
			fmt.Fprintln(w, "Current page is empty but the server reports more pages.")
			fmt.Fprintln(w, "Pass --page-all to walk every page, or --page-token to resume from the cursor below:")
			fmt.Fprintf(w, "  next page_token: %s\n", pageToken)
			return
		}
		fmt.Fprintln(w, "No wiki spaces found.")
		return
	}
	for i, s := range spaces {
		fmt.Fprintf(w, "[%d] %s\n", i+1, valueOrDash(s["name"]))
		fmt.Fprintf(w, "    space_id:     %s\n", valueOrDash(s["space_id"]))
		fmt.Fprintf(w, "    space_type:   %s\n", valueOrDash(s["space_type"]))
		fmt.Fprintf(w, "    visibility:   %s\n", valueOrDash(s["visibility"]))
		fmt.Fprintf(w, "    open_sharing: %s\n", valueOrDash(s["open_sharing"]))
		if desc, _ := s["description"].(string); desc != "" {
			fmt.Fprintf(w, "    description:  %s\n", desc)
		}
		fmt.Fprintln(w)
	}
	if hasMore && pageToken != "" {
		fmt.Fprintf(w, "Next page token: %s\n", pageToken)
	}
}

func valueOrDash(v interface{}) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return "-"
}

// validateWikiListPagination performs flag-level validation shared by
// +space-list and +node-list.
func validateWikiListPagination(runtime *common.RuntimeContext, maxPageSize int) error {
	if n := runtime.Int("page-size"); n < 1 || n > maxPageSize {
		return common.FlagErrorf("--page-size must be between 1 and %d", maxPageSize)
	}
	if n := runtime.Int("page-limit"); n < 0 {
		return common.FlagErrorf("--page-limit must be a non-negative integer")
	}
	return nil
}

// wikiListShouldAutoPaginate reports whether the fetch loop should keep
// requesting additional pages. An explicit --page-token disables auto loop
// because the caller has supplied a specific cursor.
func wikiListShouldAutoPaginate(runtime *common.RuntimeContext) bool {
	if strings.TrimSpace(runtime.Str("page-token")) != "" {
		return false
	}
	return runtime.Bool("page-all")
}

// warnIfConflictingPagingFlags logs a notice when --page-token and --page-all
// are both set. --page-token wins (single-page fetch from the supplied cursor)
// and --page-all is silently ignored, which would otherwise look like a bug to
// callers expecting subsequent pages to be drained.
func warnIfConflictingPagingFlags(runtime *common.RuntimeContext) {
	if strings.TrimSpace(runtime.Str("page-token")) != "" && runtime.Bool("page-all") {
		fmt.Fprintln(runtime.IO().ErrOut,
			"warning: --page-token is set, so --page-all is ignored (single-page fetch from the supplied cursor)")
	}
}
