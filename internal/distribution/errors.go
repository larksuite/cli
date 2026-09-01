// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"errors"
	"os"

	"github.com/larksuite/cli/errs"
)

// classifyError maps distribution transport, protocol, and local file failures
// to the CLI error contract while preserving the original cause.
func classifyError(message string, err error) errs.TypedError {
	var typed errs.TypedError
	if errors.As(err, &typed) {
		return typed
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errs.NewInternalError(errs.SubtypeFileIO, "%s", message).WithCause(err)
	}
	return errs.NewNetworkError(errs.SubtypeNetworkProtocol, "%s", message).WithCause(err)
}
