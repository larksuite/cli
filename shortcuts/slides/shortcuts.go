// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all slides shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		SlidesCreate,
		SlidesMediaUpload,
	}
}
