// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestWrapIMNetworkErr_PassthroughTyped(t *testing.T) {
	typed := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input")
	got := wrapIMNetworkErr(typed, "download failed")
	if got != error(typed) {
		t.Fatalf("typed error must be passed through unchanged, got %v", got)
	}
}

func TestWrapIMNetworkErr_WrapsRaw(t *testing.T) {
	raw := errors.New("dial tcp: i/o timeout")
	got := wrapIMNetworkErr(raw, "download failed: %s", "x")
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("raw error must become *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkTransport {
		t.Errorf("subtype = %q, want %q", ne.Subtype, errs.SubtypeNetworkTransport)
	}
	if !errors.Is(got, raw) {
		t.Errorf("cause must be chained for errors.Is")
	}
}

func TestAppendIMRecoveryHint_TypedPreservedHintAppended(t *testing.T) {
	typed := errs.NewAPIError(errs.SubtypeNotFound, "message not found")
	got := appendIMRecoveryHint(typed, "specify --item-type explicitly")
	if got != error(typed) {
		t.Fatalf("typed error must be returned unchanged, got %T", got)
	}
	var ae *errs.APIError
	if !errors.As(got, &ae) {
		t.Fatalf("typed classification must be preserved, got %T", got)
	}
	if ae.Subtype != errs.SubtypeNotFound {
		t.Errorf("subtype = %q, want %q", ae.Subtype, errs.SubtypeNotFound)
	}
	p, ok := errs.ProblemOf(got)
	if !ok || p.Hint != "specify --item-type explicitly" {
		t.Errorf("hint = %q (ok=%v), want %q", p.Hint, ok, "specify --item-type explicitly")
	}
}

func TestAppendIMRecoveryHint_RawBecomesInternal(t *testing.T) {
	got := appendIMRecoveryHint(errors.New("boom"), "specify --item-type explicitly")
	var ie *errs.InternalError
	if !errors.As(got, &ie) {
		t.Fatalf("raw error must become *errs.InternalError, got %T", got)
	}
	if ie.Hint != "specify --item-type explicitly" {
		t.Errorf("hint = %q, want %q", ie.Hint, "specify --item-type explicitly")
	}
}

func TestAppendIMRecoveryHint_Nil(t *testing.T) {
	if appendIMRecoveryHint(nil, "hint") != nil {
		t.Errorf("nil in -> nil out")
	}
}

func TestAppendIMRecoveryHint_AppendsExistingHint(t *testing.T) {
	typed := errs.NewAPIError(errs.SubtypeNotFound, "message not found").WithHint("first")
	got := appendIMRecoveryHint(typed, "second")
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T", got)
	}
	if p.Hint != "first\nsecond" {
		t.Errorf("hint = %q, want %q", p.Hint, "first\nsecond")
	}
}
