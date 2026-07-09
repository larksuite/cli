// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// newMsgListTestRT registers chat-id (so the builder has a container) plus the
// sort flags under test; only flags present in stringFlags are "set" (Changed).
func newMsgListTestRT(t *testing.T, stringFlags map[string]string) *common.RuntimeContext {
	t.Helper()
	if stringFlags == nil {
		stringFlags = map[string]string{}
	}
	if _, ok := stringFlags["chat-id"]; !ok {
		stringFlags["chat-id"] = "oc_test"
	}
	return newChatListTestRuntimeContext(t, stringFlags, nil)
}

func TestChatMessagesList_OrderMapping(t *testing.T) {
	cases := []struct{ order, want string }{
		{"asc", "ByCreateTimeAsc"},
		{"desc", "ByCreateTimeDesc"},
	}
	for _, c := range cases {
		t.Run(c.order, func(t *testing.T) {
			rt := newMsgListTestRT(t, map[string]string{"order": c.order})
			params, err := buildChatMessageListRequest(rt, "oc_test")
			if err != nil {
				t.Fatalf("buildChatMessageListRequest() error = %v", err)
			}
			if got := params["sort_type"][0]; got != c.want {
				t.Fatalf("order=%s -> sort_type=%s, want %s", c.order, got, c.want)
			}
		})
	}
}

// TestChatMessagesList_OrderAliasParity: hidden --sort alias (asc/desc) must map
// through the SAME table as --order (NOT pass through), producing identical upstream.
func TestChatMessagesList_OrderAliasParity(t *testing.T) {
	for _, dir := range []string{"asc", "desc"} {
		t.Run(dir, func(t *testing.T) {
			newRT := newMsgListTestRT(t, map[string]string{"order": dir})
			oldRT := newMsgListTestRT(t, map[string]string{"sort": dir})
			a := mustMarshalDryRun(t, ImChatMessageList.DryRun(context.Background(), newRT))
			b := mustMarshalDryRun(t, ImChatMessageList.DryRun(context.Background(), oldRT))
			if a != b {
				t.Fatalf("alias parity broken:\n new=%s\n old=%s", a, b)
			}
		})
	}
}

func TestChatMessagesList_OrderNewWins(t *testing.T) {
	rt := newMsgListTestRT(t, map[string]string{"order": "asc", "sort": "desc"})
	params, err := buildChatMessageListRequest(rt, "oc_test")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got := params["sort_type"][0]; got != "ByCreateTimeAsc" {
		t.Fatalf("new should win: sort_type=%s, want ByCreateTimeAsc", got)
	}
}

func TestChatMessagesList_OrderFlagSurface(t *testing.T) {
	var orderFlag, aliasFlag *common.Flag
	for i := range ImChatMessageList.Flags {
		switch ImChatMessageList.Flags[i].Name {
		case "order":
			orderFlag = &ImChatMessageList.Flags[i]
		case "sort":
			aliasFlag = &ImChatMessageList.Flags[i]
		}
	}
	if orderFlag == nil || aliasFlag == nil {
		t.Fatalf("expected both --order and --sort flags declared")
	}
	if orderFlag.Default != "desc" {
		t.Errorf("--order Default = %q, want desc", orderFlag.Default)
	}
	if got := strings.Join(orderFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--order Enum = %q, want asc,desc", got)
	}
	if !aliasFlag.Hidden {
		t.Errorf("--sort must be Hidden")
	}
	if got := strings.Join(aliasFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--sort (alias) Enum = %q, want asc,desc", got)
	}
}
