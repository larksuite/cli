// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	configpkg "github.com/larksuite/cli/internal/config"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
)

// Filtering only removes rows: the vc selection must be exactly the catalog's
// vc keys, and every remaining row keeps the unfiltered field set.
func TestListDomain_FilterKeepsExactlyTheRequestedDomain(t *testing.T) {
	snap := compileCatalog()

	f, stdout, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test"})
	if err := runList(f, snap, "vc", true); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{}
	for _, key := range snap.Keys() {
		if strings.HasPrefix(key, "vc.") {
			want[key] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("the catalog has no vc keys; the filter test proves nothing")
	}
	got := map[string]bool{}
	for _, row := range rows {
		var key string
		_ = json.Unmarshal(row["key"], &key)
		got[key] = true
		for _, field := range []string{"event_type", "schema", "resolved_output_schema"} {
			if _, ok := row[field]; !ok {
				t.Errorf("%s: filtering must not reshape rows; %q is missing", key, field)
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("filtered rows = %v, want the exact vc set %v", got, want)
	}
	for key := range want {
		if !got[key] {
			t.Errorf("vc key missing from the filtered list: %s", key)
		}
	}
}

func TestListDomain_TextFilter(t *testing.T) {
	snap := compileCatalog()
	f, stdout, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test"})
	if err := runList(f, snap, "im", false); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "im.message.receive_v1") {
		t.Error("im keys must be listed")
	}
	for _, foreign := range []string{"vc.", "minutes.", "board.", "approval."} {
		if strings.Contains(out, foreign) {
			t.Errorf("foreign domain %q leaked into the filtered text output", foreign)
		}
	}
}

func TestListDomain_UnknownDomainIsRejectedWithTheValidSet(t *testing.T) {
	snap := compileCatalog()
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test"})
	err := runList(f, snap, "definitely-bogus", true)
	if err == nil {
		t.Fatal("an unknown domain must be rejected")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("want invalid_argument, got %v", err)
	}
	if !strings.Contains(err.Error(), "unknown domain: definitely-bogus") {
		t.Errorf("error must name the rejected value, got %v", err)
	}
	for _, domain := range []string{"application", "approval", "board", "card", "im", "minutes", "task", "vc"} {
		if !strings.Contains(problem.Hint, domain) {
			t.Errorf("hint must list valid domain %q, got %q", domain, problem.Hint)
		}
	}
}
