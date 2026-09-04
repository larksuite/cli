// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"github.com/larksuite/cli/shortcuts/common"
)

// Shortcuts returns all lingo (Feishu dictionary / Baike) shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		LingoEntitySearch,
		LingoEntityMatch,
		LingoEntityGet,
		LingoEntityCreate,
		LingoEntityUpdate,
		LingoEntityDelete,
	}
}
