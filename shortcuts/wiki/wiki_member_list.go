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
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	wikiMemberListDefaultPageSize = 50
	wikiMemberListMaxPageSize     = 50
)

// WikiMemberList lists the members of a wiki space. Pagination follows the
// same conventions as +space-list / +node-list (single page by default,
// --page-all to walk every page, --page-token for explicit cursor resume).
var WikiMemberList = common.Shortcut{
	Service:     "wiki",
	Command:     "+member-list",
	Description: "List members of a wiki space",
	Risk:        "read",
	// Same exact-match-scope rationale as +space-list: declare the narrowest
	// scope the API takes so tokens carrying only wiki:member:retrieve are
	// accepted.
	Scopes:    []string{"wiki:member:retrieve"},
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "space-id", Desc: "wiki space ID; use my_library for the personal document library (user only)", Required: true},
		{Name: "page-size", Type: "int", Default: strconv.Itoa(wikiMemberListDefaultPageSize), Desc: fmt.Sprintf("page size, 1-%d", wikiMemberListMaxPageSize)},
		{Name: "page-token", Desc: "page token; implies single-page fetch (no auto-pagination)"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (capped by --page-limit)"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages to fetch with --page-all (default 10, 0 = unlimited)"},
	},
	Tips: []string{
		"Default fetches a single page; pass --page-all to walk every page.",
		"--space-id my_library is a per-user alias and is only valid with --as user.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateWikiMemberSpaceID(runtime, strings.TrimSpace(runtime.Str("space-id"))); err != nil {
			return err
		}
		return validateWikiListPagination(runtime, wikiMemberListMaxPageSize)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		params := map[string]interface{}{"page_size": runtime.Int("page-size")}
		if pt := strings.TrimSpace(runtime.Str("page-token")); pt != "" {
			params["page_token"] = pt
		}
		dry := common.NewDryRunAPI()
		if wikiListShouldAutoPaginate(runtime) {
			dry.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
		}
		if spaceID == wikiMyLibrarySpaceID {
			return dry.
				Desc("2-step orchestration: resolve my_library -> list members").
				GET("/open-apis/wiki/v2/spaces/my_library").
				Desc("[1] Resolve my_library space ID").
				GET(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/members", "<resolved_space_id>")).
				Desc("[2] List wiki space members").
				Params(params)
		}
		return dry.
			GET(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/members", validate.EncodePathSegment(spaceID))).
			Params(params)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		warnIfConflictingPagingFlags(runtime)

		spaceID, err := resolveWikiMemberSpaceID(runtime, strings.TrimSpace(runtime.Str("space-id")))
		if err != nil {
			return err
		}

		members, hasMore, nextToken, err := fetchWikiMembers(runtime, spaceID)
		if err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Found %d wiki space member(s)\n", len(members))

		outData := map[string]interface{}{
			"space_id":   spaceID,
			"members":    members,
			"has_more":   hasMore,
			"page_token": nextToken,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(members)}, func(w io.Writer) {
			renderWikiMembersPretty(w, spaceID, members, hasMore, nextToken)
		})
		return nil
	},
}

// fetchWikiMembers honours the four pagination flags, matching +space-list /
// +node-list behavior so the three list shortcuts feel uniform.
func fetchWikiMembers(runtime *common.RuntimeContext, spaceID string) ([]map[string]interface{}, bool, string, error) {
	pageSize := runtime.Int("page-size")
	startToken := strings.TrimSpace(runtime.Str("page-token"))
	auto := wikiListShouldAutoPaginate(runtime)
	pageLimit := runtime.Int("page-limit")

	apiPath := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/members", validate.EncodePathSegment(spaceID))

	var (
		members       = make([]map[string]interface{}, 0)
		pageToken     = startToken
		lastHasMore   bool
		lastPageToken string
	)
	for page := 0; ; page++ {
		params := map[string]interface{}{"page_size": pageSize}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPI("GET", apiPath, params, nil)
		if err != nil {
			return nil, false, "", err
		}
		items, _ := data["members"].([]interface{})
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				members = append(members, wikiMemberRecord(m))
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
		if lastPageToken == pageToken {
			// Guard against a buggy server echoing the same cursor with
			// has_more=true: without --page-limit we would loop forever.
			fmt.Fprintf(runtime.IO().ErrOut, "Stopping pagination: server returned a non-advancing page_token.\n")
			break
		}
		if pageLimit > 0 && page+1 >= pageLimit {
			break
		}
		pageToken = lastPageToken
	}
	return members, lastHasMore, lastPageToken, nil
}

func renderWikiMembersPretty(w io.Writer, spaceID string, members []map[string]interface{}, hasMore bool, pageToken string) {
	fmt.Fprintf(w, "Wiki space: %s\n", spaceID)
	if len(members) == 0 {
		// Distinguish "nothing here" from "current page empty but server says
		// more pages follow" — the latter is a hint to keep paginating.
		if hasMore && pageToken != "" {
			fmt.Fprintln(w, "Current page is empty but the server reports more pages.")
			fmt.Fprintln(w, "Pass --page-all to walk every page, or --page-token to resume from the cursor below:")
			fmt.Fprintf(w, "  next page_token: %s\n", pageToken)
			return
		}
		fmt.Fprintln(w, "No wiki space members found.")
		return
	}
	for i, m := range members {
		fmt.Fprintf(w, "[%d] %s\n", i+1, valueOrDash(m["member_id"]))
		fmt.Fprintf(w, "    member_type: %s\n", valueOrDash(m["member_type"]))
		fmt.Fprintf(w, "    member_role: %s\n", valueOrDash(m["member_role"]))
		if t, _ := m["type"].(string); t != "" {
			fmt.Fprintf(w, "    type:        %s\n", t)
		}
		fmt.Fprintln(w)
	}
	if hasMore && pageToken != "" {
		fmt.Fprintf(w, "Next page token: %s\n", pageToken)
	}
}
