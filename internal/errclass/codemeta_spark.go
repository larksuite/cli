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

	221800:    {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition}, // Miaoda not activated for this tenant
	400002484: {Category: errs.CategoryValidation, Subtype: errs.SubtypeInvalidArgument},    // app id does not identify an app (missing or malformed)
	400002465: {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition}, // app has no database yet
	500002759: {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition}, // app has no database yet, pre-4xx renumber
	400002655: {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition}, // app has no running container yet (online observability)
	400002469: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // table does not exist

	400002477: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},         // db sync mapping is invalid
	400002478: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},         // db sync target schema mismatch
	400002479: {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition}, // db sync operation is not allowed in the current task state
	400002480: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // db sync task does not exist
	400002481: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},         // db sync task ID is invalid
	400002482: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // db sync source table does not exist
	400002483: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                  // db sync target table does not exist

	400000034: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                   // file not found or no access
	500000034: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                   // file not found or no access, pre-4xx renumber
	400000055: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},              // tenant file storage quota exceeded
	400002467: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // app administrator or developer required
	500002761: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // app administrator or developer required, pre-4xx renumber

	// The storage permission chain used to answer two unrelated states with the
	// single "user need admin or developer permission" above: the caller lacked a
	// role, and the app simply could not be found. Only the first is a permission
	// problem, so the second now carries its own code and must not be classified
	// as authorization — an agent that reads permission_denied goes off to request
	// access, which cannot resolve a missing app.
	//
	// Storage answers a malformed app id with this same code rather than a
	// separate one, so "was never an app id" is not distinguishable here. The
	// commands that only accept a real app id therefore reject that shape before
	// the request is built (see requireFileAppID), which is also the only place
	// that can say what to do about it.
	400000008: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound}, // app not found, including a malformed app id
}

func init() { mergeCodeMeta(sparkCodeMeta, "spark") }
