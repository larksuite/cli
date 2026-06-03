// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// UserOpenID is a typed Lark user identifier with the "ou_" prefix.
type UserOpenID string

func (id UserOpenID) ValidateValue(_ *common.RuntimeContext, flagName string) error {
	s := strings.TrimSpace(string(id))
	if s == "" {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "user open_id is required",
				Hint:     "pass --user-id ou_xxx",
			},
			Param: flagName,
		}
	}
	if !strings.HasPrefix(s, "ou_") {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "invalid user open_id format: expected ou_xxx",
			},
			Param: flagName,
		}
	}
	return nil
}
