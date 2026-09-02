// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import "github.com/larksuite/cli/errs"

// minWikiResourceTokenLength is the minimum length of a complete Lark
// resource token accepted by Wiki node lookup.
const minWikiResourceTokenLength = 27

func validateWikiResourceTokenLength(token, flagName string) error {
	if len(token) >= minWikiResourceTokenLength {
		return nil
	}
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"%s is too short to be a complete Lark resource token",
		flagName,
	).WithParam(flagName).WithHint(
		"Pass the complete token or a full Lark URL; do not pass a truncated, masked, or placeholder value.",
	)
}
