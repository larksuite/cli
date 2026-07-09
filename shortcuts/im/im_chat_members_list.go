// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	imChatMembersListPathFmt       = "/open-apis/im/v1/chats/%s/members/list"
	chatMembersListDefaultPageSize = 20
	chatMembersListMaxPageSize     = 100
	// chatMembersListDefaultPageDelay throttles --page-all the same way the
	// generic paginateLoop does (200ms). It matters for tenants WITHOUT the
	// server-side member cap, where a large group drains many pages back to
	// back and could otherwise trip rate limits.
	chatMembersListDefaultPageDelay = 200
)

// ImChatMembersList is the +chat-members-list shortcut: it lists chat members,
// returning users and bots in separate buckets (users[]/bots[]). It owns its
// pagination loop (mirroring the generic paginateLoop conventions: a per-page
// log line, a --page-limit cap, a non-advancing-token guard) precisely because
// the response is multi-bucket — the generic --page-all merger is built for
// single-array responses and would drop the bots[] bucket and the final-page
// truncations[] signal. See mergeChatMemberPages for the merge semantics.
var ImChatMembersList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-members-list",
	Description: "List members of a chat; returns separate users[] / bots[] buckets; callable as user or bot; --member-types filters which kinds to return; --page-all pagination; surfaces truncations[] when the server caps a bucket",
	Risk:        "read",
	// Declare the narrowest scope the API accepts so tokens carrying only
	// im:chat.members:read are honored (same rationale as +chat-list).
	Scopes:    []string{"im:chat.members:read"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "chat-id", Required: true, Desc: "chat ID (oc_xxx)"},
		{Name: "member-types", Type: "string_slice", Desc: "member types to return (user, bot); omit = all"},
		{Name: "member-id-type", Default: "open_id", Desc: "ID type for member_id in response", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", chatMembersListDefaultPageSize), Desc: fmt.Sprintf("page size, 1-%d", chatMembersListMaxPageSize)},
		{Name: "page-token", Desc: "page token; implies single-page fetch (no auto-pagination)"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (capped by --page-limit)"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages to fetch with --page-all (default 10, 0 = unlimited)"},
		{Name: "page-delay", Type: "int", Default: fmt.Sprintf("%d", chatMembersListDefaultPageDelay), Desc: "delay in ms between pages when --page-all (0 = no delay)"},
	},
	Tips: []string{
		"Default fetches a single page; pass --page-all to walk every page.",
		"With --page-all and no explicit --page-size, the max page size is used to minimize round-trips.",
		"truncations[] in the result means the server capped a bucket due to security config — the member list is incomplete.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chatID := strings.TrimSpace(runtime.Str("chat-id"))
		if chatID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--chat-id is required (oc_xxx)").WithParam("--chat-id")
		}
		if !strings.HasPrefix(chatID, "oc_") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --chat-id %q: must be an open_chat_id starting with oc_", chatID).WithParam("--chat-id")
		}
		if n := runtime.Int("page-size"); n < 1 || n > chatMembersListMaxPageSize {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be an integer between 1 and %d", chatMembersListMaxPageSize).WithParam("--page-size")
		}
		if n := runtime.Int("page-limit"); n < 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-limit must be a non-negative integer").WithParam("--page-limit")
		}
		if n := runtime.Int("page-delay"); n < 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-delay must be a non-negative integer").WithParam("--page-delay")
		}
		_, err := normalizeMemberTypes(runtime.StrSlice("member-types"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		chatID := strings.TrimSpace(runtime.Str("chat-id"))
		dry := common.NewDryRunAPI()
		if chatMembersShouldAutoPaginate(runtime) {
			dry.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
		}
		params, _ := buildChatMembersParams(runtime, strings.TrimSpace(runtime.Str("page-token")))
		return dry.
			GET(fmt.Sprintf(imChatMembersListPathFmt, validate.EncodePathSegment(chatID))).
			Params(params)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		warnIfConflictingPagingFlags(runtime)

		chatID := strings.TrimSpace(runtime.Str("chat-id"))
		res, err := fetchChatMembers(ctx, runtime, chatID)
		if err != nil {
			return err
		}

		// The truncation signal is the whole reason this is a dedicated shortcut:
		// surface it loudly so an agent never mistakes a capped list for a
		// complete one.
		if len(res.truncations) > 0 {
			writeChatMembersTruncationWarning(runtime.IO().ErrOut, res.truncations)
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Found %d user(s) and %d bot(s)\n", len(res.users), len(res.bots))

		outData := map[string]interface{}{
			"chat_id":     chatID,
			"users":       res.users,
			"bots":        res.bots,
			"truncations": res.truncations,
			"has_more":    res.hasMore,
			"page_token":  res.pageToken,
		}
		if res.userTotal != nil {
			outData["user_total"] = res.userTotal
		}
		if res.botTotal != nil {
			outData["bot_total"] = res.botTotal
		}

		runtime.OutFormat(outData, &output.Meta{Count: len(res.users) + len(res.bots)}, func(w io.Writer) {
			renderChatMembersPretty(w, chatID, res)
		})
		return nil
	},
}

// chatMembersResult is the aggregated view across one or more pages.
type chatMembersResult struct {
	users       []interface{}
	bots        []interface{}
	truncations []interface{}
	userTotal   interface{}
	botTotal    interface{}
	hasMore     bool
	pageToken   string
}

// effectiveChatMembersPageSize resolves the page_size to request. When draining
// every page (--page-all) and the caller did NOT explicitly set --page-size, it
// uses the maximum so a full walk takes the fewest round-trips. An explicit
// --page-size is always honored; without --page-all the smaller default is kept
// as a sensible single-page preview size.
func effectiveChatMembersPageSize(runtime *common.RuntimeContext) int {
	if chatMembersShouldAutoPaginate(runtime) && !runtime.Changed("page-size") {
		return chatMembersListMaxPageSize
	}
	if n := runtime.Int("page-size"); n > 0 {
		return n
	}
	return chatMembersListDefaultPageSize
}

// chatMembersShouldAutoPaginate reports whether the fetch loop should walk
// every page. An explicit --page-token disables the auto loop because the
// caller supplied a specific cursor (single-page fetch).
func chatMembersShouldAutoPaginate(runtime *common.RuntimeContext) bool {
	if strings.TrimSpace(runtime.Str("page-token")) != "" {
		return false
	}
	return runtime.Bool("page-all")
}

// buildChatMembersParams builds the query params for one page request. The
// startToken (when non-empty) seeds the page_token; the loop overrides it per
// page. Returns the params and the normalized member-types CSV (already
// validated by Validate, so the error is only a defensive guard).
func buildChatMembersParams(runtime *common.RuntimeContext, startToken string) (map[string]interface{}, error) {
	memberTypes, err := normalizeMemberTypes(runtime.StrSlice("member-types"))
	if err != nil {
		return nil, err
	}
	params := map[string]interface{}{
		"member_id_type": runtime.Str("member-id-type"),
		"page_size":      effectiveChatMembersPageSize(runtime),
	}
	if memberTypes != "" {
		params["member_types"] = memberTypes
	}
	if startToken != "" {
		params["page_token"] = startToken
	}
	return params, nil
}

// fetchChatMembers walks the list_members endpoint, honoring the four
// pagination flags the same way the generic --page-all path does. It merges
// each page into the aggregate as it arrives (rather than buffering every raw
// page), so peak memory is just the aggregated members plus the single most
// recent page — important for large groups under --page-limit 0.
func fetchChatMembers(ctx context.Context, runtime *common.RuntimeContext, chatID string) (*chatMembersResult, error) {
	auto := chatMembersShouldAutoPaginate(runtime)
	pageLimit := runtime.Int("page-limit")
	pageDelay := runtime.Int("page-delay")
	apiPath := fmt.Sprintf(imChatMembersListPathFmt, validate.EncodePathSegment(chatID))

	params, err := buildChatMembersParams(runtime, strings.TrimSpace(runtime.Str("page-token")))
	if err != nil {
		return nil, err
	}

	res := newChatMembersResult()
	var lastData map[string]interface{}
	pageToken := strings.TrimSpace(runtime.Str("page-token"))
	for page := 0; ; page++ {
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		fmt.Fprintf(runtime.IO().ErrOut, "[page %d] fetching...\n", page+1)
		data, err := runtime.CallAPITyped("GET", apiPath, params, nil)
		if err != nil {
			return nil, err
		}
		addMemberBuckets(res, data)
		lastData = data

		hasMore, nextToken := common.PaginationMeta(data)
		if !auto {
			break
		}
		if !hasMore || nextToken == "" {
			break
		}
		if nextToken == pageToken {
			// Guard against a buggy server echoing the same cursor with
			// has_more=true: without --page-limit we would loop forever.
			fmt.Fprintln(runtime.IO().ErrOut, "Stopping pagination: server returned a non-advancing page_token.")
			break
		}
		if pageLimit > 0 && page+1 >= pageLimit {
			fmt.Fprintf(runtime.IO().ErrOut, "[pagination] reached page limit (%d), stopping. Use --page-all --page-limit 0 to fetch all pages.\n", pageLimit)
			break
		}
		pageToken = nextToken
		// Throttle between pages (only reached when another page follows), so
		// draining a large untruncated list doesn't hammer the API.
		if pageDelay > 0 {
			time.Sleep(time.Duration(pageDelay) * time.Millisecond)
		}
	}
	if lastData != nil {
		applyLastPageSignals(res, lastData)
	}
	return res, nil
}

// newChatMembersResult returns an empty aggregate with non-nil buckets so the
// JSON output always carries arrays (never null).
func newChatMembersResult() *chatMembersResult {
	return &chatMembersResult{
		users:       []interface{}{},
		bots:        []interface{}{},
		truncations: []interface{}{},
	}
}

// addMemberBuckets appends one page's users[] and bots[] into the aggregate.
// Concatenating every bucket is what avoids dropping bots[] — the bug the
// generic single-array --page-all merger would hit on this multi-bucket shape.
func addMemberBuckets(res *chatMembersResult, data map[string]interface{}) {
	if u, ok := data["users"].([]interface{}); ok {
		res.users = append(res.users, u...)
	}
	if b, ok := data["bots"].([]interface{}); ok {
		res.bots = append(res.bots, b...)
	}
}

// applyLastPageSignals copies the per-request signals from the FINAL page:
// has_more / page_token / truncations / totals. These must come from the last
// page, not page 1: truncations[] is emitted only on the final page (empty
// earlier), so reading it sooner would hide a server-side cap; user_total /
// bot_total are server-wide counts, and taking the final page's value keeps a
// single, consistent source rather than a possibly-stale earlier count.
func applyLastPageSignals(res *chatMembersResult, data map[string]interface{}) {
	res.hasMore, res.pageToken = common.PaginationMeta(data)
	if t, ok := data["truncations"].([]interface{}); ok {
		res.truncations = t
	}
	res.userTotal = data["user_total"]
	res.botTotal = data["bot_total"]
}

// mergeChatMemberPages folds a slice of page payloads into one aggregate. It is
// the same logic fetchChatMembers applies incrementally, kept as a pure
// function so the multi-bucket merge + last-page-signal semantics are unit
// tested in one place.
func mergeChatMemberPages(pages []map[string]interface{}) *chatMembersResult {
	res := newChatMembersResult()
	if len(pages) == 0 {
		return res
	}
	for _, data := range pages {
		addMemberBuckets(res, data)
	}
	applyLastPageSignals(res, pages[len(pages)-1])
	return res
}

// normalizeMemberTypes validates the --member-types slice (already CSV-split by
// cobra) into a lowercased, deduped CSV string. Empty input is a no-op (return
// the API's default of all types). Any element outside {user, bot} is rejected.
func normalizeMemberTypes(raw []string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "user" && p != "bot" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --member-types value %q: expected one of user, bot", p).WithParam("--member-types")
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return strings.Join(out, ","), nil
}

// warnIfConflictingPagingFlags mirrors the wiki list shortcuts: --page-token
// wins (single-page fetch from the supplied cursor) and --page-all is ignored.
func warnIfConflictingPagingFlags(runtime *common.RuntimeContext) {
	if strings.TrimSpace(runtime.Str("page-token")) != "" && runtime.Bool("page-all") {
		fmt.Fprintln(runtime.IO().ErrOut,
			"warning: --page-token is set, so --page-all is ignored (single-page fetch from the supplied cursor)")
	}
}

// writeChatMembersTruncationWarning emits a stderr warning for every
// server-side bucket cap reported in truncations[]. It uses the repo's plain
// "warning: <code>: <message>" convention (see shortcuts/common/runner.go and
// +chat-list's bot_strip_p2p) — no emoji, so it stays legible in CI logs and
// pipes regardless of terminal encoding.
func writeChatMembersTruncationWarning(w io.Writer, truncations []interface{}) {
	for _, t := range truncations {
		tm, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		memberType := valueOrAll(tm["member_type"])
		limit := tm["limit"]
		fmt.Fprintf(w, "warning: members_truncated: %s bucket capped at %v by server security config; the member list is INCOMPLETE\n", memberType, limit)
	}
}

func valueOrAll(v interface{}) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return "member"
}

func renderChatMembersPretty(w io.Writer, chatID string, res *chatMembersResult) {
	fmt.Fprintf(w, "Chat: %s\n", chatID)
	// Show the server-wide total next to the fetched count: when truncated or
	// paged, total can far exceed len(users)/len(bots), and that gap is exactly
	// what tells the reader how incomplete the list is.
	fmt.Fprintf(w, "Users (%d%s):\n", len(res.users), totalSuffix(res.userTotal, len(res.users)))
	for i, u := range res.users {
		m, _ := u.(map[string]interface{})
		fmt.Fprintf(w, "  [%d] %s  %s\n", i+1, valueOrDash(m["member_id"]), valueOrDash(m["name"]))
	}
	fmt.Fprintf(w, "Bots (%d%s):\n", len(res.bots), totalSuffix(res.botTotal, len(res.bots)))
	for i, b := range res.bots {
		m, _ := b.(map[string]interface{})
		fmt.Fprintf(w, "  [%d] %s  %s\n", i+1, valueOrDash(m["member_id"]), valueOrDash(m["name"]))
	}
	if len(res.truncations) > 0 {
		fmt.Fprintln(w, "warning: result truncated by server security config (see truncations[]); the list is INCOMPLETE")
	}
	if res.hasMore {
		fmt.Fprint(w, "More pages available; pass --page-all (and --page-limit 0 for everything)")
		if res.pageToken != "" {
			fmt.Fprintf(w, ", or --page-token %s to resume", res.pageToken)
		}
		fmt.Fprintln(w)
	}
}

func valueOrDash(v interface{}) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return "-"
}

// totalSuffix renders " of <total>" when the server-reported total exceeds the
// number actually fetched (so a truncated/partial bucket is obvious), and ""
// when the total is absent or already matches the fetched count.
func totalSuffix(total interface{}, fetched int) string {
	n, ok := toInt(total)
	if !ok || n <= fetched {
		return ""
	}
	return fmt.Sprintf(" of %d", n)
}

// toInt coerces a JSON-decoded number (float64 / json.Number / int) to int.
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}
