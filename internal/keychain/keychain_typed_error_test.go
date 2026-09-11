// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// TestWrapErrorEmitsTypedStorageError pins keychain as the single taxonomy
// owner for its storage failures.
func TestWrapErrorEmitsTypedStorageError(t *testing.T) {
	underlying := errors.New("keyring backend exploded")
	err := wrapError("Set", underlying)

	var storageErr *errs.InternalError
	if !errors.As(err, &storageErr) {
		t.Fatalf("wrapError returned %T (%v); expected *errs.InternalError", err, err)
	}
	if storageErr.Subtype != errs.SubtypeStorage {
		t.Errorf("subtype = %q, want %q", storageErr.Subtype, errs.SubtypeStorage)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Errorf("exit code = %d, want %d (ExitInternal)", got, output.ExitInternal)
	}
	if !strings.Contains(storageErr.Message, "keychain Set failed") {
		t.Errorf("message = %q, want it to contain %q", storageErr.Message, "keychain Set failed")
	}
	if storageErr.Hint == "" {
		t.Error("hint is empty; wrapError must carry a troubleshooting hint")
	}
	if !errors.Is(err, underlying) {
		t.Error("underlying error not reachable via errors.Is; WithCause missing")
	}
}

// TestWrapErrorPassthrough pins the non-wrapping paths: nil stays nil and
// ErrNotFound is forwarded untouched so callers can keep using errors.Is.
func TestWrapErrorPassthrough(t *testing.T) {
	if err := wrapError("Get", nil); err != nil {
		t.Errorf("wrapError(nil) = %v, want nil", err)
	}
	if err := wrapError("Get", ErrNotFound); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrapError(ErrNotFound) = %v, want ErrNotFound passthrough", err)
	}
	var apiErr *errs.APIError
	if err := wrapError("Get", ErrNotFound); errors.As(err, &apiErr) {
		t.Errorf("wrapError(ErrNotFound) wrapped into %T; want passthrough", apiErr)
	}
}
