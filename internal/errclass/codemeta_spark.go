// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// sparkCodeMeta holds stable Spark business-code classifications.
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

	500002783: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},         // db sync mapping is invalid
	500002784: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},         // db sync target schema mismatch
	500002785: {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition}, // db sync operation is not allowed in the current task state
	500002786: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // db sync task does not exist
	500002787: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},         // db sync task ID is invalid
	500002788: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // db sync source table does not exist
	500002789: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // db sync target table does not exist
}

func init() { mergeCodeMeta(sparkCodeMeta, "spark") }
