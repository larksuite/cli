// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package citation exposes source metadata for command extensions.
// The host owns validation, feature gates and wire encoding.
package citation

import internalcitation "github.com/larksuite/cli/internal/citation"

type Citation = internalcitation.Citation
type SourceType = internalcitation.SourceType

const (
	SourceUnset       = internalcitation.SourceUnset
	SourceWiki        = internalcitation.SourceWiki
	SourceDoc         = internalcitation.SourceDoc
	SourceMessage     = internalcitation.SourceMessage
	SourceMinute      = internalcitation.SourceMinute
	SourceBase        = internalcitation.SourceBase
	SourceSheet       = internalcitation.SourceSheet
	SourceMeeting     = internalcitation.SourceMeeting
	SourceMeetingNote = internalcitation.SourceMeetingNote
	SourceMindnote    = internalcitation.SourceMindnote
	SourceSlides      = internalcitation.SourceSlides
	SourceFile        = internalcitation.SourceFile
)

// Time normalizes a source timestamp to RFC3339, or returns empty when invalid.
func Time(value any) string { return internalcitation.Time(value) }
