// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "strings"

// Legacy synthetic prefixes an imported "office" document token may carry.
// Exported because callers legitimately need to name them — a test that spells
// "fake_office_" itself would drift from this list the moment it changes.
const (
	FakeOfficeTokenPrefix  = "fake_office_"
	LocalOfficeTokenPrefix = "local_office_"
)

// officeTokenPrefixes is the prefix set IsLocalOfficeToken checks.
var officeTokenPrefixes = []string{FakeOfficeTokenPrefix, LocalOfficeTokenPrefix}

// officeTokenMarker is the interleaved product/region marker an imported office
// token carries.
const officeTokenMarker = "OFL0X"

// officeTokenMarkerOffsets are the byte offsets the marker occupies in an
// interleaved token — positions 5, 10, 15, 20, and 25, 1-based. The array
// length is tied to the marker so the two cannot drift apart silently: adding a
// character to one without the other stops compiling.
var officeTokenMarkerOffsets = [len(officeTokenMarker)]int{4, 9, 14, 19, 24}

// officeTokenMinLen is the shortest token the marker can be read out of, one
// past its last offset. It is a floor rather than an exact length on purpose;
// see IsLocalOfficeToken. TestOfficeTokenMinLenMatchesMarkerOffsets pins it
// to the offsets above.
const officeTokenMinLen = 25

// IsLocalOfficeToken reports whether token names a "local office" document —
// one backed by an imported office file (pptx / xlsx / docx) rather than created
// natively through the API.
//
// "Local office" is the whole category, not the LocalOfficeTokenPrefix case:
// this returns true for FakeOfficeTokenPrefix and for the interleaved marker
// too. The shared word stem is a naming coincidence, not a narrower contract.
//
// This lives in common because the token shape is a drive-level property, not a
// per-domain one: an imported office file is an imported office file whether it
// backs a spreadsheet or a deck. What differs per domain is only the drive media
// parent_type the answer selects — "office_sheet_file" vs "office_slide_file" —
// so that mapping stays with each domain and only the shape is shared. Every
// copy of the shape is somewhere a format change has to be found again; #2509
// had to be applied twice inside sheets alone, and slides was missed entirely.
//
// Two things are load-bearing about how the check is written.
//
// The marker is read at exact offsets, not by a looser strings.Contains, because
// a false positive is the dangerous direction. The drive backend does not
// validate that parent_node actually names an office file, so a native document
// wrongly classified here still uploads successfully — the damage only surfaces
// later as an image that will not render, far from its cause. A false negative
// fails loudly at the upload instead.
//
// The length is a floor rather than one exact value. Because the offsets are
// fixed, a token only has to be long enough to hold the marker; pinning the
// total length silently reclassifies every other length as native. 28 used to be
// pinned and is already stale — per #2509 the local-office format is "OFL0X + 21
// random + 1 office type enum", 27 characters. Widening the length is only safe
// because of the exact offsets: a token of the same length carrying a different
// marker (a native "pptcn" or "shtcn" one) still fails.
func IsLocalOfficeToken(token string) bool {
	for _, prefix := range officeTokenPrefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	if len(token) < officeTokenMinLen {
		return false
	}
	for i, offset := range officeTokenMarkerOffsets {
		if token[offset] != officeTokenMarker[i] {
			return false
		}
	}
	return true
}
