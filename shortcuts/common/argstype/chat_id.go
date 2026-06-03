// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ChatID is a typed Lark chat identifier with the "oc_" prefix.
// Satisfies common.Validatable.
type ChatID string

// ValidateValue checks the oc_ prefix and trims whitespace. Empty values are
// rejected here even though required-ness is enforced by the binder; this
// keeps the type safe to call as a standalone validator.
func (id ChatID) ValidateValue(_ *common.RuntimeContext, flagName string) error {
	s := strings.TrimSpace(string(id))
	if s == "" {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "chat ID is required",
				Hint:     "pass --chat-id oc_xxx",
			},
			Param: flagName,
		}
	}
	if !strings.HasPrefix(s, "oc_") {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "invalid chat ID format: expected oc_xxx",
			},
			Param: flagName,
		}
	}
	return nil
}
