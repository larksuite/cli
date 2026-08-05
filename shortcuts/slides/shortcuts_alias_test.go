// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"slices"
	"testing"
)

func TestShortcutsDeclarePresentationFlagAliases(t *testing.T) {
	wantRequired := map[string]bool{
		"+add-slide":             true,
		"+delete-slide":          true,
		"+media-upload":          true,
		"+replace-slide":         true,
		"+replace-pages":         true,
		"+update-slide":          true,
		"+update":                true,
		"+screenshot":            false,
		"+xml-get":               true,
		"+history-list":          true,
		"+history-revert":        true,
		"+history-revert-status": true,
	}
	seen := make(map[string]bool, len(wantRequired))
	for _, shortcut := range Shortcuts() {
		for _, flag := range shortcut.Flags {
			if flag.Name != "presentation" {
				continue
			}
			required, ok := wantRequired[shortcut.Command]
			if !ok {
				t.Errorf("unexpected presentation flag on %s", shortcut.Command)
				continue
			}
			seen[shortcut.Command] = true
			if !slices.Equal(flag.Aliases, presentationFlagAliases) {
				t.Errorf("%s --presentation aliases = %v, want %v", shortcut.Command, flag.Aliases, presentationFlagAliases)
			}
			if flag.Required != required {
				t.Errorf("%s --presentation required = %v, want %v", shortcut.Command, flag.Required, required)
			}
		}
	}
	for command := range wantRequired {
		if !seen[command] {
			t.Errorf("%s is missing the shared --presentation flag", command)
		}
	}
}

func TestPresentationRefFlagReturnsIndependentAliases(t *testing.T) {
	first := requiredPresentationRefFlag()
	second := listModePresentationRefFlag()
	first.Aliases[0] = "mutated"

	if !slices.Equal(second.Aliases, presentationFlagAliases) {
		t.Fatalf("second aliases = %v, want independent %v", second.Aliases, presentationFlagAliases)
	}
	if first.Desc != presentationRefDescription || !first.Required {
		t.Fatalf("required flag = %#v", first)
	}
	if want := presentationRefDescription + "; list mode only"; second.Desc != want || second.Required {
		t.Fatalf("optional flag = %#v, want description %q", second, want)
	}
}

// TestContentFlagAliasesAreScopedToWholePageCommands guards the blast radius of
// the --content spellings: only the commands that take a whole page may carry
// them. +screenshot has a --content of its own, and resolving --xml there would
// rewrite a mistyped flag into one the caller never meant to use.
func TestContentFlagAliasesAreScopedToWholePageCommands(t *testing.T) {
	wholePage := map[string]bool{
		SlidesUpdateSlide.Command: true,
		SlidesUpdate.Command:      true,
	}
	sawWholePage, sawOther := false, false
	for _, shortcut := range Shortcuts() {
		for _, flag := range shortcut.Flags {
			if flag.Name != "content" {
				continue
			}
			if wholePage[shortcut.Command] {
				sawWholePage = true
				if !slices.Equal(flag.Aliases, contentFlagAliases) {
					t.Errorf("%s --content aliases = %v, want %v", shortcut.Command, flag.Aliases, contentFlagAliases)
				}
				continue
			}
			sawOther = true
			if len(flag.Aliases) != 0 {
				t.Errorf("%s --content must carry no aliases, got %v", shortcut.Command, flag.Aliases)
			}
		}
	}
	if !sawWholePage {
		t.Fatal("expected at least one whole-page command to declare --content")
	}
	// Without a negative case the assertion above proves nothing about scoping.
	if !sawOther {
		t.Fatal("expected at least one other command with its own --content")
	}
}

// TestSlideFlagIsNotAnAlias pins a deliberate omission: several slides commands
// take --slide-id, so `--slide <id>` is a likely typo for that. Resolving it to
// --content would turn the typo into a request carrying an id where page XML
// belongs.
func TestSlideFlagIsNotAnAlias(t *testing.T) {
	if slices.Contains(contentFlagAliases, "slide") {
		t.Fatal("--slide must not be a --content alias")
	}
	if slices.Contains(presentationFlagAliases, "slide") {
		t.Fatal("--slide must not be a --presentation alias")
	}
}
