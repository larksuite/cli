// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// SafePath is a strict cwd-relative file path. Absolute paths and ".."
// segments are rejected. Does NOT emit absolute path back to stderr in any
// hint (log safety).
type SafePath string

func (p SafePath) ValidateValue(_ *common.RuntimeContext, flagName string) error {
	s := string(p)
	if s == "" {
		return nil
	}
	if filepath.IsAbs(s) {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "path must be cwd-relative; absolute paths are rejected",
				Hint:     "use a relative path like ./file.txt",
			},
			Param: flagName,
		}
	}
	// Check the RAW input for ".." segments before filepath.Clean collapses
	// them — Clean turns "a/../b" into "b", which would otherwise hide a
	// parent-traversal segment the user actually typed. Split on both
	// separators so "\.." on Windows-style input is caught too.
	for _, seg := range strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return &errs.ValidationError{
				Problem: errs.Problem{
					Category: errs.CategoryValidation,
					Subtype:  errs.SubtypeInvalidArgument,
					Message:  "path must not contain '..' segments",
				},
				Param: flagName,
			}
		}
	}
	clean := filepath.Clean(s)
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") || strings.Contains(clean, `\..\`) {
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeInvalidArgument,
				Message:  "path must not contain '..' segments",
			},
			Param: flagName,
		}
	}
	return nil
}
