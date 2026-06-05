// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "fmt"

// Identity is the identity taxonomy a command supports.
//
// Defined type (not alias) so plugin authors get compile-time +
// IDE help; raw-string boundaries (yaml, cobra annotation) cross
// through ParseIdentity.
type Identity string

const (
	IdentityUser Identity = "user"
	IdentityBot  Identity = "bot"
)

// ParseIdentity converts a raw string into an Identity. Returns
// ("", nil) for empty input ("not specified"), error for unrecognised
// values. Matching is strict (case-sensitive, no trim).
func ParseIdentity(s string) (Identity, error) {
	if s == "" {
		return "", nil
	}
	id := Identity(s)
	if id != IdentityUser && id != IdentityBot {
		return "", fmt.Errorf("invalid identity %q: must be user|bot", s)
	}
	return id, nil
}

// IsValid reports whether i is one of the two recognised values.
func (i Identity) IsValid() bool {
	return i == IdentityUser || i == IdentityBot
}

// String returns the underlying string.
func (i Identity) String() string { return string(i) }
