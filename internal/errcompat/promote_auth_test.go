// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errcompat

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
)

func TestPromoteAuthError_PromotesNeedAuthorizationError(t *testing.T) {
	needAuth := &internalauth.NeedAuthorizationError{UserOpenId: "u_xxx"}
	got := PromoteAuthError(needAuth)

	var authErr *errs.AuthenticationError
	if !errors.As(got, &authErr) {
		t.Fatalf("expected *errs.AuthenticationError, got %T", got)
	}
	if authErr.Subtype != errs.SubtypeTokenMissing {
		t.Errorf("subtype = %v, want %v", authErr.Subtype, errs.SubtypeTokenMissing)
	}

	// Cause chain must preserve original *NeedAuthorizationError so legacy
	// consumers (auth.IsNeedUserAuthorizationError + errors.As pattern in
	// internal/auth/errors.go:42) still match.
	var preserved *internalauth.NeedAuthorizationError
	if !errors.As(got, &preserved) {
		t.Error("Unwrap chain lost *NeedAuthorizationError — breaks auth.IsNeedUserAuthorizationError consumer")
	}
}

func TestPromoteAuthError_PreservesNeedUserAuthorizationMarker(t *testing.T) {
	needAuth := &internalauth.NeedAuthorizationError{UserOpenId: "u_xxx"}
	got := PromoteAuthError(needAuth)
	if !strings.Contains(got.Error(), "need_user_authorization") {
		t.Errorf("Message must contain need_user_authorization marker, got: %q", got.Error())
	}
}

func TestPromoteAuthError_PreservesUserOpenID(t *testing.T) {
	needAuth := &internalauth.NeedAuthorizationError{UserOpenId: "u_test_open_id"}
	got := PromoteAuthError(needAuth)

	var authErr *errs.AuthenticationError
	if !errors.As(got, &authErr) {
		t.Fatalf("expected *errs.AuthenticationError, got %T", got)
	}
	if authErr.UserOpenID != "u_test_open_id" {
		t.Errorf("UserOpenID = %q, want preserved", authErr.UserOpenID)
	}
}

// TestPromoteAuthError_CarriesAuthLoginHint pins that the recovery action
// prompt is attached at promotion time — without this Hint, downstream
// consumers see authentication/token_missing but no "run: lark-cli auth login"
// guidance, mirroring the pre-typed UX failure when NeedAuthorizationError
// surfaced as a bare network error. cmd's applyNeedAuthorizationHint relies
// on this Hint being non-empty so scope enrichment appends instead of
// overwrites the recovery prompt.
func TestPromoteAuthError_CarriesAuthLoginHint(t *testing.T) {
	got := PromoteAuthError(&internalauth.NeedAuthorizationError{UserOpenId: "u_xxx"})
	var authErr *errs.AuthenticationError
	if !errors.As(got, &authErr) {
		t.Fatalf("expected *errs.AuthenticationError, got %T", got)
	}
	if !strings.Contains(authErr.Hint, "lark-cli auth login") {
		t.Errorf("Hint must guide user to re-authorize, got: %q", authErr.Hint)
	}
}

func TestPromoteAuthError_Nil_ReturnsNil(t *testing.T) {
	if got := PromoteAuthError(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}
