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
