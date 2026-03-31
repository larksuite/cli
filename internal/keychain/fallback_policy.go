// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"errors"
)

// ShouldUseFallback reports whether a keychain write failure should degrade to
// the CLI-managed encrypted fallback store.
// New fallback-eligible errors should be expressed via ErrUnavailable /
// WrapUnavailable so callers share one typed contract instead of matching text.
func ShouldUseFallback(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrUnavailable)
}
