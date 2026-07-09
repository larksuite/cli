// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestMarkRawNilReturnsNil(t *testing.T) {
	if got := errs.MarkRaw(nil); got != nil {
		t.Fatalf("MarkRaw(nil) = %v, want nil", got)
	}
}

func TestIsRaw(t *testing.T) {
	base := fmt.Errorf("boom")

	if !errs.IsRaw(errs.MarkRaw(base)) {
		t.Errorf("IsRaw(MarkRaw(err)) = false, want true")
	}
	if errs.IsRaw(base) {
		t.Errorf("IsRaw(bare err) = true, want false")
	}
	if errs.IsRaw(nil) {
		t.Errorf("IsRaw(nil) = true, want false")
	}

	// Raw marking survives further wrapping above it in the chain.
	wrapped := fmt.Errorf("outer: %w", errs.MarkRaw(base))
	if !errs.IsRaw(wrapped) {
		t.Errorf("IsRaw(wrap(MarkRaw(err))) = false, want true")
	}
}

func TestMarkRawPreservesErrorMessage(t *testing.T) {
	base := fmt.Errorf("boom")
	if got := errs.MarkRaw(base).Error(); got != "boom" {
		t.Fatalf("MarkRaw(err).Error() = %q, want %q", got, "boom")
	}
}

func TestMarkRawPreservesErrorsIsChain(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := fmt.Errorf("ctx: %w", sentinel)

	if !errors.Is(errs.MarkRaw(wrapped), sentinel) {
		t.Fatalf("errors.Is(MarkRaw(err), sentinel) = false, want true")
	}
}

func TestProblemOfPunchesThroughMarkRaw(t *testing.T) {
	typed := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad flag")
	raw := errs.MarkRaw(typed)

	p, ok := errs.ProblemOf(raw)
	if !ok {
		t.Fatalf("ProblemOf(MarkRaw(typed)) ok = false, want true")
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("ProblemOf(MarkRaw(typed)).Category = %v, want %v", p.Category, errs.CategoryValidation)
	}

	// errors.As still finds the concrete typed error through the raw wrapper.
	var ve *errs.ValidationError
	if !errors.As(raw, &ve) {
		t.Errorf("errors.As(MarkRaw(typed), *ValidationError) = false, want true")
	}
}

// TestMarkRawUnwrapsToInnerTypedError pins the envelope-serialization
// contract: UnwrapTypedError must return the inner concrete typed error,
// not the rawPassthrough wrapper. The wrapper has no exported fields, so if it
// were returned the JSON envelope would marshal to an empty "{}" error.
func TestMarkRawUnwrapsToInnerTypedError(t *testing.T) {
	base := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad flag")
	typed, ok := errs.UnwrapTypedError(errs.MarkRaw(base))
	if !ok {
		t.Fatal("UnwrapTypedError(MarkRaw(typed)) must find a typed error")
	}
	out, err := json.Marshal(typed)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) == "{}" {
		t.Fatalf("UnwrapTypedError returned the opaque rawPassthrough wrapper; envelope would be empty: %s", out)
	}
	if got := errs.CategoryOf(typed); got != errs.CategoryValidation {
		t.Fatalf("unwrapped category = %q, want validation", got)
	}
}
