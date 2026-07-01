// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// driveCodeMeta holds drive/docs-service Lark code → CodeMeta mappings.
// Only codes whose meaning is verifiable from repo evidence are registered;
// ambiguous codes fall back to CategoryAPI via BuildAPIError.
// BuildAPIError consumes this map via mergeCodeMeta + LookupCodeMeta.
var driveCodeMeta = map[int]CodeMeta{
	1061001:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // Drive "unknown error"
	1061002:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // params error
	1061004:  {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied},   // forbidden
	1061007:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                     // file has been deleted
	1061043:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},                // file size beyond limit
	1061044:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                     // parent folder does not exist (upload)
	1062009:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // actual size inconsistent with declared size
	1063001:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // secure label invalid parameter
	1063002:  {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied},   // secure label permission denied
	1063013:  {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition},    // secure label downgrade requires approval
	1069302:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // comment endpoint "Invalid or missing parameters"
	99992402: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // platform field validation failed
	9499:     {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // invalid parameter type in JSON field
	2200:     {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // Drive tenant/internal errors
}

func init() { mergeCodeMeta(driveCodeMeta, "drive") }
