// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all docs shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		DocsSearch,
		DocsCreate,
		DocsFetch,
		DocsUpdate,
		DocMediaInsert,
		DocMediaPreview,
		DocMediaDownload,
	}
}
