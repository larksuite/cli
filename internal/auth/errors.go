// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"

	"github.com/larksuite/cli/internal/output"
)

const (
	LarkErrBlockByPolicy        = 21001 // access denied by access control policy
	LarkErrBlockByPolicyTryAuth = 21000 // access denied by access control policy; challenge is required to be completed by user in order to gain access
)

// RefreshTokenRetryable contains error codes that allow one immediate retry.
// All other refresh errors clear the token immediately.
var RefreshTokenRetryable = map[int]bool{
	output.LarkErrRefreshServerError: true,
}

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
	return fmt.Sprintf("need_user_authorization (user: %s)", e.UserOpenId)
}

// SecurityPolicyError is returned when a request is blocked by access control policies.
type SecurityPolicyError struct {
	Code         int
	Message      string
	ChallengeURL string
	CLIHint      string
	Err          error
}

// Error returns the error message for SecurityPolicyError.
func (e *SecurityPolicyError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("security policy error [%d]: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("security policy error [%d]: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *SecurityPolicyError) Unwrap() error {
	return e.Err
}
