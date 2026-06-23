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
		stringFlags["thread"] = "omt_test"
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

// TestThreadsMessagesList_OrderAliasParity proves DryRun(--sort dir) == DryRun(--order dir).
// This is the test the refactor exists to make meaningful (single shared mapping).
func TestThreadsMessagesList_OrderAliasParity(t *testing.T) {
	for _, dir := range []string{"asc", "desc"} {
		t.Run(dir, func(t *testing.T) {
			newRT := newThreadsTestRT(t, map[string]string{"order": dir})
			oldRT := newThreadsTestRT(t, map[string]string{"sort": dir})
			a := mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), newRT))
			b := mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), oldRT))
			if a != b {
				t.Fatalf("alias parity broken:\n new=%s\n old=%s", a, b)
			}
		})
	}
}

func TestThreadsMessagesList_OrderFlagSurface(t *testing.T) {
	var orderFlag, aliasFlag *common.Flag
	for i := range ImThreadsMessagesList.Flags {
		switch ImThreadsMessagesList.Flags[i].Name {
		case "order":
			orderFlag = &ImThreadsMessagesList.Flags[i]
		case "sort":
			aliasFlag = &ImThreadsMessagesList.Flags[i]
		}
	}
	if orderFlag == nil || aliasFlag == nil {
		t.Fatalf("expected both --order and --sort flags declared")
	}
	if orderFlag.Default != "asc" {
		t.Errorf("--order Default = %q, want asc", orderFlag.Default)
	}
	if got := strings.Join(orderFlag.Enum, ","); got != "asc,desc" {
		t.Errorf("--order Enum = %q, want asc,desc", got)
	}
	if !aliasFlag.Hidden {
		t.Errorf("--sort must be Hidden")
	}
	if aliasFlag.Default != "" {
		t.Errorf("--sort (hidden alias) must not carry a Default, got %q", aliasFlag.Default)
	}
}
