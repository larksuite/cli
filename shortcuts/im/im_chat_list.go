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

// imChatListPath is the upstream HTTP path for the +chat-list shortcut.
const imChatListPath = "/open-apis/im/v1/chats"

// bot_strip_p2p is the request-level adjustment notice emitted when bot
// identity receives a mixed --types containing "p2p": the p2p value is
// removed from the outgoing query (which the API would otherwise reject)
// and the caller is informed via a stderr warning + a structured entry
// in outData["notices"]. This is a notice, not a filter — it lives in a
// separate slot from outData["filter"] so the two never collide.
const (
	botStripP2pCode    = "bot_strip_p2p"
	botStripP2pMessage = "To protect user privacy, bot identity cannot list p2p chats; --types=p2p,group was sent as types=group. Use --as user to include p2p."
)

// writeBotStripP2pWarning prints the bot_strip_p2p adjustment to stderr in
// the repo's standard "warning: <code>: <message>" form (matches the format
// used in shortcuts/common/runner.go's unknown-format fallback).
func writeBotStripP2pWarning(errOut io.Writer) {
	fmt.Fprintf(errOut, "warning: %s: %s\n", botStripP2pCode, botStripP2pMessage)
}

// ImChatList is the +chat-list shortcut: wraps GET /open-apis/im/v1/chats to
// list groups the current user/bot is a member of. Supports sort order,
// pagination, and (user identity only) muted-chat filtering via --exclude-muted.
var ImChatList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-list",
	Description: "List chats the current user/bot is a member of; defaults to groups; pass --types=p2p,group to include p2p single chats (user-only); user/bot; supports sorting, pagination, --exclude-muted (user-only)",
	Risk:        "read",
	Scopes:      []string{"im:chat:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-id-type", Default: "open_id", Desc: "ID type for owner_id in response", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "sort-type", Default: "ByCreateTimeAsc", Desc: "sort order", Enum: []string{"ByCreateTimeAsc", "ByActiveTimeDesc"}},
		{Name: "types", Type: "string_slice", Desc: "chat types to include (group, p2p); omit = groups only (backward compatible); p2p requires user identity"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "exclude-muted", Type: "bool", Desc: "(user identity only) drop chats the current user has muted (do-not-disturb); bot identity returns all chats unfiltered"},
	},
	// DryRun previews the GET /open-apis/im/v1/chats request without executing.
	// When bot identity strips p2p from --types, emits the same stderr warning
	// Execute would emit, so DryRun output truthfully reflects what the API
	// will receive (matches the shortcuts/drive/drive_search.go pattern of
	// echoing request-level adjustments in both DryRun and Execute).
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		effective, stripped, _ := resolveTypes(runtime) // Validate has already guaranteed err == nil
		if stripped {
			writeBotStripP2pWarning(runtime.IO().ErrOut)
		}
		return common.NewDryRunAPI().
			GET(imChatListPath).
			Params(buildChatListParams(runtime, effective))
	},
	// Validate enforces flag preconditions: page-size bounds, --types element
	// enum, and the bot + single-p2p rejection (mixed types degrade in Execute).
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if n := runtime.Int("page-size"); n < 1 || n > 100 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be an integer between 1 and 100").WithParam("--page-size")
		}
		parts, err := normalizeTypes(runtime.StrSlice("types"))
		if err != nil {
			return err
		}
		if len(parts) == 1 && parts[0] == "p2p" && runtime.IsBot() {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				`--types=p2p (single chats) is only supported with user identity (--as user). To protect user privacy, bot identity cannot list p2p chats. Use --as user, or include "group" in --types.`).WithParam("--types")
		}
		return nil
	},
	// Execute fetches one page of chats, optionally applies --exclude-muted
	// via MaybeApplyMuteFilter, and renders the result. outData["filter"] is
	// populated only when --exclude-muted is set (backward compatible).
	// outData["notices"] is populated only when bot identity strips p2p from
	// --types — a request-level adjustment that lives in its own slot so it
	// never collides with the row-level mute filter.
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		effective, stripped, _ := resolveTypes(runtime) // Validate guarantees err == nil
		if stripped {
			writeBotStripP2pWarning(runtime.IO().ErrOut)
		}
		params := buildChatListParams(runtime, effective)
		resData, err := runtime.CallAPITyped("GET", imChatListPath, params, nil)
		if err != nil {
			return err
		}

		rawItems, _ := resData["items"].([]interface{})
		hasMore, pageToken := common.PaginationMeta(resData)

		var items []map[string]interface{}
		for _, raw := range rawItems {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			items = append(items, item)
		}

		mfOut, err := MaybeApplyMuteFilter(runtime, MuteFilterInput{
			ExcludeMuted: runtime.Bool("exclude-muted"),
			IsBot:        runtime.IsBot(),
			Chats:        items,
			ChatIDKey:    "chat_id",
			HasMore:      hasMore,
		})
		if err != nil {
			return err
		}
		items = mfOut.Chats

		outData := map[string]interface{}{
			"chats":      items,
			"has_more":   hasMore,
			"page_token": pageToken,
		}
		if mfOut.Meta.Applied != "" {
			outData["filter"] = MuteFilterMetaToMap(mfOut.Meta)
		}
		if stripped {
			outData["notices"] = []map[string]interface{}{
				{"code": botStripP2pCode, "message": botStripP2pMessage},
			}
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(items) == 0 {
				fmt.Fprintln(w, "No chats found.")
				if mfOut.Meta.Hint != "" {
					fmt.Fprintln(w, mfOut.Meta.Hint)
				}
				return
			}
			rows := make([]map[string]interface{}, 0, len(items))
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
				if external, ok := m["external"].(bool); ok {
					row["external"] = external
				}
				if status, _ := m["chat_status"].(string); status != "" {
					row["chat_status"] = status
				}
				if chatMode, _ := m["chat_mode"].(string); chatMode != "" {
					row["chat_mode"] = chatMode
					if chatMode == "p2p" {
						if pt, _ := m["p2p_target_type"].(string); pt != "" {
							row["p2p_target_type"] = pt
						}
						if pid, _ := m["p2p_target_id"].(string); pid != "" {
							row["p2p_target_id"] = pid
						}
					}
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d chat(s) listed", len(rows))
			if hasMore {
				fmt.Fprint(w, " (more available, use --page-token to fetch next page")
				if pageToken != "" {
					fmt.Fprintf(w, ", page_token: %s", pageToken)
				}
				fmt.Fprint(w, ")")
			}
			fmt.Fprintln(w)
			if mfOut.Meta.Hint != "" {
				fmt.Fprintln(w, mfOut.Meta.Hint)
			}
		})
		return nil
	},
}

// normalizeTypes validates and normalizes the --types slice already parsed by cobra.
// cobra's StringSlice handles the CSV split automatically — both --types=p2p,group
// and repeated --types p2p --types group arrive here as a 2-element []string,
// so this function never re-splits on commas.
// Returns the normalized (lowercased, deduped, in input order) parts on success.
// Empty raw input is a no-op (returns nil, nil).
// Returns ErrValidation when any element is empty or outside {"p2p", "group"}.
func normalizeTypes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--types must contain at least one of p2p, group").WithParam("--types")
		}
		if p != "p2p" && p != "group" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--types contains invalid value %q: expected one of p2p, group", p).WithParam("--types")
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

// resolveTypes layers bot identity downgrade on top of normalizeTypes.
// Under bot identity, "p2p" is stripped from the parts and the caller is
// informed (DryRun / Execute emit a stderr warning; Execute additionally
// writes a structured entry under outData["notices"]).
// Validate has already rejected "bot + parts == ['p2p']" cases, so kept is
// never empty here.
//
// Returns (effective CSV, stripped, err):
//   - effective: comma-joined types to send as the API query param
//   - stripped:  true iff bot identity removed "p2p" from a mixed --types value
//   - err:       forwarded from normalizeTypes
func resolveTypes(runtime *common.RuntimeContext) (string, bool, error) {
	parts, err := normalizeTypes(runtime.StrSlice("types"))
	if err != nil {
		return "", false, err
	}
	if !runtime.IsBot() {
		return strings.Join(parts, ","), false, nil
	}
	// Bot identity: strip "p2p" so the API call succeeds with just groups.
	// Validate has already rejected the "bot + only p2p" case, so kept is never empty here.
	// Allocate a fresh slice (rather than aliasing parts[:0]) — parts has at most 2
	// elements so the cost is negligible, and avoiding shared backing storage removes
	// a class of "two slices, same array" surprises if a future caller keeps parts.
	stripped := false
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "p2p" {
			stripped = true
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, ","), stripped, nil
}

// buildChatListParams builds the query parameters. effectiveTypes is the
// CSV string already normalized + bot-stripped by resolveTypes; pass "" to
// omit the types query param entirely (backward compatible default).
func buildChatListParams(runtime *common.RuntimeContext, effectiveTypes string) map[string]interface{} {
	params := map[string]interface{}{
		"user_id_type": runtime.Str("user-id-type"),
		"sort_type":    runtime.Str("sort-type"),
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
		params["types"] = effectiveTypes
	}
	return params
}
