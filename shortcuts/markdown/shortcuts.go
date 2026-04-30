// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all markdown shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		MarkdownCreate,
		MarkdownFetch,
		MarkdownOverwrite,
	}
}
