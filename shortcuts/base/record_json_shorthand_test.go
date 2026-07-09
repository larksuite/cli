// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func mountBaseShortcutFlags(t *testing.T, s common.Shortcut, name string) *cobra.Command {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, &cmdutil.Factory{})
	cmd, _, err := parent.Find([]string{name})
	if err != nil {
		t.Fatalf("Find(%s) error = %v", name, err)
	}
	return cmd
}

// record-list 获得 --json 简写
func TestRecordListRegistersJSONShorthand(t *testing.T) {
	cmd := mountBaseShortcutFlags(t, BaseRecordList, "+record-list")
	fl := cmd.Flags().Lookup("json")
	if fl == nil {
		t.Fatal("+record-list missing --json shorthand")
	}
	if fl.Usage != "shorthand for --format json" {
		t.Errorf("usage = %q, want shorthand", fl.Usage)
	}
	if def := cmd.Flags().Lookup("format").DefValue; def != "markdown" {
		t.Errorf("format default = %q, want markdown (unchanged)", def)
	}
}

// record-search / record-get 的 --json 保持请求体语义，不被覆盖（回归锚点）
func TestRecordSearchGetKeepRequestBodyJSON(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		cmdName  string
	}{
		{"record-search", BaseRecordSearch, "+record-search"},
		{"record-get", BaseRecordGet, "+record-get"},
	} {
		cmd := mountBaseShortcutFlags(t, tc.shortcut, tc.cmdName)
		fl := cmd.Flags().Lookup("json")
		if fl == nil {
			t.Fatalf("%s: --json (request body) missing", tc.name)
		}
		if strings.Contains(fl.Usage, "shorthand") {
			t.Fatalf("%s: request-body --json overwritten by shorthand: %q", tc.name, fl.Usage)
		}
		if fl.Value.Type() != "string" {
			t.Fatalf("%s: --json type = %q, want string", tc.name, fl.Value.Type())
		}
	}
}

// Enum 已接入：help 描述携带枚举后缀（框架对带 Enum 的 flag 自动追加 " (markdown|json)"）
func TestRecordReadFormatFlagCarriesEnum(t *testing.T) {
	cmd := mountBaseShortcutFlags(t, BaseRecordList, "+record-list")
	usage := cmd.Flags().Lookup("format").Usage
	if !strings.Contains(usage, "(markdown|json)") {
		t.Fatalf("format usage missing enum suffix: %q", usage)
	}
}
