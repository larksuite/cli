// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// assertValidationParamError asserts that err is a typed *errs.ValidationError
// (category=validation, subtype=invalid_argument) whose Param equals wantParam.
// Message substrings are intentionally NOT asserted — per AGENTS.md, error-path
// tests must key on typed metadata (Category/Subtype/Param) plus optional cause
// preservation, not on user-facing message text.
func assertValidationParamError(t *testing.T, err error, wantParam string) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected typed validation error with param=%q, got nil", wantParam)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Category != errs.CategoryValidation {
		t.Errorf("category = %s, want %s", ve.Category, errs.CategoryValidation)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %s, want %s", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != wantParam {
		t.Errorf("param = %q, want %q", ve.Param, wantParam)
	}
	return ve
}

// assertInternalError asserts err is a typed *errs.InternalError with the given
// subtype. Used to key error-path tests on typed metadata rather than message.
func assertInternalError(t *testing.T, err error, wantSubtype errs.Subtype) *errs.InternalError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected typed internal error subtype=%s, got nil", wantSubtype)
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", err, err)
	}
	if ie.Category != errs.CategoryInternal {
		t.Errorf("category = %s, want %s", ie.Category, errs.CategoryInternal)
	}
	if ie.Subtype != wantSubtype {
		t.Errorf("subtype = %s, want %s", ie.Subtype, wantSubtype)
	}
	return ie
}
