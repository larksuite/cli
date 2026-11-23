// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestLookupCodeMetaBaseTableCopyCodes(t *testing.T) {
	tests := []struct {
		code      int
		category  errs.Category
		subtype   errs.Subtype
		retryable bool
	}{
		// Copy Table domain errors documented in chapter 18.2.
		{code: 800020304, category: errs.CategoryAuthorization, subtype: errs.SubtypePermissionDenied},
		{code: 800010102, category: errs.CategoryValidation, subtype: errs.SubtypeFailedPrecondition},
		{code: 800080105, category: errs.CategoryAPI, subtype: errs.SubtypeQuotaExceeded},
		{code: 800040819, category: errs.CategoryAPI, subtype: errs.SubtypeConflict},
		{code: 800070003, category: errs.CategoryAPI, subtype: errs.SubtypeUnknown},
		{code: 800100112, category: errs.CategoryInternal, subtype: errs.SubtypeUnknown},
		{code: 800100113, category: errs.CategoryInternal, subtype: errs.SubtypeUnknown},
		{code: 800040114, category: errs.CategoryAPI, subtype: errs.SubtypeConflict, retryable: true},
		{code: 800070115, category: errs.CategoryAPI, subtype: errs.SubtypeUnknown},
		{code: 800010109, category: errs.CategoryValidation, subtype: errs.SubtypeInvalidArgument},
		{code: 800030110, category: errs.CategoryAPI, subtype: errs.SubtypeNotFound},
		{code: 800070111, category: errs.CategoryAPI, subtype: errs.SubtypeUnknown},

		// Shared RPC errors used by Copy Table, documented in chapter 18.3.
		{code: 800040802, category: errs.CategoryAPI, subtype: errs.SubtypeQuotaExceeded},
		{code: 800040803, category: errs.CategoryAPI, subtype: errs.SubtypeQuotaExceeded},
		{code: 800020812, category: errs.CategoryAuthorization, subtype: errs.SubtypePermissionDenied},
		{code: 800040832, category: errs.CategoryAPI, subtype: errs.SubtypeQuotaExceeded},
		{code: 800040817, category: errs.CategoryAPI, subtype: errs.SubtypeQuotaExceeded},
		{code: 800080821, category: errs.CategoryPolicy, subtype: errs.SubtypeAccessDenied},
		{code: 800070831, category: errs.CategoryAPI, subtype: errs.SubtypeUnknown},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.code), func(t *testing.T) {
			meta, ok := LookupCodeMeta(test.code)
			if !ok || meta.Category != test.category || meta.Subtype != test.subtype || meta.Retryable != test.retryable {
				t.Fatalf("LookupCodeMeta(%d) = %#v, %v", test.code, meta, ok)
			}
		})
	}
}
