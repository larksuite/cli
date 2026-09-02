// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

var baseCodeMeta = map[int]CodeMeta{
	// Base write-path errors.
	1254291: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict, Retryable: true},

	// Copy Table domain errors (technical design chapter 18.2).
	800020304: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied},
	800010102: {Category: errs.CategoryValidation, Subtype: errs.SubtypeFailedPrecondition},
	800080105: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},
	800040819: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict},
	800070003: {Category: errs.CategoryAPI, Subtype: errs.SubtypeUnknown},
	800100112: {Category: errs.CategoryInternal, Subtype: errs.SubtypeUnknown},
	800100113: {Category: errs.CategoryInternal, Subtype: errs.SubtypeUnknown},
	800040114: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict, Retryable: true},
	800070115: {Category: errs.CategoryAPI, Subtype: errs.SubtypeUnknown},
	800010109: {Category: errs.CategoryValidation, Subtype: errs.SubtypeInvalidArgument},
	800030110: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},
	800070111: {Category: errs.CategoryAPI, Subtype: errs.SubtypeUnknown},

	// Shared RPC errors used by Copy Table (technical design chapter 18.3).
	800040802: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},
	800040803: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},
	800020812: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied},
	800040832: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},
	800040817: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},
	800080821: {Category: errs.CategoryPolicy, Subtype: errs.SubtypeAccessDenied},
	800070831: {Category: errs.CategoryAPI, Subtype: errs.SubtypeUnknown},
}

func init() {
	mergeCodeMeta(baseCodeMeta, "base")
}
