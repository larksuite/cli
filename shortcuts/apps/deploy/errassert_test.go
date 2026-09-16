// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// requireValidation asserts that err is the typed validation error the command
// layer expects, and returns it so a test can go on to check the fields that
// matter to it.
//
// Matching on message text alone would keep passing if the error stopped being
// typed, or lost its subtype, or dropped the cause it wraps -- the envelope a
// caller parses would break while the test stayed green.
func requireValidation(t *testing.T, err error, allowed ...errs.Subtype) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	for _, want := range allowed {
		if ve.Subtype == want {
			return ve
		}
	}
	t.Errorf("Subtype = %q, want one of %v", ve.Subtype, allowed)
	return ve
}

// requireCause asserts that the error preserves the underlying failure, so the
// reason an I/O path gave is still reachable from the envelope.
func requireCause(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("error should preserve its cause %v, got %v", target, err)
	}
}
