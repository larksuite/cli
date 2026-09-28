// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

// SourceType is the globally defined business-scene enum carried on the wire
// as an integer. Zero is reserved for "unset" and is never a valid scene.
// The scene set mirrors the read-command catalog's source hosts; values
// follow the downstream consumer's assignment. Scenes without a catalog host
// get no constant here.
type SourceType int

const (
	SourceUnset SourceType = 0

	SourceWiki        SourceType = 1  // wiki nodes and spaces
	SourceDoc         SourceType = 2  // cloud documents
	SourceMessage     SourceType = 3  // IM messages and chats
	SourceMinute      SourceType = 4  // minutes recordings
	SourceBase        SourceType = 5  // bitable (Base) tables and records
	SourceSheet       SourceType = 6  // spreadsheets
	SourceMeeting     SourceType = 7  // meetings
	SourceMeetingNote SourceType = 8  // meeting notes
	SourceMindnote    SourceType = 9  // mindnotes
	SourceSlides      SourceType = 10 // slides presentations
	SourceFile        SourceType = 11 // drive files
)

var allocated = map[SourceType]struct{}{
	SourceWiki: {}, SourceDoc: {}, SourceMessage: {}, SourceMinute: {},
	SourceBase: {}, SourceSheet: {}, SourceMeeting: {}, SourceMeetingNote: {},
	SourceMindnote: {}, SourceSlides: {}, SourceFile: {},
}

// IsAllocated reports whether st is an assigned business scene. SourceUnset
// and any value outside the assigned table are not valid on the wire.
func IsAllocated(st SourceType) bool {
	_, ok := allocated[st]
	return ok
}
