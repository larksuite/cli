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
		DriveCopy,
		DriveDownload,
		DrivePreview,
		DriveCover,
		DriveAddComment,
		DriveListComments,
		DriveBatchQueryComments,
		DriveResolveComment,
		DriveRestoreComment,
		DriveAddReply,
		DriveListReplies,
		DriveUpdateReply,
		DriveDeleteReply,
		DriveReactReply,
		DriveExport,
		DriveExportDownload,
		DriveImport,
		DriveVersionHistory,
		DriveVersionGet,
		DriveVersionRevert,
		DriveVersionDelete,
		DriveMove,
		DriveUpdateTitle,
		DriveDelete,
		DriveStatus,
		DrivePush,
		DrivePull,
		DriveSync,
		DriveTaskResult,
		DriveApplyPermission,
		DriveMemberAdd,
		DriveMemberList,
		DriveMemberRemove,
		DrivePermissionGetSetting,
		DriveSecureLabelList,
		DriveSecureLabelUpdate,
		DriveSearch,
		DriveInspect,
	}
}
