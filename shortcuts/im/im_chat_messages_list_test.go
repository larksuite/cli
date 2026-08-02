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

// TestChatMessagesList_LegacySortParity proves the command-owned compatibility
// stage resolves historical --sort to the canonical --order value.
func TestChatMessagesList_LegacySortParity(t *testing.T) {
	for _, dir := range []string{"asc", "desc"} {
		t.Run(dir, func(t *testing.T) {
			newRT, _ := newMountedIMRuntime(t, &ImChatMessageList, "--chat-id", "oc_test", "--order", dir)
			oldRT, _ := newMountedIMRuntime(t, &ImChatMessageList, "--chat-id", "oc_test", "--sort", dir)
			a := mustMarshalDryRun(t, ImChatMessageList.DryRun(context.Background(), newRT))
			b := mustMarshalDryRun(t, ImChatMessageList.DryRun(context.Background(), oldRT))
			if a != b {
				t.Fatalf("alias parity broken:\n new=%s\n old=%s", a, b)
			}
		})
	}
}

func TestChatMessagesList_CanonicalOrderWinsOverLegacySort(t *testing.T) {
	for _, args := range [][]string{
		{"--order", "asc", "--sort", "desc"},
		{"--sort", "desc", "--order", "asc"},
	} {
		rt, _ := newMountedIMRuntime(t, &ImChatMessageList, args...)
		params, err := buildChatMessageListRequest(rt, "oc_test")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got := params["sort_type"][0]; got != "ByCreateTimeAsc" {
			t.Fatalf("canonical --order must win for %v: sort_type=%s", args, got)
		}
	}
}

func TestChatMessagesList_OrderFlagSurface(t *testing.T) {
	var orderFlag, sortFlag *common.Flag
	for i := range ImChatMessageList.Flags {
		if ImChatMessageList.Flags[i].Name == "order" {
			orderFlag = &ImChatMessageList.Flags[i]
		}
		if ImChatMessageList.Flags[i].Name == "sort" {
			sortFlag = &ImChatMessageList.Flags[i]
		}
	}
	if orderFlag == nil {
		t.Fatal("expected canonical --order declaration")
	}
	if orderFlag.Default != "desc" {
		t.Errorf("--order Default = %q, want desc", orderFlag.Default)
	}
	if got := strings.Join(orderFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--order Enum = %q, want asc,desc", got)
	}
	if got := strings.Join(orderFlag.Aliases, ","); got != "sort-order" {
		t.Errorf("--order Aliases = %q, want sort-order", got)
	}
	if sortFlag == nil || !sortFlag.Hidden {
		t.Fatal("historical --sort must remain an independent hidden compatibility flag")
	}
	if got := strings.Join(sortFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--sort Enum = %q, want asc,desc", got)
	}
}
