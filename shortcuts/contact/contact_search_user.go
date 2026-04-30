// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	searchUserURL = "/open-apis/contact/v3/users/search"

	maxSearchUserQueryChars = 50
	maxSearchUserUserIDs    = 100
	maxSearchUserPageSize   = 30
)

type searchUserBoolFilter struct {
	Flag  string
	Apply func(*searchUserAPIFilter)
}

// Three flags rename their API counterparts to remove jargon / overloaded
// terms: has-chatted→has_contact, exclude-external-users→exclude_outer_contact,
// left-organization→is_resigned. Reading API docs alongside this CLI requires
// translating through this table.
var searchUserBoolFilters = []searchUserBoolFilter{
	{"left-organization", func(f *searchUserAPIFilter) { f.IsResigned = true }},
	{"has-chatted", func(f *searchUserAPIFilter) { f.HasContact = true }},
	{"exclude-external-users", func(f *searchUserAPIFilter) { f.ExcludeOuterContact = true }},
	{"has-enterprise-email", func(f *searchUserAPIFilter) { f.HasEnterpriseEmail = true }},
}

var fixedLocaleFallback = []string{
	"ja_jp", "zh_hk", "zh_tw", "ko_kr",
	"id_id", "vi_vn", "th_th",
	"pt_br", "es_es", "de_de", "fr_fr", "it_it", "ru_ru",
}

// display_info is empirically observed as up to 3 newline-separated segments:
//
//	<h>{matched}</h>...   ← hit highlights (line 0)
//	{department}          ← may be empty (line 1, optional)
//	[Contacted X days ago] ← recency hint (last line, optional)
//
// The format is undocumented and segments may be omitted; we extract by role
// (regex for highlights, line-1 for department, last bracketed line for
// recency) rather than by fixed index, so missing segments degrade gracefully.
var (
	displayInfoHighlightRE = regexp.MustCompile(`<h>(.*?)</h>`)
	displayInfoRecencyRE   = regexp.MustCompile(`^\[(.+)\]$`)
)

type searchUserAPIRequest struct {
	Query  string               `json:"query,omitempty"`
	Filter *searchUserAPIFilter `json:"filter,omitempty"`
}

// All bool fields use omitempty: validation rejects =false, so any field set
// here is true; unset fields stay out of the request entirely.
type searchUserAPIFilter struct {
	UserIDs             []string `json:"user_ids,omitempty"`
	IsResigned          bool     `json:"is_resigned,omitempty"`
	HasContact          bool     `json:"has_contact,omitempty"`
	ExcludeOuterContact bool     `json:"exclude_outer_contact,omitempty"`
	HasEnterpriseEmail  bool     `json:"has_enterprise_email,omitempty"`
}

type searchUserAPIEnvelope struct {
	Code int                `json:"code"`
	Msg  string             `json:"msg"`
	Data *searchUserAPIData `json:"data"`
}

type searchUserAPIData struct {
	Items     []searchUserAPIItem `json:"items"`
	HasMore   bool                `json:"has_more"`
	PageToken string              `json:"page_token"`
}

type searchUserAPIItem struct {
	ID          string            `json:"id"`
	DisplayInfo string            `json:"display_info"`
	MetaData    searchUserAPIMeta `json:"meta_data"`
}

type searchUserAPIMeta struct {
	I18nNames             map[string]string `json:"i18n_names"`
	MailAddress           string            `json:"mail_address"`
	EnterpriseMailAddress string            `json:"enterprise_mail_address"`
	IsRegistered          bool              `json:"is_registered"`
	ChatID                string            `json:"chat_id"`
	IsCrossTenant         bool              `json:"is_cross_tenant"`
	// API ships the user's profile signature in `description`; the field name
	// is misleading because it carries the personal signature ("个性签名"),
	// not a generic description. We surface it as `signature` downstream.
	Description string `json:"description"`
}

// JSON tags on searchUser are the public contract for agents and downstream
// scripts; never rename without bumping the shortcut version.
type searchUser struct {
	OpenID          string   `json:"open_id"`
	LocalizedName   string   `json:"localized_name"`
	Email           string   `json:"email"`
	EnterpriseEmail string   `json:"enterprise_email"`
	IsActivated     bool     `json:"is_activated"`
	IsCrossTenant   bool     `json:"is_cross_tenant"`
	P2PChatID       string   `json:"p2p_chat_id"`
	HasChatted      bool     `json:"has_chatted"`
	Department      string   `json:"department"`
	Signature       string   `json:"signature,omitempty"`
	ChatRecencyHint string   `json:"chat_recency_hint"`
	MatchSegments   []string `json:"match_segments"`
}

type searchUserResponse struct {
	Users   []searchUser `json:"users"`
	HasMore bool         `json:"has_more"`
}

var ContactSearchUser = common.Shortcut{
	Service:     "contact",
	Command:     "+search-user",
	Description: "Search Lark/Feishu users by keyword, open_id list, or filter (requires --as user)",
	Risk:        "read",
	Scopes:      []string{"contact:user:search"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword (≤ 50 characters)"},
		{Name: "user-ids", Desc: "open_ids to look up or restrict --query against (CSV; me = caller; ≤ 100)"},
		{Name: "has-chatted", Type: "bool", Desc: "restrict to users you've chatted with (omit to disable; =false rejected)"},
		{Name: "has-enterprise-email", Type: "bool", Desc: "restrict to users with enterprise email (omit to disable; =false rejected)"},
		{Name: "exclude-external-users", Type: "bool", Desc: "exclude external (cross-tenant) users; default includes them (omit to disable; =false rejected)"},
		{Name: "left-organization", Type: "bool", Desc: "restrict to users who have left the organization (omit to disable; =false rejected)"},
		{Name: "lang", Desc: "override locale for localized_name (e.g. zh_cn, en_us)"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "rows per request, 1-30"},
		{Name: "queries", Desc: "comma-separated keywords searched in parallel; output is a flat users[] with matched_query plus a queries[] sidecar"},
	},
	Tips: []string{
		"Keyword search: lark-cli contact +search-user --query 'alice'",
		"Look up by ID (or 'me' for self): lark-cli contact +search-user --user-ids 'ou_xxx,me'",
		"Filter-only enumeration — users you've chatted with: lark-cli contact +search-user --has-chatted",
		"Refine same-name hits: lark-cli contact +search-user --query '张三' --has-chatted --exclude-external-users",
		"Multi-name fanout: lark-cli contact +search-user --queries 'alice,bob,张三'",
		"open_id is the stable identifier for follow-up commands; on has_more=true add filters or tighten --query — there is no auto-pagination.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateSearchUser(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if raw := strings.TrimSpace(runtime.Str("queries")); raw != "" {
			queries := parseAndDedupQueries(raw)
			filter, err := buildFanoutFilter(runtime)
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}
			api := common.NewDryRunAPI()
			for _, q := range queries {
				body := &searchUserAPIRequest{Query: q}
				if filter != nil {
					body.Filter = filter
				}
				api.POST(searchUserURL).
					Params(map[string]interface{}{"page_size": runtime.Int("page-size")}).
					Body(body)
			}
			return api
		}
		body, err := buildSearchUserBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(searchUserURL).
			Params(map[string]interface{}{"page_size": runtime.Int("page-size")}).
			Body(body)
	},
	Execute: executeSearchUser,
}

func executeSearchUser(ctx context.Context, runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("queries")) != "" {
		return executeSearchUserFanout(ctx, runtime)
	}
	return executeSearchUserSingle(ctx, runtime)
}

func executeSearchUserSingle(ctx context.Context, runtime *common.RuntimeContext) error {
	body, err := buildSearchUserBody(runtime)
	if err != nil {
		return err
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     searchUserURL,
		Body:        body,
		QueryParams: larkcore.QueryParams{"page_size": []string{strconv.Itoa(runtime.Int("page-size"))}},
	})
	if err != nil {
		return err
	}
	if apiResp.StatusCode != http.StatusOK {
		return output.ErrAPI(apiResp.StatusCode, http.StatusText(apiResp.StatusCode), string(apiResp.RawBody))
	}

	var resp searchUserAPIEnvelope
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return output.ErrWithHint(output.ExitInternal, "validation", "unmarshal response failed", err.Error())
	}
	if resp.Code != 0 {
		return output.ErrAPI(resp.Code, resp.Msg, string(apiResp.RawBody))
	}

	users, hasMore := projectUsers(resp.Data, runtime.Str("lang"), runtime.Config.Brand)
	out := searchUserResponse{Users: users, HasMore: hasMore}

	runtime.OutFormat(out, &output.Meta{Count: len(users)}, func(w io.Writer) {
		if len(users) == 0 {
			fmt.Fprintln(w, "No users found.")
			return
		}
		output.PrintTable(w, prettyUserRows(users))
	})
	if hasMore && isHumanReadableFormat(runtime.Format) {
		fmt.Fprintln(runtime.IO().ErrOut,
			"\nhint: more matches exist; refine the query (e.g., add --has-chatted, a full email, or a department keyword)")
	}
	return nil
}

func isHumanReadableFormat(format string) bool {
	return format == "pretty" || format == "table"
}

// We deliberately do not surface a numeric rank: the API returns no relevance
// score, and a derived ordinal would tempt agents to over-trust it.
func projectUsers(data *searchUserAPIData, lang string, brand core.LarkBrand) ([]searchUser, bool) {
	if data == nil {
		return []searchUser{}, false
	}
	users := make([]searchUser, 0, len(data.Items))
	for i := range data.Items {
		users = append(users, rowFromItem(&data.Items[i], lang, brand))
	}
	return users, data.HasMore
}

func parseDisplayInfo(raw string) (segments []string, department, recencyHint string) {
	segments = make([]string, 0)
	if raw == "" {
		return segments, "", ""
	}
	for _, m := range displayInfoHighlightRE.FindAllStringSubmatch(raw, -1) {
		segments = append(segments, m[1])
	}
	lines := strings.Split(raw, "\n")
	if len(lines) >= 2 {
		department = strings.TrimSpace(lines[1])
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if m := displayInfoRecencyRE.FindStringSubmatch(line); m != nil {
			recencyHint = m[1]
		}
		break
	}
	return segments, department, recencyHint
}

// map[] shape forced by output.PrintTable; this is the only map conversion in
// this file.
func prettyUserRows(users []searchUser) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		rows = append(rows, map[string]interface{}{
			"localized_name":    u.LocalizedName,
			"department":        common.TruncateStr(u.Department, 50),
			"enterprise_email":  u.EnterpriseEmail,
			"has_chatted":       u.HasChatted,
			"chat_recency_hint": u.ChatRecencyHint,
			"open_id":           u.OpenID,
		})
	}
	return rows
}

// Priority: explicit --lang → brand-preferred locales (feishu→zh_cn first,
// lark→en_us first) → fixedLocaleFallback → dictionary order → openID.
// Does NOT fall back to display_info, which may contain phone/email instead
// of a name.
func pickName(i18n map[string]string, lang string, brand core.LarkBrand, openID string) string {
	primary := make([]string, 0, 3)
	if lang != "" {
		primary = append(primary, strings.ReplaceAll(strings.ToLower(lang), "-", "_"))
	}
	switch brand {
	case core.BrandLark:
		primary = append(primary, "en_us", "zh_cn")
	default:
		primary = append(primary, "zh_cn", "en_us")
	}

	for _, loc := range primary {
		if v := i18n[loc]; v != "" {
			return v
		}
	}
	for _, loc := range fixedLocaleFallback {
		if v := i18n[loc]; v != "" {
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
			if v := i18n[k]; v != "" {
				return v
			}
		}
	}
	return openID
}

// Cross-tenant users may have empty email / department; pass through as empty
// string so consumers can distinguish "unknown" from "confirmed absent".
func rowFromItem(item *searchUserAPIItem, lang string, brand core.LarkBrand) searchUser {
	meta := &item.MetaData
	i18n := meta.I18nNames
	if i18n == nil {
		i18n = map[string]string{}
	}
	segments, department, recencyHint := parseDisplayInfo(item.DisplayInfo)

	return searchUser{
		OpenID:          item.ID,
		LocalizedName:   pickName(i18n, lang, brand, item.ID),
		Email:           meta.MailAddress,
		EnterpriseEmail: meta.EnterpriseMailAddress,
		IsActivated:     meta.IsRegistered,
		IsCrossTenant:   meta.IsCrossTenant,
		P2PChatID:       meta.ChatID,
		HasChatted:      meta.ChatID != "",
		Department:      department,
		Signature:       meta.Description,
		ChatRecencyHint: recencyHint,
		MatchSegments:   segments,
	}
}

func validateSearchUser(runtime *common.RuntimeContext) error {
	if !hasAnySearchInput(runtime) {
		return common.FlagErrorf(
			"specify at least one of --query, --queries, --user-ids, --has-chatted, --has-enterprise-email, --exclude-external-users, --left-organization",
		)
	}

	queriesRaw := strings.TrimSpace(runtime.Str("queries"))
	if queriesRaw != "" {
		if strings.TrimSpace(runtime.Str("query")) != "" {
			return common.FlagErrorf("--query and --queries are mutually exclusive")
		}
		if strings.TrimSpace(runtime.Str("user-ids")) != "" {
			return common.FlagErrorf("--user-ids and --queries are mutually exclusive")
		}
		queries := parseAndDedupQueries(queriesRaw)
		if len(queries) == 0 {
			return common.FlagErrorf("--queries: no valid query parsed from %q (separate entries with ',')", queriesRaw)
		}
		if len(queries) > maxFanoutQueries {
			return common.FlagErrorf("--queries: must be at most %d entries (got %d)", maxFanoutQueries, len(queries))
		}
		for _, q := range queries {
			if utf8.RuneCountInString(q) > maxSearchUserQueryChars {
				return common.FlagErrorf("--queries: entry %q exceeds %d characters", q, maxSearchUserQueryChars)
			}
		}
	}

	if q := strings.TrimSpace(runtime.Str("query")); q != "" {
		if utf8.RuneCountInString(q) > maxSearchUserQueryChars {
			return common.FlagErrorf("--query: length must be between 1 and %d characters", maxSearchUserQueryChars)
		}
	}

	if raw := strings.TrimSpace(runtime.Str("user-ids")); raw != "" {
		ids, err := common.ResolveOpenIDs("--user-ids", common.SplitCSV(raw), runtime)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return common.FlagErrorf("--user-ids: no valid open_id parsed from %q (separate entries with ',')", raw)
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

	// Reject explicit =false: agents passing it almost always mean "do not
	// filter", but the API treats it as "must NOT match". Hard error prevents
	// silent wrong-result bugs.
	for _, bf := range searchUserBoolFilters {
		if runtime.Cmd.Flags().Changed(bf.Flag) && !runtime.Bool(bf.Flag) {
			return common.FlagErrorf(
				"--%s: pass the flag to enable the filter; omit it to disable filtering (=false is rejected to prevent silent wrong results)",
				bf.Flag,
			)
		}
	}

	if n := runtime.Int("page-size"); n < 1 || n > maxSearchUserPageSize {
		return common.FlagErrorf("--page-size: must be between 1 and %d", maxSearchUserPageSize)
	}
	return nil
}

// Cannot use common.AtLeastOne: it only inspects string flags; bool filters
// need Changed() detection.
func hasAnySearchInput(runtime *common.RuntimeContext) bool {
	if strings.TrimSpace(runtime.Str("query")) != "" {
		return true
	}
	if strings.TrimSpace(runtime.Str("queries")) != "" {
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

func buildSearchUserBody(runtime *common.RuntimeContext) (*searchUserAPIRequest, error) {
	req := &searchUserAPIRequest{}

	if q := strings.TrimSpace(runtime.Str("query")); q != "" {
		req.Query = q
	}

	filter := &searchUserAPIFilter{}
	hasFilter := false

	if raw := strings.TrimSpace(runtime.Str("user-ids")); raw != "" {
		ids, err := common.ResolveOpenIDs("--user-ids", common.SplitCSV(raw), runtime)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			filter.UserIDs = ids
			hasFilter = true
		}
	}

	for _, bf := range searchUserBoolFilters {
		if runtime.Cmd.Flags().Changed(bf.Flag) && runtime.Bool(bf.Flag) {
			bf.Apply(filter)
			hasFilter = true
		}
	}

	if hasFilter {
		req.Filter = filter
	}
	return req, nil
}
