// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all feed shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		FeedCreate,
	}
}
