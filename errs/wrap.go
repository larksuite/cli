// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import "errors"

// WrapInternal wraps a non-typed error into *InternalError.
// Typed errors (anything implementing problemCarrier) pass through unchanged.
// Component is metric-only and derived by the dispatcher, so it is not a parameter here.
func WrapInternal(err error) error {
	if err == nil {
		return nil
	}
	var c problemCarrier
	if errors.As(err, &c) {
		return err
	}
	return &InternalError{
		Problem: Problem{
			Category: CategoryInternal,
			Subtype:  SubtypeUnknown,
			Message:  err.Error(),
		},
		Cause: err,
	}
}
