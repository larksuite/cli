// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// sparkCodeMeta holds stable Spark app-role business-code classifications.
// Command-specific recovery guidance belongs in the Apps shortcut layer; the
// numeric code remains the source-specific discriminator on the error envelope.
var sparkCodeMeta = map[int]CodeMeta{
	3340001: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // request parameters are invalid
	3344027: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},              // role user count exceeds the service limit
	3344028: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},              // role department count exceeds the service limit
	3344029: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},              // role chat count exceeds the service limit
	3344030: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // app administrator required
	3344031: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // app administrator or developer required
	3344034: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // invalid role ID
	3344035: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                   // role does not exist
	3344036: {Category: errs.CategoryAPI, Subtype: errs.SubtypeAlreadyExists},              // role ID already exists
	3344037: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},              // app role count exceeds the service limit
	3344038: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // invalid role name
	3344039: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // invalid role description
	3344040: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // unsupported member type
	3344041: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // invalid member ID
}

func init() { mergeCodeMeta(sparkCodeMeta, "spark") }
