// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	searchUserURL = "/open-apis/contact/v3/users/search"

	// Caps reflect the runtime constraints the API owner ships with (tighter
	// than the schema's declared maxLen, which is upper bound not effective).
	maxSearchUserQueryRunes    = 64
	maxSearchUserUserIDs       = 100
	defaultSearchUserPageSize  = 20
	maxSearchUserPageSize      = 30
	defaultSearchUserPageLimit = 20
	maxSearchUserPageLimit     = 40
)

// searchUserBoolFilters is the single source of truth binding each bool filter
// flag to its API body field. One flag name diverges from the API field name
// to reduce ambiguity: "has-chatted" maps to API has_contact. The API name
// overloads "contact" to mean chat history, which collides with the other
// filter exclude_outer_contact (where "contact" means person); the flag name
// matches our output field has_chatted so callers do not have to juggle two
// meanings.
var searchUserBoolFilters = []struct{ Flag, API string }{
	{"is-resigned", "is_resigned"},
	{"has-chatted", "has_contact"},
	{"exclude-outer-contact", "exclude_outer_contact"},
	{"has-enterprise-email", "has_enterprise_email"},
}

// fixedLocaleFallback is the ordered locale list used by pickName after
// brand-preferred locales fail to match; keeps the picked name deterministic
// beyond zh_cn / en_us.
var fixedLocaleFallback = []string{
	"ja_jp", "zh_hk", "zh_tw", "ko_kr",
	"id_id", "vi_vn", "th_th",
	"pt_br", "es_es", "de_de", "fr_fr", "it_it", "ru_ru",
}

var ContactSearchUser = common.Shortcut{
	Service:     "contact",
	Command:     "+search-user",
	Description: "Search users (results sorted by relevance)",
	Risk:        "read",
	Scopes:      []string{"contact:user:search"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword (max 64 runes; recommended for best results)"},
		{Name: "user-ids", Desc: "filter results to these open_id list, CSV; supports \"me\"; max 100"},
		{Name: "is-resigned", Type: "bool", Desc: "set to restrict to resigned-and-chatted users (opt-in filter)"},
		{Name: "has-chatted", Type: "bool", Desc: "set to restrict to users you've chatted with (opt-in filter)"},
		{Name: "exclude-outer-contact", Type: "bool", Desc: "set to exclude outer contacts and only return same-tenant users (opt-in filter)"},
		{Name: "has-enterprise-email", Type: "bool", Desc: "set to restrict to users with enterprise email (opt-in filter)"},
		{Name: "lang", Desc: "override locale for the picked name (e.g. zh_cn, en_us, ja_jp); default: tenant brand"},
		{Name: "page-size", Default: "20", Desc: "page size, 1-30 (default 20)"},
		{Name: "page-all", Type: "bool", Desc: "auto-paginate results until has_more=false or --page-limit reached"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max pages fetched when auto-paginating (default 20, max 40; passing this flag alone implicitly enables --page-all)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateSearchUser(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildSearchUserBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(searchUserURL).
			Params(buildSearchUserParams(runtime)).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildSearchUserBody(runtime)
		if err != nil {
			return err
		}
		autoPaginate, pageLimit := searchUserPaginationConfig(runtime)

		var (
			// Initialized as non-nil so empty result sets serialize as [] (not null).
			allUsers         = make([]map[string]interface{}, 0)
			lastHasMore      bool
			lastPageToken    string
			truncatedByLimit bool
			pageCount        int
		)
		lang := runtime.Str("lang")
		brand := runtime.Config.Brand
		params := buildSearchUserParams(runtime)

		for {
			pageCount++
			data, err := runtime.CallAPI("POST", searchUserURL, params, body)
			if err != nil {
				return err
			}
			users, hasMore, pageToken := extractSearchUsers(data, lang, brand)
			allUsers = append(allUsers, users...)
			lastHasMore = hasMore
			lastPageToken = pageToken

			if !autoPaginate || !hasMore || pageCount >= pageLimit {
				if autoPaginate && hasMore && pageCount >= pageLimit {
					truncatedByLimit = true
				}
				break
			}
			// Prepare params for the next iteration with the fresh page_token.
			nextParams := make(map[string]interface{}, len(params)+1)
			for k, v := range params {
				nextParams[k] = v
			}
			nextParams["page_token"] = pageToken
			params = nextParams
		}

		outData := map[string]interface{}{
			"users":      allUsers,
			"has_more":   lastHasMore,
			"page_token": lastPageToken,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(allUsers)}, func(w io.Writer) {
			if len(allUsers) == 0 {
				fmt.Fprintln(w, "No users found.")
				return
			}
			output.PrintTable(w, prettyUserRows(allUsers))
		})
		if lastHasMore && isHumanReadableFormat(runtime.Format) {
			// Hints go to stderr so stdout stays clean for the actual table
			// output, matching the CLI convention used elsewhere by the runner
			// for warnings and informational messages.
			if truncatedByLimit {
				fmt.Fprintf(runtime.IO().ErrOut,
					"\nwarning: stopped after fetching %d page(s); refine the query with more filters, or raise --page-limit (max %d)\n",
					pageCount, maxSearchUserPageLimit)
			} else {
				fmt.Fprintf(runtime.IO().ErrOut,
					"\n(more available; refine the query or use --page-all to auto-paginate)\n")
			}
		}
		return nil
	},
}

// isHumanReadableFormat reports whether the given output format is meant for
// human reading (so a "(more available, ...)" hint can be appended without
// corrupting structured output streams like ndjson or csv).
func isHumanReadableFormat(format string) bool {
	return format == "pretty" || format == "table"
}

// searchUserPaginationConfig resolves the (autoPaginate, pageLimit) tuple from
// user flags. Rules (mirroring shortcuts/im/im_messages_search.go for
// consistency across search-type shortcuts):
//   - --page-all alone                   → auto-paginate, pageLimit = max (40)
//   - --page-limit N alone               → auto-paginate, pageLimit = N  (implicit enable)
//   - --page-all + --page-limit N        → auto-paginate, pageLimit = N
//   - neither                            → single-page (manual pagination)
func searchUserPaginationConfig(runtime *common.RuntimeContext) (autoPaginate bool, pageLimit int) {
	autoPaginate = runtime.Bool("page-all")
	if runtime.Cmd != nil && runtime.Cmd.Flags().Changed("page-limit") {
		autoPaginate = true
	}
	pageLimit = defaultSearchUserPageLimit
	if runtime.Cmd != nil && runtime.Cmd.Flags().Changed("page-limit") {
		pageLimit = runtime.Int("page-limit")
		if pageLimit > maxSearchUserPageLimit {
			pageLimit = maxSearchUserPageLimit
		}
	} else if runtime.Bool("page-all") {
		pageLimit = maxSearchUserPageLimit
	}
	return autoPaginate, pageLimit
}

// extractSearchUsers projects the API response into the shortcut's user
// objects and surfaces pagination metadata.
func extractSearchUsers(data map[string]interface{}, lang string, brand core.LarkBrand) ([]map[string]interface{}, bool, string) {
	items := common.GetSlice(data, "items")
	users := make([]map[string]interface{}, 0, len(items))
	common.EachMap(items, func(item map[string]interface{}) {
		users = append(users, rowFromItem(item, lang, brand))
	})
	return users, common.GetBool(data, "has_more"), common.GetString(data, "page_token")
}

// prettyUserRows projects the 12-field user objects down to the 6 pretty
// table columns, with display_info truncated and its newlines flattened.
func prettyUserRows(users []map[string]interface{}) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		di := common.GetString(u, "display_info")
		rows = append(rows, map[string]interface{}{
			"display_info":  common.TruncateStr(strings.ReplaceAll(di, "\n", " "), 60),
			"name":          u["name"],
			"open_id":       u["open_id"],
			"email":         u["email"],
			"is_registered": u["is_registered"],
			"has_chatted":   u["has_chatted"],
		})
	}
	return rows
}

// pickName returns a single display name from meta.i18n_names, deterministically.
// Priority: explicit --lang, then brand-preferred locales (feishu → zh_cn first,
// lark → en_us first), then fixedLocaleFallback order, then the dictionary-order
// sweep that tolerates locales not in the fixed list, and finally the
// caller-provided openID.
//
// It does NOT fall back to display_info — the hit-highlight segment there may
// be a phone number or email, not a name.
func pickName(meta map[string]interface{}, lang string, brand core.LarkBrand, openID string) string {
	i18n := common.GetMap(meta, "i18n_names")

	primary := make([]string, 0, 3)
	if lang != "" {
		primary = append(primary, strings.ReplaceAll(strings.ToLower(lang), "-", "_"))
	}
	switch brand {
	case core.BrandLark:
		primary = append(primary, "en_us", "zh_cn")
	default: // feishu or unknown brand: Chinese first
		primary = append(primary, "zh_cn", "en_us")
	}

	for _, loc := range primary {
		if v := common.GetString(i18n, loc); v != "" {
			return v
		}
	}
	for _, loc := range fixedLocaleFallback {
		if v := common.GetString(i18n, loc); v != "" {
			return v
		}
	}
	if len(i18n) > 0 {
		keys := make([]string, 0, len(i18n))
		for k := range i18n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v := common.GetString(i18n, k); v != "" {
				return v
			}
		}
	}
	return openID
}

// rowFromItem projects a single items[i] from the API response into the
// 12-field user shape. Cross-tenant users may have missing email / department
// fields; those pass through as empty string / nil rather than defaults so
// consumers can distinguish "unknown" from "confirmed absent".
func rowFromItem(item map[string]interface{}, lang string, brand core.LarkBrand) map[string]interface{} {
	openID := common.GetString(item, "id")
	meta := common.GetMap(item, "meta_data")
	chatID := common.GetString(meta, "chat_id")

	return map[string]interface{}{
		"name":             pickName(meta, lang, brand, openID),
		"open_id":          openID,
		"i18n_names":       common.GetMap(meta, "i18n_names"),
		"email":            common.GetString(meta, "mail_address"),
		"enterprise_email": common.GetString(meta, "enterprise_mail_address"),
		"is_registered":    common.GetBool(meta, "is_registered"),
		"chat_id":          chatID,
		"has_chatted":      chatID != "",
		"is_cross_tenant":  common.GetBool(meta, "is_cross_tenant"),
		"tenant_id":        common.GetString(meta, "tenant_id"),
		"description":      common.GetString(meta, "description"),
		"display_info":     common.GetString(item, "display_info"),
	}
}

// validateSearchUser enforces the input contract: at least one search input
// (query or filter) must be provided — "empty search" is rejected because
// the API owner flags it as unsupported in practice.
func validateSearchUser(runtime *common.RuntimeContext) error {
	if !hasAnySearchInput(runtime) {
		return common.FlagErrorf(
			"specify at least one of --query, --user-ids, --is-resigned, --has-chatted, --exclude-outer-contact, --has-enterprise-email",
		)
	}

	if q := strings.TrimSpace(runtime.Str("query")); q != "" {
		if utf8.RuneCountInString(q) > maxSearchUserQueryRunes {
			return common.FlagErrorf("--query: length must be between 1 and %d characters", maxSearchUserQueryRunes)
		}
	}

	if raw := strings.TrimSpace(runtime.Str("user-ids")); raw != "" {
		ids, err := common.ResolveOpenIDs("--user-ids", common.SplitCSV(raw), runtime)
		if err != nil {
			return err
		}
		if len(ids) > maxSearchUserUserIDs {
			return common.FlagErrorf("--user-ids: must be at most %d entries", maxSearchUserUserIDs)
		}
		for _, id := range ids {
			if _, err := common.ValidateUserID(id); err != nil {
				return err
			}
		}
	}

	if _, err := common.ValidatePageSize(runtime, "page-size", defaultSearchUserPageSize, 1, maxSearchUserPageSize); err != nil {
		return err
	}
	if runtime.Cmd != nil && runtime.Cmd.Flags().Changed("page-limit") {
		if n := runtime.Int("page-limit"); n < 1 || n > maxSearchUserPageLimit {
			return common.FlagErrorf("--page-limit: must be an integer between 1 and %d", maxSearchUserPageLimit)
		}
	}
	return nil
}

// hasAnySearchInput reports whether the user supplied any of the six search
// inputs. Cannot be replaced with common.AtLeastOne: that helper only
// inspects string flags, whereas the bool filters need Changed() detection
// to preserve tri-state semantics.
func hasAnySearchInput(runtime *common.RuntimeContext) bool {
	if strings.TrimSpace(runtime.Str("query")) != "" {
		return true
	}
	if strings.TrimSpace(runtime.Str("user-ids")) != "" {
		return true
	}
	for _, bf := range searchUserBoolFilters {
		if runtime.Cmd.Flags().Changed(bf.Flag) {
			return true
		}
	}
	return false
}

// buildSearchUserBody constructs the POST body. Bool filters enter the body
// only when the user explicitly set them (Changed()), preserving the
// tri-state distinction between "unset" and "explicit false".
func buildSearchUserBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{}

	if q := strings.TrimSpace(runtime.Str("query")); q != "" {
		body["query"] = q
	}

	filter := map[string]interface{}{}

	if raw := strings.TrimSpace(runtime.Str("user-ids")); raw != "" {
		ids, err := common.ResolveOpenIDs("--user-ids", common.SplitCSV(raw), runtime)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			filter["user_ids"] = ids
		}
	}

	for _, bf := range searchUserBoolFilters {
		if runtime.Cmd.Flags().Changed(bf.Flag) {
			filter[bf.API] = runtime.Bool(bf.Flag)
		}
	}

	if len(filter) > 0 {
		body["filter"] = filter
	}
	return body, nil
}

// buildSearchUserParams constructs the query-string params. page_size falls
// back to the default when omitted. page_token is managed internally by the
// Execute loop when auto-paginating, not exposed as a CLI flag.
func buildSearchUserParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	pageSize, _ := common.ValidatePageSize(runtime, "page-size", defaultSearchUserPageSize, 1, maxSearchUserPageSize)
	params["page_size"] = pageSize
	return params
}
