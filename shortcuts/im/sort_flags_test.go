// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
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
	return &common.RuntimeContext{
		Cmd: cmd,
		Factory: &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}},
	}
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

func TestAliasFlagValueWritesOneNoteToStderr(t *testing.T) {
	rt := newAliasTestRT(t, "start", "", "start-time", map[string]string{
		"start-time": "2026-07-27 00:00:00 +08:00",
	})

	for range 2 {
		if _, ok := aliasFlagValue(rt, "start-time", "start"); !ok {
			t.Fatal("aliasFlagValue() did not select --start-time")
		}
	}

	stderr := rt.IO().ErrOut.(*bytes.Buffer).String()
	if got := strings.Count(stderr, "note: --start-time is an alias for --start\n"); got != 1 {
		t.Fatalf("alias note count = %d, want 1; stderr=%q", got, stderr)
	}
	if stdout := rt.IO().Out.(*bytes.Buffer).String(); stdout != "" {
		t.Fatalf("alias note leaked to stdout: %q", stdout)
	}
}

func TestAliasIntFlagValue(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().Int("limit", 0, "")
	if err := cmd.Flags().Set("limit", "50"); err != nil {
		t.Fatal(err)
	}
	rt := &common.RuntimeContext{
		Cmd: cmd,
		Factory: &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}},
	}

	got, ok := aliasIntFlagValue(rt, "limit", "page-size")
	if !ok || got != 50 {
		t.Fatalf("aliasIntFlagValue() = (%d, %v), want (50, true)", got, ok)
	}
	if stderr := rt.IO().ErrOut.(*bytes.Buffer).String(); stderr != "note: --limit is an alias for --page-size\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}
