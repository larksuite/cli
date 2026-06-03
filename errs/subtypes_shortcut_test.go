// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import "testing"

func TestShortcutSubtypes_Values(t *testing.T) {
	tests := []struct {
		name string
		got  Subtype
		want string
	}{
		{"OneOfMissing", SubtypeShortcutOneOfMissing, "shortcut_oneof_missing"},
		{"OneOfMultiple", SubtypeShortcutOneOfMultiple, "shortcut_oneof_multiple"},
		{"GroupIncomplete", SubtypeShortcutGroupIncomplete, "shortcut_group_incomplete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Errorf("got %q, want %q", string(tt.got), tt.want)
			}
		})
	}
}
