// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all sheets shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		// Spreadsheet management
		SheetCreate,
		SheetInfo,
		SheetExport,

		// Sheet management
		SheetCreateSheet,
		SheetCopySheet,
		SheetDeleteSheet,
		SheetUpdateSheet,

		// Cell data
		SheetRead,
		SheetWrite,
		SheetAppend,
		SheetFind,
		SheetReplace,

		// Cell style and merge
		SheetSetStyle,
		SheetBatchSetStyle,
		SheetMergeCells,
		SheetUnmergeCells,

		// Cell images
		SheetWriteImage,

		// Row/column management
		SheetAddDimension,
		SheetInsertDimension,
		SheetUpdateDimension,
		SheetMoveDimension,
		SheetDeleteDimension,

		// Filter views
		SheetCreateFilterView,
		SheetUpdateFilterView,
		SheetListFilterViews,
		SheetGetFilterView,
		SheetDeleteFilterView,
		SheetCreateFilterViewCondition,
		SheetUpdateFilterViewCondition,
		SheetListFilterViewConditions,
		SheetGetFilterViewCondition,
		SheetDeleteFilterViewCondition,

		// Dropdown
		SheetSetDropdown,
		SheetUpdateDropdown,
		SheetGetDropdown,
		SheetDeleteDropdown,

		// Float images
		SheetMediaUpload,
		SheetCreateFloatImage,
		SheetUpdateFloatImage,
		SheetGetFloatImage,
		SheetListFloatImages,
		SheetDeleteFloatImage,
	}
}
