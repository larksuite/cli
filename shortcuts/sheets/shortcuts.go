// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all sheets shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		SheetInfo,
		SheetRead,
		SheetWrite,
		SheetWriteImage,
		SheetAppend,
		SheetFind,
		SheetCreate,
		SheetExport,
	}
}
