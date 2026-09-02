// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import "fmt"

// parseError is an internal parser-domain error. Command boundaries attach the
// relevant flag name and convert it to the public errs.* contract.
type parseError struct {
	message string
}

func (e *parseError) Error() string { return e.message }

func newParseError(format string, args ...any) error {
	return &parseError{message: fmt.Sprintf(format, args...)}
}
