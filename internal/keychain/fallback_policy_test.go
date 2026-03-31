// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"errors"
	"testing"
)

func TestShouldUseFallback_RequiresTypedUnavailableError(t *testing.T) {
	if ShouldUseFallback(errors.New("exit status 155")) {
		t.Fatal("expected raw error strings to stop triggering fallback")
	}

	if !ShouldUseFallback(WrapUnavailable(errors.New("sandbox denied"))) {
		t.Fatal("expected wrapped unavailable errors to trigger fallback")
	}
}
