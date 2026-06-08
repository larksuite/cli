// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
)

func TestTaskInputStatError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		if err := taskInputStatError(nil, "--file"); err != nil {
			t.Errorf("taskInputStatError(nil) = %v, want nil", err)
		}
	})

	t.Run("path validation failure maps to unsafe file path", func(t *testing.T) {
		err := taskInputStatError(fmt.Errorf("bad: %w", fileio.ErrPathValidation), "--file")
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %T, want *errs.ValidationError", err)
		}
		if ve.Subtype != errs.SubtypeInvalidArgument {
			t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
		}
		if output.ExitCodeOf(err) != output.ExitValidation {
			t.Errorf("exit = %d, want %d", output.ExitCodeOf(err), output.ExitValidation)
		}
		if !strings.Contains(err.Error(), "unsafe file path") {
			t.Errorf("message = %q, want 'unsafe file path'", err.Error())
		}
		if ve.Param != "--file" {
			t.Errorf("param = %q, want --file", ve.Param)
		}
	})

	t.Run("generic error uses readMsg prefix", func(t *testing.T) {
		err := taskInputStatError(errors.New("permission denied"), "--file", "cannot access file")
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %T, want *errs.ValidationError", err)
		}
		if !strings.Contains(err.Error(), "cannot access file") {
			t.Errorf("message = %q, want 'cannot access file' prefix", err.Error())
		}
		if ve.Param != "--file" {
			t.Errorf("param = %q, want --file", ve.Param)
		}
	})

	t.Run("default prefix when no readMsg", func(t *testing.T) {
		err := taskInputStatError(errors.New("boom"), "--file")
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %T, want *errs.ValidationError", err)
		}
		if !strings.Contains(err.Error(), "cannot read file") {
			t.Errorf("message = %q, want default 'cannot read file'", err.Error())
		}
		if ve.Param != "--file" {
			t.Errorf("param = %q, want --file", ve.Param)
		}
	})
}

func TestWrapTaskNetworkErr(t *testing.T) {
	// wrapTaskNetworkErr is only ever called inside an `if err != nil` guard
	// (DoAPIStream failure), mirroring drive's wrapDriveNetworkErr, so it does
	// not special-case a nil cause.
	t.Run("untyped cause becomes typed network error wrapping the cause", func(t *testing.T) {
		cause := errors.New("dial timeout")
		err := wrapTaskNetworkErr(cause, "upload failed")
		var ne *errs.NetworkError
		if !errors.As(err, &ne) {
			t.Fatalf("err = %T, want *errs.NetworkError", err)
		}
		if ne.Subtype != errs.SubtypeNetworkTransport {
			t.Errorf("subtype = %q, want %q", ne.Subtype, errs.SubtypeNetworkTransport)
		}
		if !errors.Is(err, cause) {
			t.Error("expected the original cause to be wrapped (errors.Is)")
		}
	})

	t.Run("already-typed cause is passed through unchanged", func(t *testing.T) {
		typed := errs.NewAPIError(errs.SubtypeNotFound, "missing")
		err := wrapTaskNetworkErr(typed, "upload failed")
		var ae *errs.APIError
		if !errors.As(err, &ae) {
			t.Fatalf("err = %T, want the original *errs.APIError passed through", err)
		}
		if ae.Subtype != errs.SubtypeNotFound {
			t.Errorf("subtype = %q, want %q (not re-wrapped as network)", ae.Subtype, errs.SubtypeNotFound)
		}
	})
}
