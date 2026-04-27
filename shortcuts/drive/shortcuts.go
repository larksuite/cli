// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all drive shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		DriveUpload,
		DriveCreateFolder,
		DriveCreateShortcut,
		DriveDownload,
		DriveAddComment,
		DriveExport,
		DriveExportDownload,
		DriveImport,
		DriveMove,
		DriveDelete,
		DriveTaskResult,
		DriveApplyPermission,
		DriveSearch,
	}
}
