// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// SpreadsheetRef is a Lark Sheets reference: either a raw "shtcn..." token
// or a feishu.cn/larksuite.com URL containing one. Normalize extracts the
// token from URLs; ValidateValue checks the final token shape. URL→token is
// a business-canonicalisation hint and is safe to emit to stderr.
type SpreadsheetRef string

func (s SpreadsheetRef) Normalize(_ context.Context, raw string) (SpreadsheetRef, []string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, nil
	}
	if !strings.Contains(trimmed, "://") {
		return SpreadsheetRef(trimmed), nil, nil
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if strings.HasPrefix(seg, "shtcn") {
			// Strip any ?query or #fragment suffix so a URL like
			// .../shtcnXXX?sheet=0#row=5 yields a clean token, not one
			// polluted by trailing parameters that would later pass the
			// prefix-only ValidateValue check.
			token := seg
			if i := strings.IndexAny(token, "?#"); i >= 0 {
				token = token[:i]
			}
			return SpreadsheetRef(token), []string{"extracted spreadsheet token from URL"}, nil
		}
	}
	return "", nil, &errs.ValidationError{
		Problem: errs.Problem{
			Category: errs.CategoryValidation,
			Subtype:  errs.SubtypeInvalidArgument,
			Message:  "URL does not contain a recognisable spreadsheet token",
		},
		Param: "",
	}
}

func (s SpreadsheetRef) ValidateValue(_ *common.RuntimeContext, flagName string) error {
	v := strings.TrimSpace(string(s))
	if v == "" {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "spreadsheet reference is required",
			},
			Param: flagName,
		}
	}
	if !strings.HasPrefix(v, "shtcn") {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "spreadsheet token must start with 'shtcn'",
			},
			Param: flagName,
		}
	}
	return nil
}
