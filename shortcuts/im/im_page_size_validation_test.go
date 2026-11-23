// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

type imPageSizeValidationCase struct {
	shortcut    common.Shortcut
	flags       map[string]string
	defaultSize int
	maxSize     int
}

func imPageSizeValidationCases() []imPageSizeValidationCase {
	return []imPageSizeValidationCase{
		{shortcut: ImThreadsMessagesList, flags: map[string]string{"thread": "omt_test"}, defaultSize: threadsMessagesListDefaultPageSize, maxSize: threadsMessagesListMaxPageSize},
		{shortcut: ImChatMessageList, flags: map[string]string{"chat-id": "oc_test"}, defaultSize: chatMessagesListDefaultPageSize, maxSize: chatMessagesListMaxPageSize},
		{shortcut: ImMessagesSearch, flags: map[string]string{"query": "test"}, defaultSize: messagesSearchDefaultPageSize, maxSize: messagesSearchMaxPageSize},
		{shortcut: ImFlagList, defaultSize: flagListDefaultPageSize, maxSize: flagListMaxPageSize},
		{shortcut: ImFeedGroupList, defaultSize: feedGroupListDefaultPageSize, maxSize: feedGroupListMaxPageSize},
		{shortcut: ImFeedGroupListItem, flags: map[string]string{"feed-group-id": "ofg_test"}, defaultSize: feedGroupListItemDefaultPageSize, maxSize: feedGroupListItemMaxPageSize},
		{shortcut: ImChatSearch, flags: map[string]string{"query": "test"}, defaultSize: chatSearchDefaultPageSize, maxSize: chatSearchMaxPageSize},
		{shortcut: ImChatList, defaultSize: chatListDefaultPageSize, maxSize: chatListMaxPageSize},
		{shortcut: ImChatMembersList, flags: map[string]string{"chat-id": "oc_test"}, defaultSize: chatMembersListDefaultPageSize, maxSize: chatMembersListMaxPageSize},
		{shortcut: ImMessageReadUsers, flags: map[string]string{"message-id": "om_test"}, defaultSize: messageReadUsersDefaultPageSize, maxSize: messageReadUsersMaxPageSize},
	}
}

func TestIMPageSizeFlagContracts(t *testing.T) {
	covered := make(map[string]struct{})
	for _, tc := range imPageSizeValidationCases() {
		t.Run(tc.shortcut.Command, func(t *testing.T) {
			covered[tc.shortcut.Command] = struct{}{}
			pageSizeFlag := findIMPageSizeFlag(t, &tc.shortcut)
			if got, want := pageSizeFlag.Default, strconv.Itoa(tc.defaultSize); got != want {
				t.Fatalf("page-size default = %q, want %q", got, want)
			}
			if got, want := pageSizeFlag.Desc, fmt.Sprintf("page size (1-%d)", tc.maxSize); got != want {
				t.Fatalf("page-size description = %q, want %q", got, want)
			}
			if tc.defaultSize < 1 || tc.defaultSize > tc.maxSize {
				t.Fatalf("page-size default %d is outside 1-%d", tc.defaultSize, tc.maxSize)
			}
		})
	}

	for _, shortcut := range Shortcuts() {
		if !hasIMFlag(&shortcut, "page-size") {
			continue
		}
		if _, ok := covered[shortcut.Command]; !ok {
			t.Errorf("%s has --page-size but no boundary contract test", shortcut.Command)
		}
	}
}

func TestIMPageSizeValidationAcceptsMaximumAndRejectsNextValue(t *testing.T) {
	for _, tc := range imPageSizeValidationCases() {
		t.Run(tc.shortcut.Command, func(t *testing.T) {
			for _, test := range []struct {
				name      string
				pageSize  int
				wantError bool
			}{
				{name: "accepts-server-maximum", pageSize: tc.maxSize},
				{name: "rejects-maximum-plus-one", pageSize: tc.maxSize + 1, wantError: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					requestCount := 0
					runtime := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						requestCount++
						t.Fatalf("validation sent an HTTP request: %s %s", req.Method, req.URL.String())
						return nil, nil
					}))
					flags := mergeListPageAllFlags(tc.flags, map[string]string{"page-size": strconv.Itoa(test.pageSize)})
					runtime.Cmd = newListPageAllCommand(t, tc.shortcut, flags)

					err := tc.shortcut.Validate(context.Background(), runtime)
					if !test.wantError {
						if err != nil {
							t.Fatalf("Validate() error = %v", err)
						}
					} else {
						assertValidationError(t, tc.shortcut.Command, err, "--page-size")
						wantMessage := fmt.Sprintf("invalid --page-size %d: must be between 1 and %d", test.pageSize, tc.maxSize)
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

func findIMPageSizeFlag(t *testing.T, shortcut *common.Shortcut) *common.Flag {
	t.Helper()
	for i := range shortcut.Flags {
		if shortcut.Flags[i].Name == "page-size" {
			return &shortcut.Flags[i]
		}
	}
	t.Fatalf("%s is missing --page-size", shortcut.Command)
	return nil
}

func hasIMFlag(shortcut *common.Shortcut, name string) bool {
	for i := range shortcut.Flags {
		if shortcut.Flags[i].Name == name {
			return true
		}
	}
	return false
}
