// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "github.com/larksuite/cli/internal/commandbridge"

// ShortcutSchema returns the immutable schema contract of a hosted command.
// The internal token keeps schema inspection out of common's public surface.
func ShortcutSchema(shortcut Shortcut, _ commandbridge.Access) (any, bool) {
	if shortcut.typed == nil {
		return nil, false
	}
	return shortcut.typed.contract, true
}
