// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/html/payload"
)

// publishRecoveryError adapts known payload failures to HTML recovery guidance
// while preserving the error type, metadata and cause chain.
func publishRecoveryError(err error) error {
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		return err
	}
	switch {
	case errors.Is(err, payload.ErrSensitiveFiles):
		validation.WithHint("remove the listed credential files and their references from the publication, then retry")
	case errors.Is(err, payload.ErrEntryConflict):
		validation.WithHint("rename the conflicting non-entry index.html and update its references; the entry is always published as index.html")
	case errors.Is(err, payload.ErrDependencyLimit):
		validation.WithHint("reduce the number or nesting depth of local dependencies referenced by the HTML entry, then retry")
	}
	return err
}
