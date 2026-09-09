// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

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

// TestCreateLocalOfficeTokenGoldenVectors pins the derivation against tokens
// produced by the desktop client's own fake-token-factory for the same seeds.
//
// These are the whole contract. The token is a local name both sides compute
// independently, so a derivation that is merely self-consistent is worthless —
// it would name a different document than the client does, for every file. The
// vectors cover an empty seed, an absolute path, a non-ASCII path (the seed is
// hashed as UTF-8 bytes, not runes), and a seed longer than the digest.
func TestCreateLocalOfficeTokenGoldenVectors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		seed    string
		docType LocalOfficeDocType
		want    string
	}{
		{"empty seed, sheets", "", LocalOfficeSheets, "FS7VORzoOFgtecLxz4q0qiKuXQE"},
		{"empty seed, slides", "", LocalOfficeSlides, "FS7VORzoOFgtecLxz4q0qiKuXQP"},
		{"empty seed, docx", "", LocalOfficeDocx, "FS7VORzoOFgtecLxz4q0qiKuXQW"},

		{"path seed, sheets", "/tmp/source:converted-result", LocalOfficeSheets, "oyT5ORJIqFnkz4LWbEh0zqeEXAE"},
		{"path seed, slides", "/tmp/source:converted-result", LocalOfficeSlides, "oyT5ORJIqFnkz4LWbEh0zqeEXAP"},
		{"path seed, docx", "/tmp/source:converted-result", LocalOfficeDocx, "oyT5ORJIqFnkz4LWbEh0zqeEXAW"},

		{"non-ASCII path seed", "/Users/me/Documents/年度报表.xlsx", LocalOfficeSheets, "DKToO2li4F3nQRLUrZZ0qWOqXFE"},
		{"path with timestamp seed", "/tmp/a.pptx:1700000000000", LocalOfficeSlides, "qsQgOCHCbFQKNZLGFkH0Q0bfXMP"},
		{"seed longer than the digest", strings.Repeat("a", 300), LocalOfficeDocx, "2NlcOwKdpFocaBLo7GY0sLfSXnW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := CreateLocalOfficeToken(tc.seed, tc.docType)
			if err != nil {
				t.Fatalf("CreateLocalOfficeToken(%q, %q) returned error: %v", tc.seed, tc.docType, err)
			}
			if got != tc.want {
				t.Fatalf("CreateLocalOfficeToken(%q, %q) = %q, want %q", tc.seed, tc.docType, got, tc.want)
			}
		})
	}
}

// TestCreateLocalOfficeTokenShape checks the properties every generated token
// has to hold, whatever the seed: the local-file flows hand the same path back
// in on every open and expect the same document, and the token has to survive
// the reader in this same file.
func TestCreateLocalOfficeTokenShape(t *testing.T) {
	t.Parallel()
	const seed = "/tmp/report.xlsx"
	for _, docType := range []LocalOfficeDocType{LocalOfficeSheets, LocalOfficeSlides, LocalOfficeDocx} {
		t.Run(string(docType), func(t *testing.T) {
			t.Parallel()
			token, err := CreateLocalOfficeToken(seed, docType)
			if err != nil {
				t.Fatalf("CreateLocalOfficeToken returned error: %v", err)
			}
			if len(token) != localOfficeTokenLen {
				t.Fatalf("len(%q) = %d, want %d", token, len(token), localOfficeTokenLen)
			}
			if !IsLocalOfficeToken(token) {
				t.Fatalf("IsLocalOfficeToken(%q) = false, want true", token)
			}
			if token[len(token)-1] != docType.Suffix() {
				t.Fatalf("token %q ends with %q, want %q", token, token[len(token)-1], docType.Suffix())
			}
			for _, c := range token {
				if !strings.ContainsRune(localOfficeTokenAlphabet, c) {
					t.Fatalf("token %q contains non-base62 character %q", token, c)
				}
			}
			again, err := CreateLocalOfficeToken(seed, docType)
			if err != nil {
				t.Fatalf("second CreateLocalOfficeToken returned error: %v", err)
			}
			if again != token {
				t.Fatalf("CreateLocalOfficeToken is not deterministic: %q then %q", token, again)
			}
		})
	}
}

// TestCreateLocalOfficeTokenDocTypeOnlyChangesSuffix keeps the document type
// out of the hashed seed. The desktop client derives one body per seed and only
// swaps the trailing enum, so a token that varied earlier than its last
// character would disagree with it for two of the three types.
func TestCreateLocalOfficeTokenDocTypeOnlyChangesSuffix(t *testing.T) {
	t.Parallel()
	const seed = "/tmp/source:fork-result"
	var bodies, suffixes []string
	for _, docType := range []LocalOfficeDocType{LocalOfficeSheets, LocalOfficeSlides, LocalOfficeDocx} {
		token, err := CreateLocalOfficeToken(seed, docType)
		if err != nil {
			t.Fatalf("CreateLocalOfficeToken(%q, %q) returned error: %v", seed, docType, err)
		}
		bodies = append(bodies, token[:len(token)-1])
		suffixes = append(suffixes, token[len(token)-1:])
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Fatalf("token bodies differ by doc type: %q vs %q", bodies[0], bodies[i])
		}
	}
	if want := []string{"E", "P", "W"}; !reflect.DeepEqual(suffixes, want) {
		t.Fatalf("suffixes = %v, want %v", suffixes, want)
	}
}

// TestCreateLocalOfficeTokenRejectsUnknownDocType pins the refusal to emit a
// token with no document-type enum. A 26-character token still satisfies
// IsLocalOfficeToken — the marker offsets are all it reads — so the mistake
// would travel as a plausible token and only fail much later at an upload.
func TestCreateLocalOfficeTokenRejectsUnknownDocType(t *testing.T) {
	t.Parallel()
	for _, docType := range []LocalOfficeDocType{"", "sheet", "doc", "SHEETS", "pptx"} {
		t.Run(string(docType), func(t *testing.T) {
			t.Parallel()
			token, err := CreateLocalOfficeToken("/tmp/report.xlsx", docType)
			if err == nil {
				t.Fatalf("CreateLocalOfficeToken(_, %q) = %q, want an error", docType, token)
			}
			if token != "" {
				t.Fatalf("CreateLocalOfficeToken(_, %q) returned token %q alongside its error", docType, token)
			}
			var validation *errs.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("CreateLocalOfficeToken(_, %q) error = %T, want *errs.ValidationError", docType, err)
			}
		})
	}
}

// TestLocalOfficeDocTypeSuffixes keeps the enum characters distinct and keeps
// the zero suffix reserved for an unknown type, which is what
// CreateLocalOfficeToken tests to reject one.
func TestLocalOfficeDocTypeSuffixes(t *testing.T) {
	t.Parallel()
	seen := map[byte]LocalOfficeDocType{}
	for docType, suffix := range localOfficeDocTypeSuffixes {
		if suffix == 0 {
			t.Fatalf("doc type %q has the zero suffix reserved for unknown types", docType)
		}
		if other, dup := seen[suffix]; dup {
			t.Fatalf("doc types %q and %q share suffix %q", other, docType, suffix)
		}
		seen[suffix] = docType
	}
	if got := LocalOfficeDocType("nope").Suffix(); got != 0 {
		t.Fatalf("unknown doc type Suffix() = %q, want 0", got)
	}
}
