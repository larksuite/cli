// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// problemCarrier is the non-exported extraction interface.
// Used by ProblemOf via errors.As, working around the Go embed semantic where
// *Problem cannot match *PermissionError directly.
type problemCarrier interface {
	ProblemDetail() *Problem
}
