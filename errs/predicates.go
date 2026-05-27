// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"errors"
)

// ProblemOf extracts the embedded Problem via the non-exported problemCarrier interface.
// This is the supported way to read shared fields without depending on a specific typed error.
//
// A typed error whose embedded *Problem is nil is treated as "not a problem
// carrier" — returning (nil, true) here would cause CategoryOf / IsRetryable
// and other downstream readers to dereference nil.
func ProblemOf(err error) (*Problem, bool) {
	var c problemCarrier
	if errors.As(err, &c) {
		if p := c.ProblemDetail(); p != nil {
			return p, true
		}
	}
	return nil, false
}

// UnwrapTypedError walks the wrap chain and returns the first error that
// embeds Problem (i.e. any typed error in this package). Returns the typed
// error itself (as error) so callers — notably JSON marshaling — see the
// concrete value's own struct tags rather than an opaque wrapper.
func UnwrapTypedError(err error) (error, bool) {
	var c problemCarrier
	if errors.As(err, &c) {
		if e, ok := c.(error); ok {
			return e, true
		}
	}
	return nil, false
}

// CategoryOf returns the error's Category for metrics/logging/dispatch routing.
// Falls back to CategoryInternal for non-typed errors.
func CategoryOf(err error) Category {
	if p, ok := ProblemOf(err); ok {
		return p.Category
	}
	return CategoryInternal
}

// IsRetryable reads Problem.Retryable; non-typed errors are non-retryable by default.
func IsRetryable(err error) bool {
	if p, ok := ProblemOf(err); ok {
		return p.Retryable
	}
	return false
}

// IsValidation reports whether err is a *ValidationError.
func IsValidation(err error) bool { var x *ValidationError; return errors.As(err, &x) }

// IsPermission reports whether err is a *PermissionError.
func IsPermission(err error) bool { var x *PermissionError; return errors.As(err, &x) }

// IsNetwork reports whether err is a *NetworkError.
func IsNetwork(err error) bool { var x *NetworkError; return errors.As(err, &x) }

// IsAPI reports whether err is an *APIError.
func IsAPI(err error) bool { var x *APIError; return errors.As(err, &x) }

// IsSecurityPolicy reports whether err is a *SecurityPolicyError.
func IsSecurityPolicy(err error) bool { var x *SecurityPolicyError; return errors.As(err, &x) }

// IsContentSafety reports whether err is a *ContentSafetyError.
func IsContentSafety(err error) bool { var x *ContentSafetyError; return errors.As(err, &x) }

// IsInternal reports whether err is an *InternalError.
func IsInternal(err error) bool { var x *InternalError; return errors.As(err, &x) }

// IsConfirmationRequired reports whether err is a *ConfirmationRequiredError.
func IsConfirmationRequired(err error) bool {
	var x *ConfirmationRequiredError
	return errors.As(err, &x)
}

// IsAuthentication reports whether err is an *AuthenticationError.
func IsAuthentication(err error) bool { var x *AuthenticationError; return errors.As(err, &x) }

// IsConfig reports whether err is a *ConfigError.
func IsConfig(err error) bool { var x *ConfigError; return errors.As(err, &x) }
