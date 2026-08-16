// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

// SourceType is the globally defined business-scene enum carried on the wire
// as an integer. Zero is reserved for "unset" and is never a valid scene.
// The scene set mirrors the read-command catalog's source hosts; values 2-8
// follow the downstream consumer's protocol, and the provisional block below
// uses values outside that protocol's known range until the consumer assigns
// authoritative ones. Scenes without a catalog host get no constant here.
type SourceType int

const (
	SourceUnset SourceType = 0

	// Values assigned by the downstream consumer's protocol.
	SourceWiki    SourceType = 2 // wiki nodes and spaces
	SourceDoc     SourceType = 3 // cloud documents
	SourceMessage SourceType = 6 // IM messages and chats
	SourceMinute  SourceType = 8 // minutes recordings

	// Provisional values for catalog hosts the consumer's protocol has not
	// assigned yet. They deliberately sit above the protocol's known range so
	// a future authoritative assignment cannot collide silently; renumber them
	// here once the consumer publishes the real values.
	SourceBitable     SourceType = 13 // bitable (Base) tables and records
	SourceSheet       SourceType = 14 // spreadsheets
	SourceMeeting     SourceType = 15 // meetings
	SourceMeetingNote SourceType = 16 // meeting notes
)

var allocated = map[SourceType]struct{}{
	SourceWiki: {}, SourceDoc: {}, SourceMessage: {}, SourceMinute: {},
	SourceBitable: {}, SourceSheet: {}, SourceMeeting: {}, SourceMeetingNote: {},
}

// IsAllocated reports whether st is an assigned business scene. SourceUnset
// and any value outside the assigned table are not valid on the wire.
func IsAllocated(st SourceType) bool {
	_, ok := allocated[st]
	return ok
}
