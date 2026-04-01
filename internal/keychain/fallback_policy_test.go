// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"context"
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

func TestWrapUnavailable_PreservesUnderlyingCause(t *testing.T) {
	err := WrapUnavailable(context.DeadlineExceeded)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("expected ErrUnavailable to remain detectable")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected underlying cause to remain detectable")
	}
}
