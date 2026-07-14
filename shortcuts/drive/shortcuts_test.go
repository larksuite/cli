// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"reflect"
	"testing"
)

// TestShortcutsIncludesExpectedCommands verifies the drive shortcut registry contains the expected commands.
func TestShortcutsIncludesExpectedCommands(t *testing.T) {
	t.Parallel()

	got := Shortcuts()
	want := []string{
		"+upload",
		"+create-folder",
		"+create-shortcut",
		"+download",
		"+preview",
		"+cover",
		"+add-comment",
		"+list-comments",
		"+export",
		"+export-download",
		"+import",
		"+version-history",
		"+version-get",
		"+version-revert",
		"+version-delete",
		"+move",
		"+delete",
		"+status",
		"+push",
		"+pull",
		"+sync",
		"+task_result",
		"+apply-permission",
		"+member-add",
		"+secure-label-list",
		"+secure-label-update",
		"+search",
		"+inspect",
	}

	if len(got) != len(want) {
		t.Fatalf("len(Shortcuts()) = %d, want %d", len(got), len(want))
	}

	seen := make(map[string]bool, len(got))
	for _, shortcut := range got {
		if seen[shortcut.Command] {
			t.Fatalf("duplicate shortcut command: %s", shortcut.Command)
		}
		seen[shortcut.Command] = true
	}

	for _, command := range want {
		if !seen[command] {
			t.Fatalf("missing shortcut command %q in Shortcuts()", command)
		}
	}
}

func TestDriveSearchSupportsUserAndBotIdentity(t *testing.T) {
	t.Parallel()

	want := []string{"user", "bot"}
	if !reflect.DeepEqual(DriveSearch.AuthTypes, want) {
		t.Fatalf("DriveSearch.AuthTypes = %v, want %v", DriveSearch.AuthTypes, want)
	}
}
