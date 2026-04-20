// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all calendar shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		CalendarAgenda,
		CalendarCreate,
		CalendarFreebusy,
		CalendarRoomFind,
		CalendarRsvp,
		CalendarSuggestion,
	}
}
