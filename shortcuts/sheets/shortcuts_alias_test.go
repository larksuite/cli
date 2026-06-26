// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestWithTokenAlias verifies the PostMount-based --token → --spreadsheet-token
// alias: it resolves at parse time, and it composes onto (rather than replaces)
// any pre-existing PostMount — the property that lets it coexist with
// +csv-put's --range/--start-cell flag-group setup.
func TestWithTokenAlias(t *testing.T) {
	t.Parallel()

	// Alias resolves to the canonical flag.
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("spreadsheet-token", "", "")
	withTokenAlias(nil)(cmd)
	if err := cmd.Flags().Parse([]string{"--token", "shtABC"}); err != nil {
		t.Fatalf("--token should resolve as an alias: %v", err)
	}
	if got := cmd.Flags().Lookup("spreadsheet-token").Value.String(); got != "shtABC" {
		t.Errorf("--token should set --spreadsheet-token; got %q", got)
	}

	// Composes with an existing PostMount instead of dropping it.
	prevRan := false
	cmd2 := &cobra.Command{Use: "y"}
	cmd2.Flags().String("spreadsheet-token", "", "")
	withTokenAlias(func(_ *cobra.Command) { prevRan = true })(cmd2)
	if !prevRan {
		t.Error("pre-existing PostMount should still run")
	}
	if err := cmd2.Flags().Parse([]string{"--token", "shtZ"}); err != nil {
		t.Fatalf("--token should still resolve when composed: %v", err)
	}
}

// TestShortcuts_TokenAliasOnSpreadsheetTokenCommands asserts every shortcut that
// takes --spreadsheet-token ends up with a PostMount (the composed token alias),
// so the reflex typo is forgiven wherever the canonical flag exists.
func TestShortcuts_TokenAliasOnSpreadsheetTokenCommands(t *testing.T) {
	t.Parallel()
	for _, s := range Shortcuts() {
		if hasFlag(s.Flags, "spreadsheet-token") && s.PostMount == nil {
			t.Errorf("%s takes --spreadsheet-token but has no PostMount (token alias missing)", s.Command)
		}
	}
}
