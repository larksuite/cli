// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package note

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all note-domain shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		NoteDetail,
		NoteTranscript,
	}
}
