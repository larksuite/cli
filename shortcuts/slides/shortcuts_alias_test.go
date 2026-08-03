// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestWithFlagAliases(t *testing.T) {
	cases := []struct {
		canonical string
		aliases   []string
	}{
		{canonical: "presentation", aliases: presentationFlagAliases},
		{canonical: "content", aliases: contentFlagAliases},
	}
	for _, tc := range cases {
		for _, alias := range tc.aliases {
			t.Run(tc.canonical+"/"+alias, func(t *testing.T) {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String(tc.canonical, "", tc.canonical+" value")
				withFlagAliases(wholePageAliasMap, nil)(cmd)

				if err := cmd.Flags().Parse([]string{"--" + alias, "valABC"}); err != nil {
					t.Fatalf("--%s should resolve to --%s: %v", alias, tc.canonical, err)
				}
				got, err := cmd.Flags().GetString(tc.canonical)
				if err != nil {
					t.Fatalf("read --%s: %v", tc.canonical, err)
				}
				if got != "valABC" {
					t.Fatalf("--%s set --%s to %q, want valABC", alias, tc.canonical, got)
				}
				if usage := cmd.Flags().FlagUsages(); strings.Contains(usage, "--"+alias) {
					t.Fatalf("hidden compatibility alias --%s leaked into help:\n%s", alias, usage)
				}
			})
		}
	}
}

// TestFlagAliasesOnlyResolveDeclaredFlags pins the guard that keeps a mistyped
// alias reported as the flag the caller actually typed: when the command does
// not declare the canonical target, the alias must be left alone.
func TestFlagAliasesOnlyResolveDeclaredFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("presentation", "", "presentation reference")
	withFlagAliases(wholePageAliasMap, nil)(cmd)

	err := cmd.Flags().Parse([]string{"--xml", "<slide/>"})
	if err == nil {
		t.Fatal("--xml resolved on a command without --content, want unknown-flag error")
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Fatalf("error should name the flag the user typed, got: %v", err)
	}
}

// TestContentAliasesStayOffOtherShortcuts is the regression guard for a
// package-wide alias table: --content exists on other slides shortcuts, so a
// shared table silently turned `--xml` / `--slide-xml` there into a --content
// value the caller never meant to pass. Only the whole-page commands may
// resolve them.
func TestContentAliasesStayOffOtherShortcuts(t *testing.T) {
	wholePage := map[string]bool{"+update-slide": true, "+update": true}
	for _, shortcut := range Shortcuts() {
		if wholePage[shortcut.Command] || !declaresFlag(shortcut.Flags, "content") {
			continue
		}
		if shortcut.PostMount == nil {
			continue
		}
		cmd := &cobra.Command{Use: shortcut.Command}
		cmd.Flags().String("content", "", "content")
		cmd.Flags().String("presentation", "", "presentation reference")
		shortcut.PostMount(cmd)

		for _, alias := range contentFlagAliases {
			if err := cmd.Flags().Parse([]string{"--" + alias, "x"}); err == nil {
				t.Errorf("%s resolved --%s to --content; content aliases must be scoped to the whole-page commands", shortcut.Command, alias)
			}
		}
	}
}

// TestFlagAliasesAreNotCanonicalNames guards the termination argument in
// withFlagAliases: fs.Lookup re-enters the normalizer with the canonical name,
// which must not itself be an alias key.
func TestFlagAliasesAreNotCanonicalNames(t *testing.T) {
	for _, aliases := range []map[string]string{presentationAliasMap, wholePageAliasMap} {
		canonical := map[string]bool{}
		for _, name := range aliases {
			canonical[name] = true
		}
		for alias, target := range aliases {
			if canonical[alias] {
				t.Errorf("alias %q is also a canonical flag name; normalization would recurse", alias)
			}
			if alias == target {
				t.Errorf("alias %q maps to itself", alias)
			}
		}
	}
}

// TestFlagAliasesDoNotShadowRealFlags catches the dangerous direction of
// pflag.SetNormalizeFunc: it re-normalizes flags that are already registered,
// so if a shortcut ever declares a flag whose name is an alias key, that flag
// collapses into the canonical one — no panic, no error, just a missing flag.
func TestFlagAliasesDoNotShadowRealFlags(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.PostMount == nil {
			continue
		}
		for _, flag := range shortcut.Flags {
			if canonical, ok := wholePageAliasMap[flag.Name]; ok {
				t.Errorf("%s declares --%s, which is an alias of --%s; the normalizer would erase it", shortcut.Command, flag.Name, canonical)
			}
		}
	}
}

func TestShortcutsAttachFlagAliases(t *testing.T) {
	count := 0
	for _, shortcut := range Shortcuts() {
		if !declaresFlag(shortcut.Flags, "presentation") {
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

func declaresFlag(flags []common.Flag, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}
