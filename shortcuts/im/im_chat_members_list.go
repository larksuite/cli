// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// imChatMembersListPath is the upstream path template for +chat-members-list.
// chat_id is substituted at call time.
const imChatMembersListPath = "/open-apis/im/v1/chats/%s/members/list"

// ImChatMembersList is the +chat-members-list shortcut: wraps
// GET /open-apis/im/v1/chats/{chat_id}/members/list to return BOTH user and bot
// members of a group in one call, faithfully split into users[]/bots[] buckets.
var ImChatMembersList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-members-list",
	Description: "List all members (users and bots) of a group chat in one call; user/bot; returns users[]/bots[] buckets with totals; supports --member-types filter, --member-id-type, pagination and --page-all",
	Risk:        "read",
	Scopes:      []string{"im:chat.members:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "chat-id", Required: true, Desc: "group chat ID (oc_xxx)"},
		{Name: "member-types", Type: "string_slice", Desc: "filter member types to fetch (user, bot); omit = both"},
		{Name: "member-id-type", Default: "open_id", Desc: "ID type for member_id in response", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages, accumulating users[]+bots[]"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max pages when --page-all is enabled (default 20, max 1000)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := common.ValidateChatIDTyped("--chat-id", runtime.Str("chat-id")); err != nil {
			return err
		}
		if n := runtime.Int("page-size"); n < 1 || n > 100 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be an integer between 1 and 100").WithParam("--page-size")
		}
		if runtime.Bool("page-all") {
			if n := runtime.Int("page-limit"); n < 1 || n > 1000 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-limit must be an integer between 1 and 1000").WithParam("--page-limit")
			}
		}
		if _, err := normalizeMemberTypes(runtime.StrSlice("member-types")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		effective, _ := normalizeMemberTypes(runtime.StrSlice("member-types")) // Validate guarantees err == nil
		chatID, _ := common.ValidateChatIDTyped("--chat-id", runtime.Str("chat-id"))
		path := fmt.Sprintf(imChatMembersListPath, validate.EncodePathSegment(chatID))
		return common.NewDryRunAPI().
			GET(path).
			Params(buildMembersListParams(runtime, strings.Join(effective, ",")))
	},
	Execute: executeMembersList, // defined in Task 2
}

// normalizeMemberTypes validates/normalizes the --member-types slice: trims,
// lowercases, dedupes (preserving order), and rejects anything outside
// {user, bot}. Empty input returns (nil, nil).
func normalizeMemberTypes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-types must contain at least one of user, bot").WithParam("--member-types")
		}
		if p != "user" && p != "bot" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-types contains invalid value %q: expected one of user, bot", p).WithParam("--member-types")
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

// buildMembersListParams assembles the query params. effectiveTypes is the
// CSV produced by normalizeMemberTypes; pass "" to omit member_types.
func buildMembersListParams(runtime *common.RuntimeContext, effectiveTypes string) map[string]interface{} {
	params := map[string]interface{}{
		"member_id_type": runtime.Str("member-id-type"),
	}
	if n := runtime.Int("page-size"); n > 0 {
		params["page_size"] = n
	} else {
		params["page_size"] = 20
	}
	if pt := runtime.Str("page-token"); pt != "" {
		params["page_token"] = pt
	}
	if effectiveTypes != "" {
		params["member_types"] = effectiveTypes
	}
	return params
}

// executeMembersList runs the single-page or --page-all fetch and renders output.
func executeMembersList(ctx context.Context, runtime *common.RuntimeContext) error {
	effective, _ := normalizeMemberTypes(runtime.StrSlice("member-types")) // Validate guarantees err == nil
	params := buildMembersListParams(runtime, strings.Join(effective, ","))
	chatID, _ := common.ValidateChatIDTyped("--chat-id", runtime.Str("chat-id")) // Validate guarantees err == nil
	path := fmt.Sprintf(imChatMembersListPath, validate.EncodePathSegment(chatID))

	var data map[string]interface{}
	var err error
	if runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token") {
		data, err = fetchAllMemberPages(ctx, runtime, path, params, runtime.Int("page-limit"))
	} else {
		data, err = runtime.CallAPITyped("GET", path, params, nil)
	}
	if err != nil {
		return err
	}

	outData := assembleMembersOutput(data)
	if truncs, ok := outData["truncations"].([]interface{}); ok && len(truncs) > 0 {
		writeMembersTruncationWarning(runtime.IO().ErrOut, truncs)
	}

	runtime.OutFormat(outData, nil, func(w io.Writer) { renderMembersPretty(w, outData) })
	return nil
}

// assembleMembersOutput extracts the two buckets and pagination metadata from a
// single API data object into the shortcut's output shape. Empty buckets are
// rendered as [] (never omitted) for stable downstream parsing.
func assembleMembersOutput(data map[string]interface{}) map[string]interface{} {
	users, _ := data["users"].([]interface{})
	if users == nil {
		users = []interface{}{}
	}
	bots, _ := data["bots"].([]interface{})
	if bots == nil {
		bots = []interface{}{}
	}
	hasMore, pageToken := common.PaginationMeta(data)
	truncs, _ := data["truncations"].([]interface{})
	if truncs == nil {
		truncs = []interface{}{}
	}
	out := map[string]interface{}{
		"users":       users,
		"bots":        bots,
		"has_more":    hasMore,
		"page_token":  pageToken,
		"truncations": truncs,
	}
	if v, ok := data["user_total"]; ok {
		out["user_total"] = v
	}
	if v, ok := data["bot_total"]; ok {
		out["bot_total"] = v
	}
	return out
}

// writeMembersTruncationWarning emits one stderr warning per truncated type.
func writeMembersTruncationWarning(errOut io.Writer, truncs []interface{}) {
	for _, raw := range truncs {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			continue
		}
		fmt.Fprintf(errOut, "warning: member list truncated by server (member_type=%v, limit=%v); not all members returned\n", m["member_type"], m["limit"])
	}
}

// renderMembersPretty renders the non-JSON table view.
func renderMembersPretty(w io.Writer, outData map[string]interface{}) {
	users, _ := outData["users"].([]interface{})
	bots, _ := outData["bots"].([]interface{})
	if len(users) == 0 && len(bots) == 0 {
		fmt.Fprintln(w, "No members found.")
		return
	}
	rows := make([]map[string]interface{}, 0, len(users)+len(bots))
	for _, raw := range users {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"type": "user", "member_id": m["member_id"], "name": m["name"], "tenant_key": m["tenant_key"],
		})
	}
	for _, raw := range bots {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"type": "bot", "member_id": m["member_id"], "name": m["name"], "app_id": m["app_id"], "tenant_key": m["tenant_key"],
		})
	}
	output.PrintTable(w, rows)
	ut := totalString(outData["user_total"])
	fmt.Fprintf(w, "\n%s user(s), %v bot(s)", ut, outData["bot_total"])
	if hm, _ := outData["has_more"].(bool); hm {
		fmt.Fprintf(w, " (more available, use --page-token to fetch next page")
		if pt, _ := outData["page_token"].(string); pt != "" {
			fmt.Fprintf(w, ", page_token: %s", pt)
		}
		fmt.Fprint(w, ")")
	}
	fmt.Fprintln(w)
}

// fetchAllMemberPages loops up to pageLimit pages, accumulating users[]/bots[]
// from each page. user_total/bot_total/truncations come from the last page
// (group-level totals are page-invariant). has_more/page_token reflect the last
// page fetched — when the loop stops at pageLimit with has_more=true the token
// is preserved so the caller can continue. Stops early if page_token does not
// advance (guard against an infinite loop).
func fetchAllMemberPages(ctx context.Context, runtime *common.RuntimeContext, path string, baseParams map[string]interface{}, pageLimit int) (map[string]interface{}, error) {
	if pageLimit < 1 {
		pageLimit = 1
	}
	allUsers := []interface{}{}
	allBots := []interface{}{}
	var last map[string]interface{}
	token := ""
	for i := 0; i < pageLimit; i++ {
		params := make(map[string]interface{}, len(baseParams)+1)
		for k, v := range baseParams {
			params[k] = v
		}
		if token != "" {
			params["page_token"] = token
		} else {
			delete(params, "page_token")
		}
		data, err := runtime.CallAPITyped("GET", path, params, nil)
		if err != nil {
			return nil, err
		}
		last = data
		if u, ok := data["users"].([]interface{}); ok {
			allUsers = append(allUsers, u...)
		}
		if b, ok := data["bots"].([]interface{}); ok {
			allBots = append(allBots, b...)
		}
		hasMore, nextToken := common.PaginationMeta(data)
		if !hasMore || nextToken == "" {
			break
		}
		if nextToken == token {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: page_token did not change, stopping pagination to avoid infinite loop\n")
			break
		}
		token = nextToken
	}
	// Rebuild a synthetic "last page" data carrying accumulated buckets but the
	// last page's totals / has_more / page_token / truncations.
	merged := map[string]interface{}{}
	for k, v := range last {
		merged[k] = v
	}
	merged["users"] = allUsers
	merged["bots"] = allBots
	return merged, nil
}

// totalString renders user_total, mapping the API's -1 "hidden" sentinel to a
// human label. JSON numbers decode as float64.
func totalString(v interface{}) string {
	switch n := v.(type) {
	case float64:
		if n == -1 {
			return "hidden count of"
		}
		return fmt.Sprintf("%d", int(n))
	case int:
		if n == -1 {
			return "hidden count of"
		}
		return fmt.Sprintf("%d", n)
	default:
		return fmt.Sprintf("%v", v)
	}
}
