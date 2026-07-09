// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestValidationParamMetadata locks the structured-param contract across the
// sheets domain: a validation failure must carry typed metadata
// (category/subtype via errs.ProblemOf) and tag the offending flag(s) via
// Param/Params, so consumers (agents) know which flag to fix without parsing
// the human message. Covers the four representative shapes — required,
// at-least-one, mutually-exclusive, and local-input-file errors.
func TestValidationParamMetadata(t *testing.T) {
	t.Parallel()

	// assertValidationProblem checks the typed metadata (category + subtype) via
	// errs.ProblemOf and returns the ValidationError for param-level assertions.
	assertValidationProblem := func(t *testing.T, err error) *errs.ValidationError {
		t.Helper()
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("error = %T %v, want typed problem", err, err)
		}
		if p.Category != errs.CategoryValidation {
			t.Errorf("category = %q, want %q", p.Category, errs.CategoryValidation)
		}
		if p.Subtype != errs.SubtypeInvalidArgument {
			t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("error = %T, want *errs.ValidationError", err)
		}
		return ve
	}

	assertParam := func(t *testing.T, err error, want string) {
		t.Helper()
		if ve := assertValidationProblem(t, err); ve.Param != want {
			t.Fatalf("param = %q, want %q", ve.Param, want)
		}
	}

	assertParams := func(t *testing.T, err error, want ...string) {
		t.Helper()
		ve := assertValidationProblem(t, err)
		got := map[string]bool{}
		for _, p := range ve.Params {
			got[p.Name] = true
		}
		if len(ve.Params) != len(want) {
			t.Fatalf("params = %#v, want %v", ve.Params, want)
		}
		for _, w := range want {
			if !got[w] {
				t.Fatalf("params = %#v, missing %q", ve.Params, w)
			}
		}
	}

	t.Run("required flag tags single param", func(t *testing.T) {
		t.Parallel()
		// --image-token satisfies the image source, so the missing --image-name
		// trips the single-flag required check routed through sheetsValidationForFlag.
		fv := newMapFlagViewForCommand("+float-image-create", map[string]interface{}{"image-token": "tok"})
		_, err := floatImageProperties(fv, "", true)
		assertParam(t, err, "--image-name")
	})

	t.Run("at-least-one tags every candidate flag", func(t *testing.T) {
		t.Parallel()
		fv := newMapFlagViewForCommand("+float-image-create", map[string]interface{}{})
		_, err := floatImageProperties(fv, "", true)
		assertParams(t, err, "--image", "--image-token", "--image-uri")
	})

	t.Run("mutually exclusive tags only the conflicting flags", func(t *testing.T) {
		t.Parallel()
		// Only --image and --image-token are set; the param list must not blame
		// the untouched --image-uri.
		fv := newMapFlagViewForCommand("+float-image-create", map[string]interface{}{
			"image":       "a.png",
			"image-token": "tok",
		})
		_, err := floatImageProperties(fv, "", true)
		assertParams(t, err, "--image", "--image-token")
	})

	t.Run("local input file error tags flag and preserves cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("stat failed")
		err := sheetsInputStatError("image", cause)
		assertParam(t, err, "--image")
		if !errors.Is(err, cause) {
			t.Errorf("expected the original stat error preserved as the cause")
		}
	})
}
