// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import "github.com/larksuite/cli/shortcuts/common"

const (
	docsCreateContentFlagBase = "document body; XML by default or Markdown when --doc-format markdown."
	docsUpdateContentFlagBase = "replacement or inserted content; XML by default or Markdown when --doc-format markdown; empty with str_replace deletes match."
)

// Shortcuts returns all docs shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		DocsSearch,
		DocsCreate,
		DocsFetch,
		DocsUpdate,
		DocsScript,
		DocsHistoryList,
		DocsHistoryRevert,
		DocsHistoryRevertStatus,
		DocMediaInsert,
		DocMediaUpload,
		DocMediaPreview,
		DocMediaDownload,
		DocResourceDownload,
		DocResourceUpdate,
		DocResourceDelete,
	}
}
