// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"strconv"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	imReadDefaultPageLimit = 20
	imReadMaxPageLimit     = 1000
	imReadMaxPageDelay     = 60_000
)

type imPageFetcher func(pageToken string) (map[string]any, error)

var imPageAllPolicy = common.PageAllPolicy{AllowUnlimited: true}

func imPaginationFlags(defaultLimit int) []common.Flag {
	if defaultLimit < 0 {
		defaultLimit = imReadDefaultPageLimit
	}
	return []common.Flag{
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages"},
		{Name: "page-limit", Type: "int", Default: strconv.Itoa(defaultLimit), Desc: "maximum pages fetched with --page-all (0 = unlimited)"},
	}
}

func validateIMPagination(runtime *common.RuntimeContext) error {
	if limit := runtime.Int("page-limit"); limit < 0 || limit > imReadMaxPageLimit {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--page-limit must be an integer between 0 and %d (0 = unlimited)",
			imReadMaxPageLimit,
		).WithParam("--page-limit")
	}
	if runtime.Cmd.Flags().Lookup("page-delay") != nil {
		delay := runtime.Int("page-delay")
		if delay < 0 || delay > imReadMaxPageDelay {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--page-delay must be an integer between 0 and %d",
				imReadMaxPageDelay,
			).WithParam("--page-delay")
		}
	}
	return nil
}

// collectIMPages adapts IM flags and truncation evidence to the shared
// callback walker. The IM layer owns no pagination loop or cursor policy.
func collectIMPages(runtime *common.RuntimeContext, autoPaginate bool, fetch imPageFetcher) ([]map[string]any, client.PaginationStatus, error) {
	pageDelay := 0
	if runtime.Cmd.Flags().Lookup("page-delay") != nil {
		pageDelay = runtime.Int("page-delay")
	}
	pages := make([]map[string]any, 0, 1)
	status, err := common.WalkPages(runtime, common.PageWalkOptions{
		AutoPaginate:   autoPaginate,
		PageLimit:      runtime.Int("page-limit"),
		AllowUnlimited: imPageAllPolicy.AllowUnlimited,
		PageDelay:      time.Duration(pageDelay) * time.Millisecond,
		StartPageToken: runtime.Str("page-token"),
		IsTruncated:    explicitlyTruncated,
	}, common.PageFetcher(fetch), func(page map[string]any, _ int) error {
		pages = append(pages, page)
		return nil
	})
	return pages, status, err
}

func explicitlyTruncated(page map[string]any) bool {
	if truncated, _ := page["truncated"].(bool); truncated {
		return true
	}
	switch truncations := page["truncations"].(type) {
	case []any:
		return len(truncations) > 0
	case []map[string]any:
		return len(truncations) > 0
	default:
		return false
	}
}

// mergeIMPageArrays preserves the first page's non-pagination metadata, merges
// every named array bucket, and carries the final page's cursor state.
func mergeIMPageArrays(pages []map[string]any, arrayFields ...string) map[string]any {
	merged := make(map[string]any)
	if len(pages) == 0 {
		for _, field := range arrayFields {
			merged[field] = []any{}
		}
		return merged
	}
	for key, value := range pages[0] {
		merged[key] = value
	}
	for _, field := range []string{"has_more", "page_token", "next_page_token"} {
		delete(merged, field)
	}
	for _, field := range arrayFields {
		items := make([]any, 0)
		for _, page := range pages {
			if pageItems, ok := page[field].([]any); ok {
				items = append(items, pageItems...)
			}
		}
		merged[field] = items
	}
	last := pages[len(pages)-1]
	if value, ok := last["has_more"]; ok {
		merged["has_more"] = value
	}
	if value, ok := last["page_token"]; ok {
		merged["page_token"] = value
	}
	if value, ok := last["next_page_token"]; ok {
		merged["next_page_token"] = value
	}
	return merged
}

func cloneQueryParams(source map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(source))
	for key, values := range source {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
