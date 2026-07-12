// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package attendance

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all attendance shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		AttendanceRecords,
	}
}
