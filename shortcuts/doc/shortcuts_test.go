// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import "testing"

func TestShortcutsDoesNotRegisterMediaInsert(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.Command == "+media-insert" {
			t.Fatal("Shortcuts() still registers removed +media-insert command")
		}
	}
}
