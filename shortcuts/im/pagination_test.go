// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestIMReadShortcutsExposeUniformPaginationFlags(t *testing.T) {
	t.Parallel()

	shortcuts := []common.Shortcut{
		ImChatList,
		ImChatMembersList,
		ImChatMessageList,
		ImChatSearch,
		ImFeedGroupList,
		ImFeedGroupListItem,
		ImFeedShortcutList,
		ImFlagList,
		ImMessagesSearch,
		ImThreadsMessagesList,
	}
	for _, shortcut := range shortcuts {
		t.Run(shortcut.Command, func(t *testing.T) {
			t.Parallel()
			flags := make(map[string]common.Flag, len(shortcut.Flags))
			for _, flag := range shortcut.Flags {
				flags[flag.Name] = flag
			}
			for _, name := range []string{"page-all", "page-limit"} {
				if _, ok := flags[name]; !ok {
					t.Fatalf("%s does not expose --%s", shortcut.Command, name)
				}
			}
			if flags["page-limit"].Type != "int" {
				t.Fatalf("%s --page-limit type = %q, want int", shortcut.Command, flags["page-limit"].Type)
			}
		})
	}
}

func requireIMReadEnvelopeMeta(t *testing.T, envelope map[string]any) (map[string]any, map[string]any) {
	t.Helper()
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want object", envelope["meta"])
	}
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("meta.pagination = %#v, want object", meta["pagination"])
	}
	notice, ok := envelope["_notice"].(map[string]any)
	if !ok {
		t.Fatalf("_notice = %#v, want object", envelope["_notice"])
	}
	readNotice, ok := notice["im_read"].(map[string]any)
	if !ok {
		t.Fatalf("_notice.im_read = %#v, want object", notice["im_read"])
	}
	return pagination, readNotice
}

func TestValidateIMPaginationAcceptsUnlimitedAndRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, 1, 20} {
		rt := newIMPaginationTestRuntime(t, false, limit, "", 0)
		if err := validateIMPagination(rt); err != nil {
			t.Fatalf("validateIMPagination(page-limit=%d) error = %v", limit, err)
		}
	}

	rt := newIMPaginationTestRuntime(t, false, -1, "", 0)
	err := validateIMPagination(rt)
	if err == nil || !strings.Contains(err.Error(), "--page-limit") {
		t.Fatalf("validateIMPagination(page-limit=-1) error = %v, want --page-limit validation error", err)
	}
}

func TestCollectIMPagesStopReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pageAll    bool
		pageLimit  int
		startToken string
		pages      []map[string]any
		wantReason string
		wantCount  int
		wantMore   bool
		wantToken  string
	}{
		{
			name:       "default exhausted",
			pageLimit:  20,
			pages:      []map[string]any{{"items": []any{"a"}, "has_more": false}},
			wantReason: "exhausted",
			wantCount:  1,
		},
		{
			name:       "default single page",
			pageLimit:  20,
			pages:      []map[string]any{{"items": []any{"a"}, "has_more": true, "page_token": "next"}},
			wantReason: "single_page",
			wantCount:  1,
			wantMore:   true,
			wantToken:  "next",
		},
		{
			name:       "page all exhausted",
			pageAll:    true,
			pageLimit:  0,
			pages:      []map[string]any{{"has_more": true, "page_token": "p2"}, {"has_more": false}},
			wantReason: "exhausted",
			wantCount:  2,
		},
		{
			name:       "page limit",
			pageAll:    true,
			pageLimit:  1,
			pages:      []map[string]any{{"has_more": true, "page_token": "p2"}},
			wantReason: "page_limit",
			wantCount:  1,
			wantMore:   true,
			wantToken:  "p2",
		},
		{
			name:       "explicit start token",
			pageAll:    true,
			pageLimit:  0,
			startToken: "middle",
			pages:      []map[string]any{{"has_more": false}},
			wantReason: "start_page_token",
			wantCount:  1,
		},
		{
			name:       "server truncation",
			pageAll:    true,
			pageLimit:  0,
			pages:      []map[string]any{{"has_more": false, "truncations": []any{map[string]any{"type": "user"}}}},
			wantReason: "server_truncation",
			wantCount:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newIMPaginationTestRuntime(t, tt.pageAll, tt.pageLimit, tt.startToken, 0)
			call := 0
			gotPages, status, err := collectIMPages(rt, tt.pageAll, func(token string) (map[string]any, error) {
				if call >= len(tt.pages) {
					t.Fatalf("unexpected page fetch %d with token %q", call+1, token)
				}
				page := tt.pages[call]
				call++
				return page, nil
			})
			if err != nil {
				t.Fatalf("collectIMPages() error = %v", err)
			}
			if len(gotPages) != tt.wantCount || status.PagesFetched != tt.wantCount {
				t.Fatalf("pages = %d, status.PagesFetched = %d, want %d", len(gotPages), status.PagesFetched, tt.wantCount)
			}
			if string(status.StopReason) != tt.wantReason {
				t.Fatalf("StopReason = %q, want %q", status.StopReason, tt.wantReason)
			}
			if status.HasMore != tt.wantMore || status.NextPageToken != tt.wantToken {
				t.Fatalf("pagination tail = (%v, %q), want (%v, %q)", status.HasMore, status.NextPageToken, tt.wantMore, tt.wantToken)
			}
		})
	}
}

func TestCollectIMPagesRejectsMissingAndRepeatedTokensWithoutLeakingThem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pages      []map[string]any
		wantReason string
	}{
		{
			name:       "missing",
			pages:      []map[string]any{{"has_more": true}},
			wantReason: "missing_token",
		},
		{
			name: "repeated",
			pages: []map[string]any{
				{"has_more": true, "page_token": "secret-token"},
				{"has_more": true, "page_token": "secret-token"},
			},
			wantReason: "repeated_token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newIMPaginationTestRuntime(t, true, 0, "", 0)
			call := 0
			gotPages, status, err := collectIMPages(rt, true, func(string) (map[string]any, error) {
				page := tt.pages[call]
				call++
				return page, nil
			})
			if err == nil {
				t.Fatal("collectIMPages() error = nil")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %#v, want typed invalid_response", err)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error leaks page token: %v", err)
			}
			if string(status.StopReason) != tt.wantReason || status.Cause == nil {
				t.Fatalf("status = %#v, want reason %q with cause", status, tt.wantReason)
			}
			if len(gotPages) != call {
				t.Fatalf("returned pages = %d, want %d", len(gotPages), call)
			}
		})
	}
}

func TestCollectIMPagesValidatesTokensBeforeNonFullReadStops(t *testing.T) {
	tests := []struct {
		name       string
		startToken string
		page       map[string]any
		wantReason client.StopReason
	}{
		{
			name:       "default single page missing token",
			page:       map[string]any{"has_more": true},
			wantReason: client.StopReasonMissingToken,
		},
		{
			name:       "explicit start token repeats",
			startToken: "opaque-start",
			page:       map[string]any{"has_more": true, "page_token": "opaque-start"},
			wantReason: client.StopReasonRepeatedToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newIMPaginationTestRuntime(t, false, imReadDefaultPageLimit, tt.startToken, 0)
			pages, status, err := collectIMPages(rt, false, func(string) (map[string]any, error) {
				return tt.page, nil
			})
			if err == nil {
				t.Fatal("collectIMPages() error = nil, want invalid_response")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal ||
				problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
			}
			if len(pages) != 1 || status.PagesFetched != 1 || status.StopReason != tt.wantReason {
				t.Fatalf("pages/status = %d/%#v, want one page and %q", len(pages), status, tt.wantReason)
			}
			if strings.Contains(err.Error(), tt.startToken) && tt.startToken != "" {
				t.Fatalf("error leaked page token: %v", err)
			}
		})
	}
}

func TestCollectIMPagesNonFullReadStillReportsNaturalExhaustion(t *testing.T) {
	rt := newIMPaginationTestRuntime(t, false, imReadDefaultPageLimit, "", 0)
	pages, status, err := collectIMPages(rt, false, func(string) (map[string]any, error) {
		return map[string]any{"has_more": false}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || status.StopReason != client.StopReasonExhausted {
		t.Fatalf("pages/status = %d/%#v", len(pages), status)
	}
}

func TestCollectIMPagesPreservesPartialPagesOnTypedFailure(t *testing.T) {
	t.Parallel()

	wantErr := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "request timed out").WithRetryable()
	rt := newIMPaginationTestRuntime(t, true, 0, "", 0)
	call := 0
	pages, status, err := collectIMPages(rt, true, func(string) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{"items": []any{"a"}, "has_more": true, "page_token": "p2"}, nil
		}
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(pages) != 1 || status.PagesFetched != 1 {
		t.Fatalf("pages/status = %d/%d, want 1/1", len(pages), status.PagesFetched)
	}
	if string(status.StopReason) != "transport_error" || status.Cause == nil {
		t.Fatalf("status = %#v, want transport_error with cause", status)
	}
}

func TestMergeIMPageArraysPreservesAllBucketsAndLastPageCursor(t *testing.T) {
	t.Parallel()

	got := mergeIMPageArrays([]map[string]any{
		{
			"items":         []any{"a"},
			"deleted_items": []any{"d1"},
			"notice":        "first notice",
			"has_more":      true,
			"page_token":    "p2",
		},
		{
			"items":         []any{"b"},
			"deleted_items": []any{"d2"},
			"has_more":      false,
			"page_token":    "",
		},
	}, "items", "deleted_items")

	if gotItems, _ := got["items"].([]any); len(gotItems) != 2 {
		t.Fatalf("items = %#v, want both pages", got["items"])
	}
	if gotDeleted, _ := got["deleted_items"].([]any); len(gotDeleted) != 2 {
		t.Fatalf("deleted_items = %#v, want both pages", got["deleted_items"])
	}
	if got["notice"] != "first notice" {
		t.Fatalf("notice = %#v, want first-page value", got["notice"])
	}
	if got["has_more"] != false || got["page_token"] != "" {
		t.Fatalf("tail pagination = (%#v, %#v), want (false, empty)", got["has_more"], got["page_token"])
	}
}

func TestMergeIMPageArraysDoesNotLeakEarlierPageTokenWhenFinalPageOmitsIt(t *testing.T) {
	t.Parallel()

	got := mergeIMPageArrays([]map[string]any{
		{
			"items":           []any{"a"},
			"has_more":        true,
			"page_token":      "stale-page-token",
			"next_page_token": "stale-next-token",
		},
		{
			"items":    []any{"b"},
			"has_more": false,
		},
	}, "items")

	if got["has_more"] != false {
		t.Fatalf("has_more = %#v, want false from final page", got["has_more"])
	}
	for _, field := range []string{"page_token", "next_page_token"} {
		if value, exists := got[field]; exists {
			t.Fatalf("%s = %#v, want field omitted with no final-page token", field, value)
		}
	}
}

func TestIMNewlyPaginatedShortcutsWalkAllPages(t *testing.T) {
	tests := []struct {
		name       string
		command    func(t *testing.T) *cobra.Command
		response   func(page int) map[string]any
		execute    func(*common.RuntimeContext) error
		wantMethod string
	}{
		{
			name:    "chat list",
			command: newPaginatedChatListCommand,
			response: func(page int) map[string]any {
				return map[string]any{"items": []any{map[string]any{"chat_id": fmt.Sprintf("oc_%d", page)}}, "has_more": page == 1, "page_token": nextTestToken(page)}
			},
			execute: func(rt *common.RuntimeContext) error {
				return ImChatList.Execute(context.Background(), rt)
			},
			wantMethod: http.MethodGet,
		},
		{
			name:    "chat search",
			command: newPaginatedChatSearchCommand,
			response: func(page int) map[string]any {
				return map[string]any{
					"items": []any{map[string]any{"meta_data": map[string]any{"chat_id": fmt.Sprintf("oc_%d", page)}}},
					"total": float64(2), "has_more": page == 1, "page_token": nextTestToken(page),
				}
			},
			execute: func(rt *common.RuntimeContext) error {
				return ImChatSearch.Execute(context.Background(), rt)
			},
			wantMethod: http.MethodPost,
		},
		{
			name:    "chat messages",
			command: newPaginatedChatMessagesCommand,
			response: func(page int) map[string]any {
				return map[string]any{"items": []any{}, "has_more": page == 1, "page_token": nextTestToken(page)}
			},
			execute: func(rt *common.RuntimeContext) error {
				return ImChatMessageList.Execute(context.Background(), rt)
			},
			wantMethod: http.MethodGet,
		},
		{
			name:    "thread messages",
			command: newPaginatedThreadMessagesCommand,
			response: func(page int) map[string]any {
				return map[string]any{"items": []any{}, "has_more": page == 1, "page_token": nextTestToken(page)}
			},
			execute: func(rt *common.RuntimeContext) error {
				return ImThreadsMessagesList.Execute(context.Background(), rt)
			},
			wantMethod: http.MethodGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokens []string
			rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != tt.wantMethod {
					t.Fatalf("method = %s, want %s", req.Method, tt.wantMethod)
				}
				tokens = append(tokens, req.URL.Query().Get("page_token"))
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": tt.response(len(tokens)),
				}), nil
			}))
			setRuntimeField(t, rt, "Cmd", tt.command(t))

			if err := tt.execute(rt); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := strings.Join(tokens, ","), ",p2"; got != want {
				t.Fatalf("page tokens = %q, want %q", got, want)
			}
		})
	}
}

func nextTestToken(page int) string {
	if page == 1 {
		return "p2"
	}
	return ""
}

func addUniformPaginationTestFlags(cmd *cobra.Command) {
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", true, "")
	cmd.Flags().Int("page-limit", 0, "")
	cmd.Flags().Int("page-delay", 0, "")
}

func newPaginatedChatListCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("user-id-type", "open_id", "")
	cmd.Flags().String("sort", "create_time", "")
	cmd.Flags().String("sort-type", "", "")
	cmd.Flags().StringSlice("types", nil, "")
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().Bool("exclude-muted", false, "")
	addUniformPaginationTestFlags(cmd)
	return cmd
}

func newPaginatedChatSearchCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{"query", "search-types", "chat-modes", "member-ids", "sort", "sort-by"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("is-manager", false, "")
	cmd.Flags().Bool("disable-search-by-user", false, "")
	cmd.Flags().Bool("exclude-muted", false, "")
	cmd.Flags().Int("page-size", 20, "")
	addUniformPaginationTestFlags(cmd)
	return cmd
}

func newPaginatedChatMessagesCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{"user-id", "start", "end", "sort"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().String("chat-id", "oc_test", "")
	cmd.Flags().String("order", "desc", "")
	cmd.Flags().String("page-size", "50", "")
	cmd.Flags().Bool("no-reactions", true, "")
	cmd.Flags().Bool("download-resources", false, "")
	addUniformPaginationTestFlags(cmd)
	return cmd
}

func newPaginatedThreadMessagesCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("thread", "omt_test", "")
	cmd.Flags().String("order", "asc", "")
	cmd.Flags().String("sort", "", "")
	cmd.Flags().String("page-size", "50", "")
	cmd.Flags().Bool("no-reactions", true, "")
	cmd.Flags().Bool("download-resources", false, "")
	addUniformPaginationTestFlags(cmd)
	return cmd
}

func newIMPaginationTestRuntime(t *testing.T, pageAll bool, pageLimit int, pageToken string, pageDelay int) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-limit", 20, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Int("page-delay", 0, "")
	if pageAll {
		if err := cmd.Flags().Set("page-all", "true"); err != nil {
			t.Fatal(err)
		}
	}
	if pageLimit != imReadDefaultPageLimit {
		if err := cmd.Flags().Set("page-limit", strconv.Itoa(pageLimit)); err != nil {
			t.Fatal(err)
		}
	}
	if pageToken != "" {
		if err := cmd.Flags().Set("page-token", pageToken); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Flags().Set("page-delay", strconv.Itoa(pageDelay)); err != nil {
		t.Fatal(err)
	}
	return &common.RuntimeContext{
		Cmd:    cmd,
		Config: &core.CliConfig{},
	}
}
