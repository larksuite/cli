// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"slices"
	"testing"
)

func TestShortcutsDeclareSpreadsheetTokenAlias(t *testing.T) {
	t.Parallel()

	count := 0
	for _, shortcut := range Shortcuts() {
		for _, flag := range shortcut.Flags {
			if flag.Name != "spreadsheet-token" {
				continue
			}
			count++
			if !slices.Contains(flag.Aliases, "token") {
				t.Errorf("%s --spreadsheet-token aliases = %v, want token", shortcut.Command, flag.Aliases)
			}
		}
	}
	if count == 0 {
		t.Fatal("expected at least one sheets shortcut with --spreadsheet-token")
	}
}

func TestShortcutsDoNotAccumulateSpreadsheetTokenAliases(t *testing.T) {
	t.Parallel()

	for call := 0; call < 2; call++ {
		for _, shortcut := range Shortcuts() {
			for _, flag := range shortcut.Flags {
				if flag.Name != "spreadsheet-token" {
					continue
				}
				if got := countString(flag.Aliases, "token"); got != 1 {
					t.Fatalf("call %d: %s has %d token aliases: %v", call+1, shortcut.Command, got, flag.Aliases)
				}
			}
		}
	}
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
