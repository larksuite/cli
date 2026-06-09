// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package backward

import "github.com/larksuite/cli/errs"

// wrapSheetsNetworkErr preserves typed boundary errors and only classifies raw
// transport failures that still surface from stream/download paths.
func wrapSheetsNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}
