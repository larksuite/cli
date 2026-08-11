// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

// ShortcutSchema returns the immutable schema contract of a Typed Shortcut.
func ShortcutSchema(shortcut Shortcut) (any, bool) {
	if shortcut.typed == nil {
		return nil, false
	}
	return shortcut.typed.contract, true
}
