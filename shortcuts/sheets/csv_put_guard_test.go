// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newCSVGuardRuntime(csvVal string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("csv", "", "")
	cmd.ParseFlags(nil)
	cmd.Flags().Set("csv", csvVal)
	return &common.RuntimeContext{Cmd: cmd}
}

// TestGuardCSVValueIsNotFilePath verifies the guard flags a bare --csv value
// only when it names a real file (a forgotten @), while leaving genuine inline
// content alone — including the case the old name-shape heuristic got wrong:
// prose that merely ends in or mentions a filename.
func TestGuardCSVValueIsNotFilePath(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("data.csv", []byte("a,b\n1,2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Bare value naming an existing file → guarded with a fix-it hint.
	err := guardCSVValueIsNotFilePath(newCSVGuardRuntime("data.csv"))
	ve := requireValidation(t, err, "existing file")
	if !strings.Contains(ve.Message, "@data.csv") {
		t.Errorf("message should suggest @data.csv, got: %q", ve.Message)
	}
	if ve.Param != "--csv" {
		t.Errorf("param = %q, want --csv", ve.Param)
	}

	// Content that is not a real file must pass through unchanged.
	for _, v := range []string{
		"改完记得更新config.json",           // prose ending in a filename — not a real file
		"remember to update data.csv", // mentions the real file but isn't its name
		"a,b\n1,2",                    // multi-cell CSV
		"hello world",
		"nope.csv", // path-shaped but no such file
		"",
	} {
		if err := guardCSVValueIsNotFilePath(newCSVGuardRuntime(v)); err != nil {
			t.Errorf("content %q must pass through, got: %v", v, err)
		}
	}
}
