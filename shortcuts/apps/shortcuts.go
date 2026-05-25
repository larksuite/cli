// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all apps domain shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		AppsCreate,
		AppsUpdate,
		AppsList,
		AppsAccessScopeSet,
		AppsAccessScopeGet,
		AppsHTMLPublish,
	}
}
