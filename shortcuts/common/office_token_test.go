// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "testing"

// TestIsLocalOfficeToken pins the shared token shape every domain's
// parent_type mapping now reads through.
//
// The negative cases carry as much weight as the positive ones. A native token
// read as office still uploads successfully — the drive backend does not
// validate that parent_node names an office file — so the failure only shows up
// later as an image that will not render. The interleaved native tokens are what
// make the length floor safe: they are long enough to be read but carry a
// different marker, so relaxing the length must not pull them in.
func TestIsLocalOfficeToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		token string
		want  bool
	}{
		{"empty token", "", false},
		{"native token, too short to read", "pptcnABC123", false},

		{"fake_office prefix", "fake_office_abc123", true},
		{"fake_office, only the prefix", FakeOfficeTokenPrefix, true},
		{"local_office prefix", "local_office_abc123", true},
		{"local_office, only the prefix", LocalOfficeTokenPrefix, true},
		{"fake_office prefix mid-string is not a prefix", "pptfake_office_abc", false},
		{"local_office prefix mid-string is not a prefix", "pptlocal_office_abc", false},

		{"interleaved OFL0X, 25 chars (marker exactly fills the token)", "aaaaOaaaaFaaaaLaaaa0aaaaX", true},
		{"interleaved OFL0X, 27 chars (current local-office format)", "aaaaOaaaaFaaaaLaaaa0aaaaXaa", true},
		{"interleaved OFL0X, 28 chars (the length that used to be pinned)", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", true},
		{"interleaved OFL0X, 28 chars with ppt office-type enum", "ccccOccccFccccLcccc0ccccXccP", true},
		{"interleaved OFL0X, 29 chars (longer than any known format)", "aaaaOaaaaFaaaaLaaaa0aaaaXaaaa", true},
		{"interleaved OFL0X, 24 chars (one short of holding the marker)", "aaaaOaaaaFaaaaLaaaa0aaaa", false},

		{"interleaved pptcn native token", "abcdpefghpijkltmnopcqrstnuv", false},
		{"interleaved shtcn native token", "abcdsefghhijkltmnopcqrstnuv", false},
		{"OFL0X present but not on the offsets", "OFL0Xaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"marker off by one offset", "aaaaaOaaaaFaaaaLaaaa0aaaaX", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsLocalOfficeToken(tc.token); got != tc.want {
				t.Fatalf("IsLocalOfficeToken(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}

// TestOfficeTokenMinLenMatchesMarkerOffsets keeps the length floor derived from
// the offsets rather than merely agreeing with them today. A floor that drifts
// above the last offset would reject valid short tokens; one that drifts below
// it would index out of range.
func TestOfficeTokenMinLenMatchesMarkerOffsets(t *testing.T) {
	t.Parallel()
	last := officeTokenMarkerOffsets[len(officeTokenMarkerOffsets)-1]
	if officeTokenMinLen != last+1 {
		t.Fatalf("officeTokenMinLen = %d, want %d (one past last marker offset %d)",
			officeTokenMinLen, last+1, last)
	}
}
