// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newAliasTestRT registers a new flag (with a default) and an old flag, then
// sets only the flags present in `set` — so Changed() reflects exactly which
// flags were "passed on the command line".
func newAliasTestRT(t *testing.T, newName, newDefault, oldName string, set map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(newName, newDefault, "")
	cmd.Flags().String(oldName, "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for k, v := range set {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("Set(%q) error = %v", k, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestAliasFlagValue(t *testing.T) {
	cases := []struct {
		name    string
		set     map[string]string
		wantVal string
		wantOK  bool
	}{
		{"only old set", map[string]string{"sort-type": "ByActiveTimeDesc"}, "ByActiveTimeDesc", true},
		{"neither set", nil, "", false},
		{"only new set", map[string]string{"sort": "active_time"}, "", false},
		{"both set new wins", map[string]string{"sort": "active_time", "sort-type": "ByCreateTimeAsc"}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newAliasTestRT(t, "sort", "create_time", "sort-type", c.set)
			gotVal, gotOK := aliasFlagValue(rt, "sort-type", "sort")
			if gotVal != c.wantVal || gotOK != c.wantOK {
				t.Fatalf("aliasFlagValue() = (%q, %v), want (%q, %v)", gotVal, gotOK, c.wantVal, c.wantOK)
			}
		})
	}
}
