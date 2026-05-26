// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

const (
	needUserAuthorizationMarker = "need_user_authorization"
)

// TokenRetryCodes contains error codes that allow retry after token refresh.
var TokenRetryCodes = map[int]bool{
	output.LarkErrTokenInvalid: true,
	output.LarkErrTokenExpired: true,
}

// NeedAuthorizationError is thrown when no valid UAT exists.
type NeedAuthorizationError struct {
	UserOpenId string
}

// Error returns the error message for NeedAuthorizationError.
func (e *NeedAuthorizationError) Error() string {
	return fmt.Sprintf("%s (user: %s)", needUserAuthorizationMarker, e.UserOpenId)
}

// IsNeedUserAuthorizationError reports whether err represents a missing-UAT
// failure, either as the original auth error or as a wrapped ExitError.
func IsNeedUserAuthorizationError(err error) bool {
	if err == nil {
		return false
	}

	var needAuthErr *NeedAuthorizationError
	if errors.As(err, &needAuthErr) {
		return true
	}

	// Deprecated: legacy *output.ExitError / string-match branches; removed after typed migration.
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil {
		return strings.Contains(exitErr.Detail.Message, needUserAuthorizationMarker)
	}
	return strings.Contains(err.Error(), needUserAuthorizationMarker)
}

// SecurityPolicyError is preserved as a Go type alias so existing
// errors.As(&SecurityPolicyError{}) consumers (cmd/root.go etc.) keep working.
// The concrete struct lives in errs/types.go.
type SecurityPolicyError = errs.SecurityPolicyError
