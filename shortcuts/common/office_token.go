// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"crypto/sha256"
	"math/big"
	"strings"
)

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

// localOfficeTokenLen is the total length of a generated local-office token:
// the interleaved marker, the base62 body, and the trailing document-type
// enum. IsLocalOfficeToken deliberately accepts other lengths (see its
// comment); this is the one length CreateLocalOfficeToken emits.
const localOfficeTokenLen = 27

// localOfficeTokenBodyLen is how many base62 characters the token carries once
// the marker and the document-type enum have taken their positions. Derived
// rather than written as 21 so the three parts always add up.
const localOfficeTokenBodyLen = localOfficeTokenLen - len(officeTokenMarker) - 1

// localOfficeTokenAlphabet is the base62 digit set, uppercase before lowercase
// before digits. The order is part of the token format, not a detail: the
// desktop client derives the same token from the same seed, so any reordering
// silently produces a different document.
const localOfficeTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// LocalOfficeDocType names the kind of document a local office token stands
// for. The values match the desktop client's own doc types, including "docx"
// for a Word document, because the two sides have to agree on the token's
// trailing enum character.
type LocalOfficeDocType string

const (
	LocalOfficeSheets LocalOfficeDocType = "sheets"
	LocalOfficeSlides LocalOfficeDocType = "slides"
	LocalOfficeDocx   LocalOfficeDocType = "docx"
)

// localOfficeDocTypeSuffixes maps each document type to the character a local
// office token ends with. Domains already read this enum off a token — see the
// office_docx_file parent_type rule in shortcuts/doc — so the mapping lives
// with the rest of the token shape rather than being spelled out per domain.
var localOfficeDocTypeSuffixes = map[LocalOfficeDocType]byte{
	LocalOfficeSheets: 'E',
	LocalOfficeSlides: 'P',
	LocalOfficeDocx:   'W',
}

// Suffix returns the character a local office token of this document type ends
// with, or 0 for a type that has none.
func (d LocalOfficeDocType) Suffix() byte {
	return localOfficeDocTypeSuffixes[d]
}

// CreateLocalOfficeToken derives the local-office document token a seed maps
// to — for the local-file open flows, the seed is the file's path, so the same
// path always names the same document.
//
// The token is not allocated by any backend. It is a deterministic local name
// the desktop client computes the same way from the same seed
// (fake-token-factory.ts), which is what lets both sides talk about a document
// that only exists on disk. Reproducing the derivation is therefore the whole
// point: seed and document type in, the same 27 characters out, on both sides.
//
// The derivation is SHA-256 of the seed, read as one big-endian integer and
// emitted least-significant digit first in base62 until the body is full. Only
// the low ~125 bits of the hash survive 21 base62 digits; that is the format,
// not an oversight, and there is nothing to reverse a token back into a path.
//
// An unknown document type is an error rather than a token with no enum
// character. Truncating the enum would still leave a token IsLocalOfficeToken
// accepts — the marker offsets are all that it reads — so the mistake would
// travel as a plausible-looking token and only surface as a failed upload much
// later.
func CreateLocalOfficeToken(seed string, docType LocalOfficeDocType) (string, error) {
	suffix := docType.Suffix()
	if suffix == 0 {
		return "", ValidationErrorf("unknown local office doc type %q, expected one of: %s, %s, %s",
			string(docType), LocalOfficeSheets, LocalOfficeSlides, LocalOfficeDocx)
	}

	digest := sha256.Sum256([]byte(seed))
	body := localOfficeTokenBody(digest[:])

	// Lay the token out from the same offsets IsLocalOfficeToken reads the
	// marker at, so the writer and the reader cannot drift apart. Every
	// position that is not a marker and not the trailing enum takes the next
	// body character, which is the interleaving the desktop client builds by
	// splicing four body characters before each marker character.
	token := make([]byte, localOfficeTokenLen)
	for i, offset := range officeTokenMarkerOffsets {
		token[offset] = officeTokenMarker[i]
	}
	token[localOfficeTokenLen-1] = suffix
	next := 0
	for i := range token {
		if token[i] == 0 {
			token[i] = body[next]
			next++
		}
	}
	return string(token), nil
}

// localOfficeTokenBody renders digest as base62 digits, least significant
// first, truncated to the body length.
func localOfficeTokenBody(digest []byte) []byte {
	n := new(big.Int).SetBytes(digest)
	radix := big.NewInt(int64(len(localOfficeTokenAlphabet)))
	remainder := new(big.Int)
	body := make([]byte, localOfficeTokenBodyLen)
	for i := range body {
		n.QuoRem(n, radix, remainder)
		body[i] = localOfficeTokenAlphabet[remainder.Int64()]
	}
	return body
}
