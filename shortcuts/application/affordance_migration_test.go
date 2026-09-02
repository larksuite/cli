// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import "testing"

func TestSlashCommandGuidanceLivesInAffordance(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if len(shortcut.Tips) != 0 {
			t.Errorf("%s still has Go Tips; keep application usage guidance in affordance/application.md", shortcut.Command)
		}
	}
}
