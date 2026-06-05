// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// driveCodeMeta holds drive/docs-service Lark code → CodeMeta mappings.
// Only codes whose meaning is verifiable from repo evidence are registered;
// ambiguous codes fall back to CategoryAPI via BuildAPIError.
// BuildAPIError consumes this map via mergeCodeMeta + LookupCodeMeta.
var driveCodeMeta = map[int]CodeMeta{
	1061044: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},          // parent folder does not exist (upload)
	1069302: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // comment endpoint "Invalid or missing parameters"
}

func init() { mergeCodeMeta(driveCodeMeta, "drive") }
