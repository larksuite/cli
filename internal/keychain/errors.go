// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"errors"
	"fmt"
)

// ErrUnavailable marks failures where the platform keychain cannot be used and
// callers may degrade to the CLI-managed encrypted fallback store.
var ErrUnavailable = errors.New("keychain unavailable")

// WrapUnavailable annotates a lower-level keychain error as fallback-eligible.
func WrapUnavailable(err error) error {
	if err == nil || errors.Is(err, ErrUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}
