// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// page builds one list_members page payload shaped like the data object the
// server returns (users[]/bots[]/truncations[] plus paging + totals).
func cmlPage(users, bots, truncations []interface{}, hasMore bool, pageToken string) map[string]interface{} {
	return map[string]interface{}{
		"users":       users,
		"bots":        bots,
		"truncations": truncations,
		"has_more":    hasMore,
		"page_token":  pageToken,
		"user_total":  324,
		"bot_total":   2,
	}
}

func us(ids ...string) []interface{} {
	out := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]interface{}{"member_id": id})
	}
	return out
}

// TestMergeChatMemberPages_MergesUsersAndBots covers Bug 1: every list bucket
// (users AND bots) must be concatenated across pages, not just one of them.
func TestMergeChatMemberPages_MergesUsersAndBots(t *testing.T) {
	pages := []map[string]interface{}{
		cmlPage(us("u1", "u2"), us("b1"), []interface{}{}, true, "p2"),
		cmlPage(us("u3"), us("b2", "b3"), []interface{}{}, false, ""),
	}

	res := mergeChatMemberPages(pages)

	if len(res.users) != 3 {
		t.Errorf("users: want 3 merged, got %d", len(res.users))
	}
	if len(res.bots) != 3 {
		t.Errorf("bots: want 3 merged, got %d", len(res.bots))
	}
}

// TestMergeChatMemberPages_TruncationsFromLastPage covers Bug 2: truncations[]
// is emitted only on the final page, so the merged view must take it from the
// last page rather than inherit page 1's empty slice.
func TestMergeChatMemberPages_TruncationsFromLastPage(t *testing.T) {
	limit := []interface{}{map[string]interface{}{"limit": 100, "member_type": "user"}}
	pages := []map[string]interface{}{
		cmlPage(us("u1"), us("b1"), []interface{}{}, true, "p2"),
		cmlPage(us("u2"), nil, limit, false, ""),
	}

	res := mergeChatMemberPages(pages)

	if len(res.truncations) != 1 {
		t.Fatalf("truncations: want last page's 1 entry, got %d (%v)", len(res.truncations), res.truncations)
	}
}

// TestMergeChatMemberPages_HasMoreAndTokenFromLastPage guards that paging
// signals come from the final page (so a --page-limit cutoff is visible).
func TestMergeChatMemberPages_HasMoreAndTokenFromLastPage(t *testing.T) {
	pages := []map[string]interface{}{
		cmlPage(us("u1"), nil, nil, true, "p2"),
		cmlPage(us("u2"), nil, nil, true, "p3"), // loop stopped early; server still has more
	}

	res := mergeChatMemberPages(pages)

	if !res.hasMore {
		t.Error("has_more: want true from last page")
	}
	if res.pageToken != "p3" {
		t.Errorf("page_token: want last page's p3, got %q", res.pageToken)
	}
}

// TestMergeChatMemberPages_TotalsFromLastPage verifies user_total / bot_total
// are taken from the final page (not an earlier, possibly-different value).
func TestMergeChatMemberPages_TotalsFromLastPage(t *testing.T) {
	pages := []map[string]interface{}{
		{"users": us("u1"), "user_total": 999, "bot_total": 7, "has_more": true, "page_token": "p2"},
		{"users": us("u2"), "user_total": 324, "bot_total": 2, "has_more": false, "page_token": ""},
	}
	res := mergeChatMemberPages(pages)
	if n, _ := toInt(res.userTotal); n != 324 {
		t.Errorf("user_total: want last page's 324, got %v", res.userTotal)
	}
	if n, _ := toInt(res.botTotal); n != 2 {
		t.Errorf("bot_total: want last page's 2, got %v", res.botTotal)
	}
}

// TestChatMembersValidate covers --chat-id presence + oc_ prefix enforcement.
func TestChatMembersValidate(t *testing.T) {
	noop := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": cmlPage(nil, nil, nil, false, "")}), nil
	})
	cases := []struct {
		name    string
		chatID  string
		wantErr bool
	}{
		{"valid oc_", "oc_abc", false},
		{"empty", "", true},
		{"missing oc_ prefix", "abc123", true},
	}
	for _, c := range cases {
		rt := newChatMembersTestRuntime(t, noop, map[string]string{"chat-id": c.chatID}, nil, nil)
		err := ImChatMembersList.Validate(context.Background(), rt)
		if c.wantErr {
			assertValidationError(t, c.name, err, "--chat-id")
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
	}
}

// assertValidationError checks err satisfies the repo's typed-error contract for
// a validation failure: a *errs.ValidationError carrying the expected Param, and
// problem metadata of category validation / subtype invalid_argument.
func assertValidationError(t *testing.T, ctx string, err error, wantParam string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("%s: want *errs.ValidationError, got %T (%v)", ctx, err, err)
		return
	}
	if ve.Param != wantParam {
		t.Errorf("%s: Param = %q, want %q", ctx, ve.Param, wantParam)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("%s: problem = %+v (ok=%v), want category=%s subtype=%s", ctx, p, ok, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}
}

func TestNormalizeMemberTypes(t *testing.T) {
	cases := []struct {
		in      []string
		want    string
		wantErr bool
	}{
		{nil, "", false},
		{[]string{"user", "bot"}, "user,bot", false},
		{[]string{"USER", "user"}, "user", false}, // lowercased + deduped
		{[]string{"admin"}, "", true},
		{[]string{""}, "", true},
	}
	for _, c := range cases {
		got, err := normalizeMemberTypes(c.in)
		if c.wantErr {
			assertValidationError(t, fmt.Sprintf("normalizeMemberTypes(%v)", c.in), err, "--member-types")
			continue
		}
		if err != nil {
			t.Errorf("normalizeMemberTypes(%v): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("normalizeMemberTypes(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEffectiveChatMembersPageSize covers the --page-all max-page-size behavior:
// drain with no explicit size → max; explicit size → honored; single page → default.
func TestEffectiveChatMembersPageSize(t *testing.T) {
	noop := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": cmlPage(nil, nil, nil, false, "")}), nil
	})
	cases := []struct {
		name string
		b    map[string]bool
		ints map[string]int
		want int
	}{
		{"page-all, size unset -> max", map[string]bool{"page-all": true}, nil, chatMembersListMaxPageSize},
		{"page-all, size explicit -> honored", map[string]bool{"page-all": true}, map[string]int{"page-size": 15}, 15},
		{"single page, size unset -> default", nil, nil, chatMembersListDefaultPageSize},
	}
	for _, c := range cases {
		rt := newChatMembersTestRuntime(t, noop, map[string]string{"chat-id": "oc_x"}, c.b, c.ints)
		if got := effectiveChatMembersPageSize(rt); got != c.want {
			t.Errorf("%s: want %d, got %d", c.name, c.want, got)
		}
	}
}

// newChatMembersTestRuntime registers the shortcut's flags and returns a
// user-identity runtime wired to the given RoundTripper for multi-page mocking.
func newChatMembersTestRuntime(t *testing.T, rt http.RoundTripper, str map[string]string, b map[string]bool, ints map[string]int) *common.RuntimeContext {
	t.Helper()
	runtime := newUserShortcutRuntime(t, rt)
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("chat-id", "", "")
	cmd.Flags().String("member-id-type", "open_id", "")
	cmd.Flags().StringSlice("member-types", nil, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().Int("page-limit", 10, "")
	cmd.Flags().Int("page-delay", 200, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	for k, v := range str {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	for k, v := range b {
		if err := cmd.Flags().Set(k, strconv.FormatBool(v)); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	for k, v := range ints {
		if err := cmd.Flags().Set(k, strconv.Itoa(v)); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	runtime.Cmd = cmd
	return runtime
}

// TestFetchChatMembers_PageAllMergesBucketsAndTruncations exercises the full
// fetch loop over mocked pages: users/bots merge across pages and the final
// page's truncations[] survives.
func TestFetchChatMembers_PageAllMergesBucketsAndTruncations(t *testing.T) {
	calls := 0
	rt := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/oc_test/members/list") {
			return shortcutJSONResponse(404, map[string]interface{}{"code": 1}), nil
		}
		calls++
		token := req.URL.Query().Get("page_token")
		if token == "" {
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": cmlPage(us("u1", "u2"), us("b1"), []interface{}{}, true, "p2"),
			}), nil
		}
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": cmlPage(us("u3"), us("b2"), []interface{}{map[string]interface{}{"limit": 100, "member_type": "user"}}, false, ""),
		}), nil
	})
	runtime := newChatMembersTestRuntime(t, rt,
		map[string]string{"chat-id": "oc_test"},
		map[string]bool{"page-all": true},
		map[string]int{"page-size": 2, "page-limit": 0, "page-delay": 0})

	res, err := fetchChatMembers(context.Background(), runtime, "oc_test")
	if err != nil {
		t.Fatalf("fetchChatMembers: %v", err)
	}
	if calls != 2 {
		t.Errorf("want 2 page calls, got %d", calls)
	}
	if len(res.users) != 3 {
		t.Errorf("users: want 3, got %d", len(res.users))
	}
	if len(res.bots) != 2 {
		t.Errorf("bots: want 2, got %d", len(res.bots))
	}
	if len(res.truncations) != 1 {
		t.Errorf("truncations: want 1 from last page, got %d", len(res.truncations))
	}
	if res.hasMore {
		t.Error("has_more: want false after draining all pages")
	}
}

// TestFetchChatMembers_PageLimitStops verifies --page-limit caps the loop and
// leaves has_more=true so the caller knows the result is incomplete.
func TestFetchChatMembers_PageLimitStops(t *testing.T) {
	seq := 0
	rt := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Every page reports more pages available, with an advancing token so the
		// loop is stopped by --page-limit, not the non-advancing-token guard.
		seq++
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": cmlPage(us("u"), nil, nil, true, fmt.Sprintf("p%d", seq)),
		}), nil
	})
	runtime := newChatMembersTestRuntime(t, rt,
		map[string]string{"chat-id": "oc_test"},
		map[string]bool{"page-all": true},
		map[string]int{"page-size": 1, "page-limit": 3, "page-delay": 0})

	res, err := fetchChatMembers(context.Background(), runtime, "oc_test")
	if err != nil {
		t.Fatalf("fetchChatMembers: %v", err)
	}
	if len(res.users) != 3 {
		t.Errorf("users: want 3 (capped at page-limit), got %d", len(res.users))
	}
	if !res.hasMore {
		t.Error("has_more: want true (loop cut short by page-limit)")
	}
	errOut := runtime.IO().ErrOut.(*bytes.Buffer)
	if !strings.Contains(errOut.String(), "reached page limit (3)") {
		t.Errorf("want page-limit notice on stderr, got: %s", errOut.String())
	}
}
