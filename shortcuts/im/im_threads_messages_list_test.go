// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func newThreadsTestRT(t *testing.T, stringFlags map[string]string) *common.RuntimeContext {
	t.Helper()
	if stringFlags == nil {
		stringFlags = map[string]string{}
	}
	if _, ok := stringFlags["thread"]; !ok {
		if _, aliasSet := stringFlags["thread-id"]; !aliasSet {
			stringFlags["thread"] = "omt_test"
		}
	}
	return newChatListTestRuntimeContext(t, stringFlags, nil)
}

func TestThreadsMessagesList_OrderMapping(t *testing.T) {
	cases := []struct{ order, want string }{
		{"asc", "ByCreateTimeAsc"},
		{"desc", "ByCreateTimeDesc"},
	}
	for _, c := range cases {
		t.Run(c.order, func(t *testing.T) {
			got := buildThreadsMessagesListParams(c.order, "omt_test", 50, "")
			if v := got["sort_type"][0]; v != c.want {
				t.Fatalf("order=%s -> sort_type=%s, want %s", c.order, v, c.want)
			}
		})
	}
}

// TestThreadsMessagesList_LegacySortParity proves the compatibility stage maps
// historical --sort to canonical --order before command logic runs.
func TestThreadsMessagesList_LegacySortParity(t *testing.T) {
	for _, dir := range []string{"asc", "desc"} {
		t.Run(dir, func(t *testing.T) {
			newRT, _ := newMountedIMRuntime(t, &ImThreadsMessagesList, "--thread", "omt_test", "--order", dir)
			oldRT, _ := newMountedIMRuntime(t, &ImThreadsMessagesList, "--thread", "omt_test", "--sort", dir)
			a := mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), newRT))
			b := mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), oldRT))
			if a != b {
				t.Fatalf("alias parity broken:\n new=%s\n old=%s", a, b)
			}
		})
	}
}

func TestThreadsMessagesList_CanonicalOrderWinsOverLegacySort(t *testing.T) {
	for _, args := range [][]string{
		{"--thread", "omt_test", "--order", "desc", "--sort", "asc"},
		{"--thread", "omt_test", "--sort", "asc", "--order", "desc"},
	} {
		rt, _ := newMountedIMRuntime(t, &ImThreadsMessagesList, args...)
		if got := resolveThreadsOrder(rt); got != "desc" {
			t.Fatalf("canonical --order must win for %v: order=%q", args, got)
		}
	}
}

func TestThreadsMessagesList_OrderFlagSurface(t *testing.T) {
	var orderFlag, sortFlag *common.Flag
	for i := range ImThreadsMessagesList.Flags {
		if ImThreadsMessagesList.Flags[i].Name == "order" {
			orderFlag = &ImThreadsMessagesList.Flags[i]
		}
		if ImThreadsMessagesList.Flags[i].Name == "sort" {
			sortFlag = &ImThreadsMessagesList.Flags[i]
		}
	}
	if orderFlag == nil {
		t.Fatal("expected canonical --order declaration")
	}
	if orderFlag.Default != "asc" {
		t.Errorf("--order Default = %q, want asc", orderFlag.Default)
	}
	if got := strings.Join(orderFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--order Enum = %q, want asc,desc", got)
	}
	if len(orderFlag.Aliases) != 0 {
		t.Errorf("--order Aliases = %q, want none", orderFlag.Aliases)
	}
	if sortFlag == nil || !sortFlag.Hidden {
		t.Fatal("historical --sort must remain an independent hidden compatibility flag")
	}
	if got := strings.Join(sortFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--sort Enum = %q, want asc,desc", got)
	}
}
