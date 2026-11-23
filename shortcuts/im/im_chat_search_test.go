// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func newSearchTestRT(t *testing.T, stringFlags map[string]string) *common.RuntimeContext {
	t.Helper()
	if stringFlags == nil {
		stringFlags = map[string]string{}
	}
	if _, ok := stringFlags["query"]; !ok {
		stringFlags["query"] = "team"
	}
	rt := newChatSearchTestRuntimeContext(t, stringFlags, nil)
	rt.Factory = &cmdutil.Factory{
		IOStreams: &cmdutil.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		},
	}
	return rt
}

func TestChatSearch_SortMapping(t *testing.T) {
	cases := []struct{ sort, want string }{
		{"create_time", "create_time_desc"},
		{"update_time", "update_time_desc"},
		{"member_count", "member_count_desc"},
	}
	for _, c := range cases {
		t.Run(c.sort, func(t *testing.T) {
			rt := newSearchTestRT(t, map[string]string{"sort": c.sort})
			body := buildSearchChatBody(rt)
			if body["sorter"] != c.want {
				t.Fatalf("sort=%s -> sorter=%v, want %s", c.sort, body["sorter"], c.want)
			}
		})
	}
}

// TestChatSearch_SortOmittedWhenUnset: no --sort and no --sort-by -> sorter omitted.
func TestChatSearch_SortOmittedWhenUnset(t *testing.T) {
	rt := newSearchTestRT(t, nil)
	body := buildSearchChatBody(rt)
	if _, present := body["sorter"]; present {
		t.Fatalf("sorter should be omitted when neither --sort nor --sort-by set")
	}
}

// TestChatSearch_SortCompatibilityParity proves Normalize translates the
// legacy upstream sorter vocabulary before the canonical-only builder runs.
func TestChatSearch_SortCompatibilityParity(t *testing.T) {
	pairs := []struct{ newVal, oldVal string }{
		{"create_time", "create_time_desc"},
		{"update_time", "update_time_desc"},
		{"member_count", "member_count_desc"},
	}
	for _, p := range pairs {
		t.Run(p.newVal, func(t *testing.T) {
			newBody := buildSearchChatBody(newSearchTestRT(t, map[string]string{"sort": p.newVal}))
			oldRT := newSearchTestRT(t, map[string]string{"sort-by": p.oldVal})
			if err := normalizeChatSearchSortCompatibility(context.Background(), oldRT.FlagContext()); err != nil {
				t.Fatal(err)
			}
			oldBody := buildSearchChatBody(oldRT)
			if newBody["sorter"] != oldBody["sorter"] {
				t.Fatalf("alias parity: new sorter=%v, old sorter=%v", newBody["sorter"], oldBody["sorter"])
			}
		})
	}
}

func TestChatSearch_SortNewWins(t *testing.T) {
	rt := newSearchTestRT(t, map[string]string{"sort": "member_count", "sort-by": "create_time_desc"})
	if err := normalizeChatSearchSortCompatibility(context.Background(), rt.FlagContext()); err != nil {
		t.Fatal(err)
	}
	body := buildSearchChatBody(rt)
	if body["sorter"] != "member_count_desc" {
		t.Fatalf("new should win: sorter=%v, want member_count_desc", body["sorter"])
	}
}

func TestChatSearch_SortFlagSurface(t *testing.T) {
	var sortFlag, legacyFlag *common.Flag
	for i := range ImChatSearch.Flags {
		switch ImChatSearch.Flags[i].Name {
		case "sort":
			sortFlag = &ImChatSearch.Flags[i]
		case "sort-by":
			legacyFlag = &ImChatSearch.Flags[i]
		}
	}
	if sortFlag == nil || legacyFlag == nil {
		t.Fatalf("expected both --sort and --sort-by flags declared")
	}
	if sortFlag.Default != "" {
		t.Errorf("--sort must have no default (sorter omitted when unset), got %q", sortFlag.Default)
	}
	if got := strings.Join(sortFlag.Enum, ","); got != "create_time,update_time,member_count" {
		t.Errorf("--sort Enum = %q", got)
	}
	if !legacyFlag.Hidden {
		t.Errorf("--sort-by must be Hidden")
	}
	if got := strings.Join(legacyFlag.Enum, ","); got != "create_time_desc,update_time_desc,member_count_desc" {
		t.Errorf("--sort-by Enum = %q, want create_time_desc,update_time_desc,member_count_desc", got)
	}
}

func TestChatSearch_DoesNotDeclareCrossCommandTypesFlag(t *testing.T) {
	for _, flag := range ImChatSearch.Flags {
		if flag.Name == "types" {
			t.Fatal("+chat-search must not expose the +chat-list --types vocabulary")
		}
	}
}
