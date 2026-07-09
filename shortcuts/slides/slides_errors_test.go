// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// TestSlidesInputStatError verifies the shared stat-error helper tags the
// offending flag via the typed Param — so callers route on the structured
// field rather than parsing the message — and always classifies as a
// validation error while preserving the underlying cause.
func TestSlidesInputStatError(t *testing.T) {
	t.Parallel()

	if err := slidesInputStatError(nil, "--slides", "ctx"); err != nil {
		t.Fatalf("nil input should return nil, got %v", err)
	}

	tests := []struct {
		name string
		in   error
	}{
		{"path validation", fileio.ErrPathValidation},
		{"generic stat error", errors.New("permission denied")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := slidesInputStatError(tt.in, "--file", "file not found")

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			if ve.Param != "--file" {
				t.Fatalf("Param = %q, want --file", ve.Param)
			}
			if !errors.Is(err, tt.in) {
				t.Fatalf("err must wrap the underlying cause %v", tt.in)
			}
		})
	}
}

// TestAppendSlidesProgressHint covers both branches of the orchestration-hint
// helper: a typed error keeps its classification and gains (or extends) the
// progress hint, while an unclassified error surfaced from a shared-helper
// boundary falls back to a typed internal error that still carries the hint
// and the original cause.
func TestAppendSlidesProgressHint(t *testing.T) {
	t.Parallel()

	if err := appendSlidesProgressHint(nil, "hint"); err != nil {
		t.Fatalf("nil input should return nil, got %v", err)
	}

	t.Run("typed error preserves classification and sets hint", func(t *testing.T) {
		t.Parallel()
		base := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input")
		err := appendSlidesProgressHint(base, "2 image(s) uploaded before failure")

		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %v, want classification preserved as *errs.ValidationError", err)
		}
		p, _ := errs.ProblemOf(err)
		if p.Hint != "2 image(s) uploaded before failure" {
			t.Fatalf("Hint = %q, want the progress hint", p.Hint)
		}
	})

	t.Run("typed error appends to an existing hint", func(t *testing.T) {
		t.Parallel()
		base := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input").WithHint("first")
		err := appendSlidesProgressHint(base, "second")

		p, _ := errs.ProblemOf(err)
		if p.Hint != "first\nsecond" {
			t.Fatalf("Hint = %q, want %q", p.Hint, "first\nsecond")
		}
	})

	t.Run("unclassified error falls back to typed internal error", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("raw boundary error")
		err := appendSlidesProgressHint(cause, "presentation was created")

		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("err = %v, want a typed errs.* error", err)
		}
		if p.Category != errs.CategoryInternal {
			t.Fatalf("Category = %v, want CategoryInternal", p.Category)
		}
		if p.Subtype != errs.SubtypeUnknown {
			t.Fatalf("Subtype = %v, want SubtypeUnknown", p.Subtype)
		}
		if p.Hint != "presentation was created" {
			t.Fatalf("Hint = %q, want the progress hint", p.Hint)
		}
		if !errors.Is(err, cause) {
			t.Fatalf("fallback must preserve the original cause via WithCause")
		}
	})
}
