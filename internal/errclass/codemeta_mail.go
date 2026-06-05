// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// mailCodeMeta holds mail-service Lark code -> CodeMeta mappings.
// Only codes whose meaning is verifiable from repo evidence are registered;
// ambiguous codes fall back to CategoryAPI via BuildAPIError.
var mailCodeMeta = map[int]CodeMeta{
	1234013: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},      // mailbox not found or not active
	1236007: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // user daily send count exceeded
	1236008: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // user daily external recipient count exceeded
	1236009: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // tenant daily external recipient count exceeded
	1236010: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // mail quota limit
	1236013: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // tenant storage limit exceeded
}

func init() { mergeCodeMeta(mailCodeMeta, "mail") }
