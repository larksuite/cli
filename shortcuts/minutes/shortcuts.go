// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all minutes shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		MinutesSearch,
		MinutesDownload,
		MinutesUpload,
	}
}
