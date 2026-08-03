// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newCompatibilityTestRT registers canonical and legacy flags with different
// value vocabularies, then marks only the supplied flags changed.
func newCompatibilityTestRT(t *testing.T, newName, newDefault, oldName string, set map[string]string) *common.RuntimeContext {
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

func TestNormalizeSortCompatibilityFlag(t *testing.T) {
	cases := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"only old set", map[string]string{"sort-type": "ByActiveTimeDesc"}, "active_time"},
		{"explicit empty old value stays accepted", map[string]string{"sort-type": ""}, ""},
		{"neither set", nil, "create_time"},
		{"only new set", map[string]string{"sort": "active_time"}, "active_time"},
		{"both set new wins", map[string]string{"sort": "active_time", "sort-type": "ByCreateTimeAsc"}, "active_time"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newCompatibilityTestRT(t, "sort", "create_time", "sort-type", c.set)
			if err := normalizeChatListSortCompatibility(context.Background(), rt.FlagContext()); err != nil {
				t.Fatalf("normalizeChatListSortCompatibility() error = %v", err)
			}
			if got := rt.Str("sort"); got != c.want {
				t.Fatalf("canonical sort = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeSortCompatibilityFlagIsSilent(t *testing.T) {
	rt := newCompatibilityTestRT(t, "sort", "create_time", "sort-type", map[string]string{
		"sort-type": "ByActiveTimeDesc",
	})

	for range 2 {
		if err := normalizeChatListSortCompatibility(context.Background(), rt.FlagContext()); err != nil {
			t.Fatal(err)
		}
	}

	if stderr := rt.IO().ErrOut.(*bytes.Buffer).String(); stderr != "" {
		t.Fatalf("compatibility normalization emitted stderr noise: %q", stderr)
	}
	if stdout := rt.IO().Out.(*bytes.Buffer).String(); stdout != "" {
		t.Fatalf("alias note leaked to stdout: %q", stdout)
	}
}

func TestNormalizeSortCompatibilityFlagValidatesLegacyVocabulary(t *testing.T) {
	rt := newCompatibilityTestRT(t, "sort", "create_time", "sort-type", map[string]string{
		"sort-type": "unexpected",
	})
	err := normalizeChatListSortCompatibility(context.Background(), rt.FlagContext())
	assertIMValidationError(t, err, "--sort-type", `invalid value "unexpected" for --sort-type`)

	rt = newCompatibilityTestRT(t, "sort", "create_time", "sort-type", map[string]string{
		"sort":      "active_time",
		"sort-type": "unexpected",
	})
	err = normalizeChatListSortCompatibility(context.Background(), rt.FlagContext())
	assertIMValidationError(t, err, "--sort-type", `invalid value "unexpected" for --sort-type`)
}
