// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapInternalPlainError(t *testing.T) {
	orig := fmt.Errorf("boom")
	wrapped := WrapInternal(orig)

	var ie *InternalError
	if !errors.As(wrapped, &ie) {
		t.Fatalf("WrapInternal did not produce *InternalError; got %T", wrapped)
	}
	if ie.Category != CategoryInternal {
		t.Errorf("Category = %q, want %q", ie.Category, CategoryInternal)
	}
	if ie.Subtype != SubtypeUnknown {
		t.Errorf("Subtype = %q, want %q", ie.Subtype, SubtypeUnknown)
	}
	if ie.Message != "boom" {
		t.Errorf("Message = %q, want %q", ie.Message, "boom")
	}
	if ie.Cause != orig {
		t.Errorf("Cause = %v, want original error %v", ie.Cause, orig)
	}
	if got := errors.Unwrap(wrapped); got != orig {
		t.Errorf("errors.Unwrap = %v, want original %v", got, orig)
	}
}

func TestWrapInternalPassesThroughTyped(t *testing.T) {
	apiErr := &APIError{Problem: Problem{Category: CategoryAPI, Message: "api boom"}}
	got := WrapInternal(apiErr)
	if got != apiErr {
		t.Errorf("WrapInternal should pass through typed errors unchanged; got %#v want %#v", got, apiErr)
	}
}

func TestWrapInternalNil(t *testing.T) {
	if got := WrapInternal(nil); got != nil {
		t.Errorf("WrapInternal(nil) = %v, want nil", got)
	}
}
