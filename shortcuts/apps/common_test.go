// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestWithAppsHint(t *testing.T) {
	t.Run("nil error stays nil", func(t *testing.T) {
		if got := withAppsHint(nil, "do x"); got != nil {
			t.Fatalf("withAppsHint(nil) = %v, want nil", got)
		}
	})

	t.Run("empty hint gets filled, code/type preserved", func(t *testing.T) {
		in := &output.ExitError{Code: 1, Detail: &output.ErrDetail{Type: "api_error", Message: "boom"}}
		out := withAppsHint(in, "run +release-list")
		var exitErr *output.ExitError
		if !errors.As(out, &exitErr) {
			t.Fatalf("returned error is not *output.ExitError: %T", out)
		}
		if exitErr.Detail.Hint != "run +release-list" {
			t.Errorf("Hint = %q, want %q", exitErr.Detail.Hint, "run +release-list")
		}
		if exitErr.Code != 1 || exitErr.Detail.Type != "api_error" || exitErr.Detail.Message != "boom" {
			t.Errorf("code/type/message mutated: code=%d type=%q msg=%q", exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message)
		}
	})

	t.Run("existing hint is preserved, not clobbered", func(t *testing.T) {
		in := output.ErrWithHint(1, "api_error", "boom", "original hint")
		out := withAppsHint(in, "new hint")
		var exitErr *output.ExitError
		if !errors.As(out, &exitErr) {
			t.Fatalf("returned error is not *output.ExitError: %T", out)
		}
		if exitErr.Detail.Hint != "original hint" {
			t.Errorf("Hint = %q, want preserved %q", exitErr.Detail.Hint, "original hint")
		}
	})

	t.Run("blank-whitespace hint is treated as empty and filled", func(t *testing.T) {
		in := output.ErrWithHint(1, "api_error", "boom", "   ")
		out := withAppsHint(in, "filled hint")
		var exitErr *output.ExitError
		if !errors.As(out, &exitErr) {
			t.Fatalf("returned error is not *output.ExitError: %T", out)
		}
		if exitErr.Detail.Hint != "filled hint" {
			t.Errorf("Hint = %q, want %q", exitErr.Detail.Hint, "filled hint")
		}
	})

	t.Run("unrecognized error type returned unchanged, no panic", func(t *testing.T) {
		in := errors.New("plain")
		out := withAppsHint(in, "ignored")
		if out == nil || out.Error() != "plain" {
			t.Fatalf("withAppsHint(plain) = %v, want unchanged plain error", out)
		}
	})
}
