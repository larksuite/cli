// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func TestOkrInputStatError(t *testing.T) {
	if okrInputStatError(nil) != nil {
		t.Fatal("nil error should map to nil")
	}

	var ve *errs.ValidationError

	pathCause := errors.New("traversal")
	pathErr := okrInputStatError(&fileio.PathValidationError{Err: pathCause})
	if !errors.As(pathErr, &ve) || ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("path validation: got %T (%v)", pathErr, pathErr)
	}
	if ve.Param != "--file" {
		t.Fatalf("path validation param = %q, want --file", ve.Param)
	}
	if !errors.Is(pathErr, fileio.ErrPathValidation) || !errors.Is(pathErr, pathCause) {
		t.Fatal("path validation cause should be retained")
	}

	genericCause := errors.New("permission denied")
	genericErr := okrInputStatError(genericCause)
	if !errors.As(genericErr, &ve) || ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("generic: got %T (%v)", genericErr, genericErr)
	}
	if ve.Param != "--file" {
		t.Fatalf("generic param = %q, want --file", ve.Param)
	}
	if !errors.Is(genericErr, genericCause) {
		t.Fatal("generic cause should be retained")
	}
}

func TestWrapOkrNetworkErr(t *testing.T) {
	typed := errs.NewValidationError(errs.SubtypeInvalidArgument, "already typed")
	if got := wrapOkrNetworkErr(typed, "wrap %v", typed); got != error(typed) {
		t.Fatalf("typed error must pass through unchanged, got %v", got)
	}

	raw := errors.New("dial tcp: i/o timeout")
	got := wrapOkrNetworkErr(raw, "upload failed: %v", raw)
	var ne *errs.NetworkError
	if !errors.As(got, &ne) || ne.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("raw error: got %T (%v)", got, got)
	}
	if !errors.Is(got, raw) {
		t.Fatal("raw cause should be retained via WithCause")
	}
}
