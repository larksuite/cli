// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"errors"
)

// ErrUnavailable marks failures where the platform keychain cannot be used and
// callers may degrade to the CLI-managed encrypted fallback store.
var ErrUnavailable = errors.New("keychain unavailable")

// unavailableError preserves both the fallback-eligibility marker and the underlying platform error.
type unavailableError struct {
	cause error
}

// Error implements error.
func (e *unavailableError) Error() string {
	return ErrUnavailable.Error() + ": " + e.cause.Error()
}

// Unwrap exposes both ErrUnavailable and the underlying cause.
func (e *unavailableError) Unwrap() []error {
	return []error{ErrUnavailable, e.cause}
}

// WrapUnavailable annotates a lower-level keychain error as fallback-eligible.
func WrapUnavailable(err error) error {
	if err == nil || errors.Is(err, ErrUnavailable) {
		return err
	}
	return &unavailableError{cause: err}
}
