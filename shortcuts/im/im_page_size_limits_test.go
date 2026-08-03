// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

type imPageSizeLimitCase struct {
	shortcut common.Shortcut
	flags    map[string]string
	limit    int
}

func imPageSizeLimitCases() []imPageSizeLimitCase {
	return []imPageSizeLimitCase{
		{shortcut: ImThreadsMessagesList, flags: map[string]string{"thread": "omt_test"}, limit: 50},
		{shortcut: ImChatMessageList, flags: map[string]string{"chat-id": "oc_test"}, limit: 50},
		{shortcut: ImMessagesSearch, flags: map[string]string{"query": "test"}, limit: 50},
		{shortcut: ImFlagList, flags: map[string]string{}, limit: 50},
		{shortcut: ImFeedGroupList, flags: map[string]string{}, limit: 50},
		{shortcut: ImFeedGroupListItem, flags: map[string]string{"feed-group-id": "ofg_test"}, limit: 50},
		{shortcut: ImChatSearch, flags: map[string]string{"query": "test"}, limit: 100},
		{shortcut: ImChatList, flags: map[string]string{}, limit: 100},
		{shortcut: ImChatMembersList, flags: map[string]string{"chat-id": "oc_test"}, limit: 100},
	}
}

func TestIMPageSizeLimitsTable(t *testing.T) {
	want := map[string]int{
		"+threads-messages-list": 50,
		"+chat-messages-list":    50,
		"+messages-search":       50,
		"+flag-list":             50,
		"+feed-group-list":       50,
		"+feed-group-list-item":  50,
		"+chat-search":           100,
		"+chat-list":             100,
		"+chat-members-list":     100,
	}
	if !reflect.DeepEqual(imPageSizeLimits, want) {
		t.Fatalf("imPageSizeLimits = %#v, want %#v", imPageSizeLimits, want)
	}
}

func TestIMPageSizeFlagsMatchLimitsTable(t *testing.T) {
	for _, tc := range imPageSizeLimitCases() {
		t.Run(tc.shortcut.Command, func(t *testing.T) {
			if got := imPageSizeLimit(tc.shortcut.Command); got != tc.limit {
				t.Fatalf("imPageSizeLimit(%q) = %d, want %d", tc.shortcut.Command, got, tc.limit)
			}
			var pageSizeFlag *common.Flag
			for i := range tc.shortcut.Flags {
				if tc.shortcut.Flags[i].Name == "page-size" {
					pageSizeFlag = &tc.shortcut.Flags[i]
					break
				}
			}
			if pageSizeFlag == nil {
				t.Fatal("page-size flag is missing")
			}
			if want := imPageSizeDescription(tc.shortcut.Command); pageSizeFlag.Desc != want {
				t.Fatalf("page-size description = %q, want %q", pageSizeFlag.Desc, want)
			}
		})
	}
}

func TestIMPageSizeValidationAcceptsLimitAndRejectsNextValue(t *testing.T) {
	for _, tc := range imPageSizeLimitCases() {
		t.Run(tc.shortcut.Command, func(t *testing.T) {
			for _, test := range []struct {
				name      string
				pageSize  int
				wantError bool
			}{
				{name: "accepts-server-limit", pageSize: tc.limit},
				{name: "rejects-limit-plus-one", pageSize: tc.limit + 1, wantError: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					requestCount := 0
					runtime := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						requestCount++
						t.Fatalf("validation sent an HTTP request: %s %s", req.Method, req.URL.String())
						return nil, nil
					}))
					flags := mergeListPageAllFlags(tc.flags, map[string]string{"page-size": fmt.Sprintf("%d", test.pageSize)})
					runtime.Cmd = newListPageAllCommand(t, tc.shortcut, flags)

					err := tc.shortcut.Validate(context.Background(), runtime)
					if !test.wantError {
						if err != nil {
							t.Fatalf("Validate() error = %v", err)
						}
					} else {
						assertValidationError(t, tc.shortcut.Command, err, "--page-size")
						wantMessage := fmt.Sprintf("invalid --page-size %d: must be between 1 and %d", test.pageSize, tc.limit)
						if err.Error() != wantMessage {
							t.Fatalf("Validate() error = %q, want %q", err.Error(), wantMessage)
						}
					}
					if requestCount != 0 {
						t.Fatalf("HTTP request count = %d, want 0", requestCount)
					}
				})
			}
		})
	}
}
