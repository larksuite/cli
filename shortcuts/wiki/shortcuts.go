// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all wiki shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		WikiMove,
		WikiNodeCreate,
		WikiDeleteSpace,
		WikiSpaceList,
		WikiNodeList,
		WikiNodeCopy,
	}
}
