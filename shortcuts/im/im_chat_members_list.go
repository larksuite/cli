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
		path := fmt.Sprintf(imChatMembersListPath, runtime.Str("chat-id"))
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

// placeholder for Task 2 — keep file compiling if Task 2 not yet written.
func executeMembersList(ctx context.Context, runtime *common.RuntimeContext) error {
	_ = output.PrintTable
	_ = io.Discard
	return nil
}
