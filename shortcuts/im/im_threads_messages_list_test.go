// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

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

func TestThreadsMessagesList_SortAliasParity(t *testing.T) {
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

func TestThreadsMessagesList_SortAliasesUseLastOccurrence(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--thread", "omt_test", "--order", "desc", "--sort", "asc"}, want: "asc"},
		{args: []string{"--thread", "omt_test", "--sort", "asc", "--order", "desc"}, want: "desc"},
	}
	for _, test := range tests {
		rt, _ := newMountedIMRuntime(t, &ImThreadsMessagesList, test.args...)
		if got := rt.Str("order"); got != test.want {
			t.Fatalf("order for %v = %q, want %q", test.args, got, test.want)
		}
	}
}

func TestThreadsMessagesList_OrderFlagSurface(t *testing.T) {
	var orderFlag *common.Flag
	for i := range ImThreadsMessagesList.Flags {
		if ImThreadsMessagesList.Flags[i].Name == "order" {
			orderFlag = &ImThreadsMessagesList.Flags[i]
		}
		if ImThreadsMessagesList.Flags[i].Name == "sort" {
			t.Fatal("--sort must not be declared independently")
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
	if got := strings.Join(orderFlag.Aliases, ","); got != "sort" {
		t.Errorf("--order Aliases = %q, want sort", got)
	}
}
