// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
)

const (
	needUserAuthorizationMarker = "need_user_authorization"
)

// TokenRetryCodes contains error codes that allow retry after token refresh.
var TokenRetryCodes = map[int]bool{
	output.LarkErrTokenInvalid: true,
	output.LarkErrTokenExpired: true,
}

// NeedAuthorizationError is the sentinel preserved in the Cause chain of the
// typed missing-UAT error so existing errors.As(&NeedAuthorizationError{})
// consumers keep matching after the construction site moved to the typed
// taxonomy. It is never surfaced on the wire on its own.
type NeedAuthorizationError struct {
	UserOpenId string
}

// Error returns the error message for NeedAuthorizationError.
func (e *NeedAuthorizationError) Error() string {
	return fmt.Sprintf("%s (user: %s)", needUserAuthorizationMarker, e.UserOpenId)
}

// NewNeedUserAuthorizationError builds the typed *errs.AuthenticationError
// returned when no valid UAT exists for userOpenID. The Message keeps the
// need_user_authorization marker, the Hint converges on the same auth-login
// recovery vocabulary as the token-missing surface in internal/client, and the
// legacy *NeedAuthorizationError sentinel is preserved in the Cause chain for
// errors.As / errors.Is traversal.
func NewNeedUserAuthorizationError(userOpenID string) error {
	return newNeedUserAuthorizationError(userOpenID, nil, recovery.UserAuthorization())
}

// newExpiredWithoutRefreshTokenError builds the typed *errs.AuthenticationError
// returned when a stored UAT's access_token expires and no refresh_token was
// issued (typically because the app has not enabled the offline_access scope).
func newExpiredWithoutRefreshTokenError(opts UATCallOptions) error {
	reason := "access_token expired without auto-refresh because no refresh_token was issued (enable the offline_access scope in the developer console if you want automatic renewal)"
	if consoleURL := errclass.ConsoleURL(string(opts.Domain), opts.AppId, []string{"offline_access"}); consoleURL != "" {
		reason = fmt.Sprintf(
			"access_token expired without auto-refresh because no refresh_token was issued (enable the offline_access scope at %s if you want automatic renewal)",
			consoleURL,
		)
	}
	reauthHint := recovery.Join("", recovery.Command(
		recovery.TargetAuthLogin,
		"run `lark-cli auth login` to re-authorize",
	)).WithFallback(
		"re-authorize through this distribution's supported authorization flow",
	)
	return newNeedUserAuthorizationError(
		opts.UserOpenId,
		nil,
		recovery.JoinHints("; ", recovery.Join("", recovery.Text(reason)), reauthHint),
	)
}

// newNeedUserAuthorizationError preserves the missing-UAT sentinel alongside
// an optional lower-layer cause and attaches build-local recovery metadata.
func newNeedUserAuthorizationError(userOpenID string, cause error, hint recovery.Hint) error {
	needAuthCause := error(&NeedAuthorizationError{UserOpenId: userOpenID})
	if cause != nil {
		needAuthCause = errors.Join(needAuthCause, cause)
	}

	e := errs.NewAuthenticationError(errs.SubtypeTokenMissing,
		"%s (user: %s)", needUserAuthorizationMarker, userOpenID).
		WithUserOpenID(userOpenID).
		WithCause(needAuthCause)
	return recovery.Attach(e, hint)
}

// IsNeedUserAuthorizationError reports whether err represents a missing-UAT
// failure. It matches the legacy *NeedAuthorizationError sentinel, which is
// preserved in the Cause chain of the typed missing-UAT error, so errors.As
// traverses into the typed *errs.AuthenticationError as well.
func IsNeedUserAuthorizationError(err error) bool {
	if err == nil {
		return false
	}

	var needAuthErr *NeedAuthorizationError
	return errors.As(err, &needAuthErr)
}

// SecurityPolicyError is preserved as a Go type alias so existing
// errors.As(&SecurityPolicyError{}) consumers (cmd/root.go etc.) keep working.
// The concrete struct lives in errs/types.go.
type SecurityPolicyError = errs.SecurityPolicyError
