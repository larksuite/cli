// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "testing"

func TestAutomationCommandsRegistered(t *testing.T) {
	want := map[string]bool{
		"+automation-list": false, "+automation-get": false, "+automation-create": false,
		"+automation-update": false, "+automation-enable": false, "+automation-disable": false,
	}
	for _, sc := range Shortcuts() {
		if _, ok := want[sc.Command]; ok {
			want[sc.Command] = true
		}
	}
	for cmd, found := range want {
		if !found {
			t.Errorf("shortcut %q not registered in Shortcuts()", cmd)
		}
	}
}
