// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import "testing"

// TestShortcutsIncludesExpectedCommands verifies the drive shortcut registry contains the expected commands.
func TestShortcutsIncludesExpectedCommands(t *testing.T) {
	t.Parallel()

	got := Shortcuts()
	want := []string{
		"+upload",
		"+create-folder",
		"+create-shortcut",
		"+download",
		"+add-comment",
		"+export",
		"+export-download",
		"+import",
		"+move",
		"+delete",
		"+status",
		"+task_result",
		"+apply-permission",
		"+search",
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
