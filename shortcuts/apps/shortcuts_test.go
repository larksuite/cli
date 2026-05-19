// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "testing"

// 钉死域内 shortcut 数量。少一条（漏挂）或多一条（误加）都会被这个测试拦截。
func TestAppsShortcuts_Returns5(t *testing.T) {
	got := Shortcuts()
	if len(got) != 5 {
		t.Fatalf("Shortcuts() returned %d entries, want 5", len(got))
	}
}
