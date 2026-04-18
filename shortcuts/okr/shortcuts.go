// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"github.com/larksuite/cli/shortcuts/common"
)

// Shortcuts returns all okr shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		OKRListCycles,
		OKRCycleDetail,
	}
}
