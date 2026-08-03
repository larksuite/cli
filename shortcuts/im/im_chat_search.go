// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	chatSearchDefaultPageLimit = 10
	chatSearchMaximumPageLimit = 1000
)

// ImChatSearch is the +chat-search shortcut: wraps POST /open-apis/im/v2/chats/search
// to find visible group chats by keyword and/or member open_ids. Supports
// member/type filters, sort order, pagination, and (user identity only) the
// --exclude-muted client-side mute filter.
var ImChatSearch = common.Shortcut{
	Service:     "im",
	Command:     "+chat-search",
	Description: "Search visible group chats by --query keyword and/or --member-ids; user/bot; e.g. look up chat_id by group name; supports type filters, sorting, auto-pagination, and --exclude-muted (user identity only)",
	Risk:        "read",
	Scopes:      []string{"im:chat:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword (server may return data.notice for overly long input)"},
		{Name: "search-types", Desc: "chat types, comma-separated (private, external, public_joined, public_not_joined)"},
		{Name: "chat-modes", Desc: "filter by chat mode, comma-separated (group, topic)"},
		{Name: "types", Hidden: true, Desc: "compatibility input handled by +chat-search validation; use --chat-modes or --search-types"},
		{Name: "member-ids", Desc: "filter by member open_ids, comma-separated"},
		{Name: "is-manager", Type: "bool", Desc: "only show chats you created or manage"},
		{Name: "disable-search-by-user", Type: "bool", Desc: "disable search-by-member-name (default: search by member name first, then group name)"},
		{Name: "sort", Desc: "sort field (always descending): create_time | update_time | member_count", Enum: []string{"create_time", "update_time", "member_count"}},
		{Name: "sort-by", Hidden: true, Desc: "alias of --sort (hidden)"},
		{Name: "page-size", Type: "int", Default: "20", Desc: imPageSizeDescription("+chat-search")},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate, capped by --page-limit"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages with --page-all (default 10; configurable range 1-1000)"},
		{Name: "exclude-muted", Type: "bool", Desc: "(user identity only) drop chats the current user has muted (do-not-disturb); bot identity returns all chats unfiltered"},
	},
	// DryRun previews the POST /open-apis/im/v2/chats/search request without executing.
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildSearchChatBody(runtime)
		params := buildSearchChatParams(runtime)
		dry := common.NewDryRunAPI()
		if chatSearchShouldAutoPaginate(runtime) {
			dry.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
		}
		return dry.
			POST("/open-apis/im/v2/chats/search").
			Params(params).
			Body(body)
	},
	// Validate enforces query/member-ids presence, search-types
	// enum, --member-ids count and format, and --page-size bounds.
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		query := runtime.Str("query")
		memberIDs := runtime.Str("member-ids")
		if query == "" && memberIDs == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--query and --member-ids cannot both be empty; provide at least one (e.g. --query \"team-name\" or --member-ids \"ou_xxx\")")
		}
		if err := applyChatSearchTypesCompatibility(runtime); err != nil {
			return err
		}
		if err := validateAliasEnum(runtime, "sort-by", "sort", "create_time_desc", "update_time_desc", "member_count_desc"); err != nil {
			return err
		}
		if st := runtime.Str("search-types"); st != "" {
			allowed := map[string]struct{}{
				"private":           {},
				"external":          {},
				"public_joined":     {},
				"public_not_joined": {},
			}
			for _, item := range common.SplitCSV(st) {
				if _, ok := allowed[item]; !ok {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --search-types value %q: expected one of private, external, public_joined, public_not_joined", item).WithParam("--search-types")
				}
			}
		}
		if cm := runtime.Str("chat-modes"); cm != "" {
			for _, mode := range common.SplitCSV(cm) {
				if mode != "group" && mode != "topic" {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --chat-modes value %q: expected one of group, topic", mode).WithParam("--chat-modes")
				}
			}
		}
		if mi := runtime.Str("member-ids"); mi != "" {
			ids := common.SplitCSV(mi)
			if len(ids) > 50 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-ids exceeds the maximum of 50 (got %d)", len(ids)).WithParam("--member-ids")
			}
			for _, id := range ids {
				if _, err := common.ValidateUserIDTyped("--member-ids", id); err != nil {
					return err
				}
			}
		}
		if _, err := validateIMPageSize(runtime, "+chat-search", 20); err != nil {
			return err
		}
		if n := runtime.Int("page-limit"); n < 1 || n > chatSearchMaximumPageLimit {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-limit must be an integer between 1 and 1000").WithParam("--page-limit")
		}
		return nil
	},
	// Execute fetches one or more pages, extracts per-item meta_data, optionally applies
	// the --exclude-muted client-side filter (with a PreSkipReason when
	// --search-types is exactly public_not_joined), and renders the result.
	// outData["filter"] is populated only when --exclude-muted is set.
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildSearchChatBody(runtime)
		params := buildSearchChatParams(runtime)
		var resData map[string]interface{}
		var err error
		if chatSearchShouldAutoPaginate(runtime) {
			resData, err = fetchChatSearchAllPages(runtime, params, body)
		} else {
			resData, err = runtime.CallAPITyped("POST", "/open-apis/im/v2/chats/search", params, body)
		}
		if err != nil {
			return err
		}

		rawItems, _ := resData["items"].([]interface{})
		totalF, _ := util.ToFloat64(resData["total"])
		total := totalF
		hasMore, pageToken := common.PaginationMeta(resData)

		// Extract MetaData from each item
		var items []map[string]interface{}
		for _, raw := range rawItems {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			meta, _ := item["meta_data"].(map[string]interface{})
			if meta == nil {
				continue
			}
			items = append(items, meta)
		}

		preSkipReason := ""
		if runtime.Bool("exclude-muted") {
			preSkipReason = detectAllNonMemberPreSkip(runtime.Str("search-types"))
		}
		mfOut, err := MaybeApplyMuteFilter(runtime, MuteFilterInput{
			ExcludeMuted:  runtime.Bool("exclude-muted"),
			IsBot:         runtime.IsBot(),
			PreSkipReason: preSkipReason,
			Chats:         items,
			ChatIDKey:     "chat_id",
			HasMore:       hasMore,
		})
		if err != nil {
			return err
		}
		items = mfOut.Chats

		outData := map[string]interface{}{
			"chats":      items,
			"total":      int(total),
			"has_more":   hasMore,
			"page_token": pageToken,
		}
		if notice, _ := resData["notice"].(string); notice != "" {
			outData["notice"] = notice
		}
		if mfOut.Meta.Applied != "" {
			outData["filter"] = MuteFilterMetaToMap(mfOut.Meta)
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(items) == 0 {
				fmt.Fprintln(w, "No matching group chats found.")
				if mfOut.Meta.Hint != "" {
					fmt.Fprintln(w, mfOut.Meta.Hint)
				}
				return
			}
			var rows []map[string]interface{}
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
				if chatMode, _ := m["chat_mode"].(string); chatMode != "" {
					row["chat_mode"] = chatMode
				}
				if external, ok := m["external"].(bool); ok {
					row["external"] = external
				}
				if status, _ := m["chat_status"].(string); status != "" {
					row["chat_status"] = status
				}
				if createTime, _ := m["create_time"].(string); createTime != "" {
					row["create_time"] = createTime
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			moreHint := ""
			if hasMore {
				moreHint = " (more available, use --page-token to fetch next page"
				if pageToken != "" {
					moreHint += fmt.Sprintf(", page_token: %s", pageToken)
				}
				moreHint += ")"
			}
			fmt.Fprintf(w, "\n%d chat(s) found%s\n", int(total), moreHint)
			if mfOut.Meta.Hint != "" {
				fmt.Fprintln(w, mfOut.Meta.Hint)
			}
		})
		return nil
	},
}

// applyChatSearchTypesCompatibility accepts the one observed cross-command
// spelling without treating --types as a normal alias. +chat-list and
// +chat-search use different value domains, so the value must be inspected
// before it can be mapped safely. An explicit --chat-modes always wins.
func applyChatSearchTypesCompatibility(runtime *common.RuntimeContext) error {
	if !runtime.Changed("types") || runtime.Changed("chat-modes") {
		return nil
	}

	typesValue := runtime.Str("types")
	types := common.SplitCSV(typesValue)
	for _, chatType := range types {
		if chatType == "p2p" {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--types %q is invalid for im +chat-search: this command only searches group chats and the service does not support p2p; use im +chat-list --types p2p to list p2p chats",
				typesValue,
			).WithParam("--types")
		}
	}

	onlyGroup := len(types) > 0
	for _, chatType := range types {
		if chatType != "group" {
			onlyGroup = false
			break
		}
	}
	if !onlyGroup {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"invalid --types value %q for im +chat-search; use --chat-modes (group|topic) or --search-types (private|external|public_joined|public_not_joined)",
			typesValue,
		).WithParam("--types")
	}

	if err := runtime.Cmd.Flags().Set("chat-modes", "group"); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to map --types to --chat-modes").WithCause(err)
	}
	if runtime.Factory != nil && runtime.Factory.IOStreams != nil && runtime.Factory.IOStreams.ErrOut != nil {
		fmt.Fprintln(runtime.Factory.IOStreams.ErrOut, "note: --types on +chat-search maps to --chat-modes")
	}
	return nil
}

func chatSearchShouldAutoPaginate(runtime *common.RuntimeContext) bool {
	return runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token")
}

func fetchChatSearchAllPages(runtime *common.RuntimeContext, params, body map[string]interface{}) (map[string]interface{}, error) {
	maxPages := runtime.Int("page-limit")
	if maxPages < 1 {
		maxPages = chatSearchDefaultPageLimit
	}
	if maxPages > chatSearchMaximumPageLimit {
		maxPages = chatSearchMaximumPageLimit
	}

	allItems := make([]interface{}, 0)
	var lastData map[string]interface{}
	var lastHasMore bool
	var lastPageToken string
	prevPageToken := "__START__"
	delete(params, "page_token")

	for page := 0; page < maxPages; page++ {
		if page > 0 {
			params["page_token"] = lastPageToken
		}
		data, err := runtime.CallAPITyped("POST", "/open-apis/im/v2/chats/search", params, body)
		if err != nil {
			return nil, err
		}
		lastData = data
		if items, ok := data["items"].([]interface{}); ok {
			allItems = append(allItems, items...)
		}
		lastHasMore, lastPageToken = common.PaginationMeta(data)
		fmt.Fprintf(runtime.IO().ErrOut, "page %d: %d chats\n", page+1, len(allItems))

		if !lastHasMore || lastPageToken == "" {
			break
		}
		if lastPageToken == prevPageToken {
			fmt.Fprintln(runtime.IO().ErrOut, "warning: page_token did not change, stopping pagination to avoid infinite loop")
			break
		}
		if page+1 >= maxPages {
			fmt.Fprintf(runtime.IO().ErrOut, "[pagination] reached page limit (%d) while has_more=true; result is incomplete. Increase --page-limit up to 1000 or resume with the page_token returned in stdout.\n", maxPages)
			break
		}
		prevPageToken = lastPageToken
	}

	if lastData == nil {
		lastData = map[string]interface{}{}
	}
	lastData["items"] = allItems
	lastData["has_more"] = lastHasMore
	lastData["page_token"] = lastPageToken
	return lastData, nil
}

// buildSearchChatBody builds the JSON request body for POST /im/v2/chats/search
// from the runtime flag values. The query string is normalized via
// normalizeChatSearchQuery (hyphenated terms get quoted). The "filter" object
// is omitted when no filter flags are set; "sorter" is omitted when --sort
// (and its hidden alias --sort-by) is unset.
func buildSearchChatBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}

	if query := runtime.Str("query"); query != "" {
		// API behavior: hyphenated keywords should be wrapped in double quotes
		// for more accurate search results.
		body["query"] = normalizeChatSearchQuery(query)
	}

	// Build filter
	filter := map[string]interface{}{}
	if st := runtime.Str("search-types"); st != "" {
		filter["search_types"] = common.SplitCSV(st)
	}
	// chat_modes is a server-side filter. The CLI exposes group/topic; the wire
	// expects default/thread. Map and dedupe (the API caps the list at 2, and
	// there are only 2 distinct modes) while preserving the user's order.
	if cm := runtime.Str("chat-modes"); cm != "" {
		seen := map[string]bool{}
		var modes []string
		for _, mode := range common.SplitCSV(cm) {
			wire := map[string]string{"group": "default", "topic": "thread"}[mode]
			if wire == "" || seen[wire] {
				continue
			}
			seen[wire] = true
			modes = append(modes, wire)
		}
		if len(modes) > 0 {
			filter["chat_modes"] = modes
		}
	}
	if mi := runtime.Str("member-ids"); mi != "" {
		filter["member_ids"] = common.SplitCSV(mi)
	}
	if runtime.Bool("is-manager") {
		filter["is_manager"] = true
	}
	if runtime.Bool("disable-search-by-user") {
		filter["disable_search_by_user"] = true
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	// Build sorter (always descending). --sort maps field -> field_desc; the hidden
	// --sort-by alias is already the upstream value (pass-through). Omitted when unset.
	sorter := map[string]string{
		"create_time":  "create_time_desc",
		"update_time":  "update_time_desc",
		"member_count": "member_count_desc",
	}[runtime.Str("sort")]
	if old, ok := aliasFlagValue(runtime, "sort-by", "sort"); ok {
		sorter = old
	}
	if sorter != "" {
		body["sorter"] = sorter
	}

	return body
}

// buildSearchChatParams builds the query parameters for the POST
// /im/v2/chats/search call. page_size defaults to the API default of 20 when
// not provided; page_token is omitted when empty.
func buildSearchChatParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
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

// normalizeChatSearchQuery wraps hyphenated search queries in double quotes
// because the search API treats hyphenated keywords specially and expects the
// whole query to be quoted. Already-quoted input is unwrapped before requoting
// so we don't emit nested quotes. Inputs without "-" pass through unchanged.
func normalizeChatSearchQuery(query string) string {
	if !strings.Contains(query, "-") {
		return query
	}
	if unquoted, err := strconv.Unquote(query); err == nil {
		query = unquoted
	}
	return strconv.Quote(query)
}

// detectAllNonMemberPreSkip returns SkipReasonAllNonMember when --search-types
// is exactly "public_not_joined" — the one combination guaranteeing no member
// chats, making the mute filter a no-op. Any other value (including empty or
// mixed) returns "".
func detectAllNonMemberPreSkip(searchTypesCSV string) string {
	types := common.SplitCSV(searchTypesCSV)
	if len(types) == 1 && types[0] == "public_not_joined" {
		return SkipReasonAllNonMember
	}
	return ""
}
