// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errcompat

import (
	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
)

// PromoteAuthError converts a legacy *internalauth.NeedAuthorizationError into
// *errs.AuthenticationError{Subtype: TokenMissing}. The Message field MUST
// contain "need_user_authorization" so the marker invariant guardrail in
// cmd/root_test.go and internal/auth/errors_test.go still holds.
//
// Hint mirrors newTokenMissingError in internal/client/client.go so both
// token-missing surfaces converge on the same recovery vocabulary. cmd's
// applyNeedAuthorizationHint appends per-command scopes onto this Hint with
// a "\n" join, so the action prompt is preserved even when scopes are added.
//
// Called from cmd/root.go.handleRootError when errors.As matches
// *NeedAuthorizationError, before WriteTypedErrorEnvelope.
func PromoteAuthError(err *internalauth.NeedAuthorizationError) error {
	if err == nil {
		return nil
	}
	return errs.NewAuthenticationError(errs.SubtypeTokenMissing,
		"need_user_authorization (user: %s)", err.UserOpenId).
		WithUserOpenID(err.UserOpenId).
		WithHint("run: lark-cli auth login to re-authorize").
		WithCause(err)
}
