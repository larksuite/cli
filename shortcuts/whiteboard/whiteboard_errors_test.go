// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func TestWrapWbNetworkErr(t *testing.T) {
	typed := errs.NewAPIError(errs.SubtypeRateLimit, "already typed")
	if got := wrapWbNetworkErr(typed, "wrap"); got != error(typed) {
		t.Fatalf("typed error must pass through unchanged, got %v", got)
	}

	raw := errors.New("connection reset by peer")
	got := wrapWbNetworkErr(raw, "fetch failed: %v", raw)
	var ne *errs.NetworkError
	if !errors.As(got, &ne) || ne.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("raw error: got %T (%v)", got, got)
	}
	if !errors.Is(got, raw) {
		t.Fatal("raw cause should be retained via WithCause")
	}
}

func TestWbSaveError(t *testing.T) {
	if wbSaveError(nil) != nil {
		t.Fatal("nil error should map to nil")
	}

	var ve *errs.ValidationError
	pathCause := errors.New("escape")
	if got := wbSaveError(&fileio.PathValidationError{Err: pathCause}); !errors.As(got, &ve) || ve.Subtype != errs.SubtypeInvalidArgument || !errors.Is(got, pathCause) {
		t.Fatalf("path validation: got %T (%v)", got, got)
	}
	if ve.Param != "--output" {
		t.Fatalf("path validation param = %q, want --output", ve.Param)
	}

	var ie *errs.InternalError
	mkdirCause := errors.New("mkdir denied")
	if got := wbSaveError(&fileio.MkdirError{Err: mkdirCause}); !errors.As(got, &ie) || ie.Subtype != errs.SubtypeFileIO || !errors.Is(got, mkdirCause) {
		t.Fatalf("mkdir: got %T (%v)", got, got)
	}
	writeCause := errors.New("disk full")
	if got := wbSaveError(writeCause); !errors.As(got, &ie) || ie.Subtype != errs.SubtypeFileIO || !errors.Is(got, writeCause) {
		t.Fatalf("default: got %T (%v)", got, got)
	}
}
