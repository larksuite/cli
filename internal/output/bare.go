// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import "fmt"

// BareError is the silent-exit signal for commands whose stdout already
// carries the complete answer and that only need the matching exit code
// without a stderr envelope. Two cases use it: a predicate writing its yes/no
// JSON (e.g. `auth check` exiting non-zero on a no-token state), and a command
// emitting its own structured result envelope under `--json` (e.g. `update`).
// Deliberately outside the typed-envelope contract.
type BareError struct{ Code int }

func (e *BareError) Error() string { return fmt.Sprintf("bare exit %d", e.Code) }

// ErrBare builds the silent-exit signal with the given code.
func ErrBare(code int) *BareError { return &BareError{Code: code} }
