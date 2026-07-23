// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWithPresentationFlagAliases(t *testing.T) {
	for _, alias := range presentationFlagAliases {
		t.Run(alias, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("presentation", "", "presentation reference")
			withPresentationFlagAliases(nil)(cmd)

			if err := cmd.Flags().Parse([]string{"--" + alias, "presABC"}); err != nil {
				t.Fatalf("--%s should resolve to --presentation: %v", alias, err)
			}
			got, err := cmd.Flags().GetString("presentation")
			if err != nil {
				t.Fatalf("read --presentation: %v", err)
			}
			if got != "presABC" {
				t.Fatalf("--%s set --presentation to %q, want presABC", alias, got)
			}
			if usage := cmd.Flags().FlagUsages(); strings.Contains(usage, "--"+alias) {
				t.Fatalf("hidden compatibility alias --%s leaked into help:\n%s", alias, usage)
			}
		})
	}
}

func TestShortcutsAttachPresentationFlagAliases(t *testing.T) {
	count := 0
	for _, shortcut := range Shortcuts() {
		if !hasPresentationFlag(shortcut.Flags) {
			continue
		}
		count++
		if shortcut.PostMount == nil {
			t.Errorf("%s has --presentation but no compatibility normalizer", shortcut.Command)
			continue
		}

		cmd := &cobra.Command{Use: shortcut.Command}
		cmd.Flags().String("presentation", "", "presentation reference")
		shortcut.PostMount(cmd)
		if err := cmd.Flags().Parse([]string{"--token", "presABC"}); err != nil {
			t.Errorf("%s did not normalize --token: %v", shortcut.Command, err)
			continue
		}
		got, err := cmd.Flags().GetString("presentation")
		if err != nil {
			t.Errorf("%s could not read --presentation: %v", shortcut.Command, err)
			continue
		}
		if got != "presABC" {
			t.Errorf("%s normalized --token to %q, want presABC", shortcut.Command, got)
		}
	}
	if count == 0 {
		t.Fatal("expected at least one slides shortcut with --presentation")
	}
}
