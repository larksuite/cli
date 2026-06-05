// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all vc shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		VCSearch,
		VCNotes,
		VCRecording,
		VCMeetingJoin,
		VCMeetingLeave,
		VCMeetingEvents,
	}
}
