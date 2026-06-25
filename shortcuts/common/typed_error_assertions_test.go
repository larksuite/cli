// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// requireProblem asserts err carries a typed errs.Problem with the given
// category and (optional) subtype, and that its message contains msgContains
// (skip the message check by passing ""). Returns the Problem so callers can
// drill into the typed envelope's category-specific fields (e.g. cast to
// *errs.ValidationError to read .Param / .Params / .Cause).
func requireProblem(t *testing.T, err error, wantCategory errs.Category, wantSubtype errs.Subtype, msgContains string) *errs.Problem {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error carrying errs.Problem, got %T: %v", err, err)
	}
	if p.Category != wantCategory {
		t.Errorf("category = %q, want %q (err=%v)", p.Category, wantCategory, err)
	}
	if wantSubtype != "" && p.Subtype != wantSubtype {
		t.Errorf("subtype = %q, want %q (err=%v)", p.Subtype, wantSubtype, err)
	}
	if msgContains != "" && !strings.Contains(p.Message, msgContains) {
		t.Errorf("message = %q, want containing %q", p.Message, msgContains)
	}
	return p
}

// requireValidation is shorthand for CategoryValidation + SubtypeInvalidArgument.
// Returns *errs.ValidationError so callers can also assert on .Param / .Params / .Cause.
func requireValidation(t *testing.T, err error, msgContains string) *errs.ValidationError {
	t.Helper()
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, msgContains)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	return ve
}
