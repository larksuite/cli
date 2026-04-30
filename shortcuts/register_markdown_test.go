// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestRegisterShortcutsMountsMarkdownCommands(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, &cmdutil.Factory{})

	for _, path := range [][]string{
		{"markdown", "+create"},
		{"markdown", "+fetch"},
		{"markdown", "+overwrite"},
	} {
		cmd, _, err := program.Find(path)
		if err != nil {
			t.Fatalf("find markdown shortcut %v: %v", path, err)
		}
		if cmd == nil || cmd.Name() != path[1] {
			t.Fatalf("markdown shortcut not mounted: %#v", cmd)
		}
	}
}
