// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestLookupCodeMeta_CalendarCodes pins each calendar-service code registered
// via the codemeta_calendar.go init() merge to its expected
// Category/Subtype/Retryable.
func TestLookupCodeMeta_CalendarCodes(t *testing.T) {
	cases := []struct {
		code        int
		wantCat     errs.Category
		wantSubtype errs.Subtype
		wantRetry   bool
	}{
		// 190014: calendar "invalid params" with a field-level detail
		// (error.details[].value) lifted into Hint by BuildAPIError.
		{190014, errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d", tc.code), func(t *testing.T) {
			meta, ok := LookupCodeMeta(tc.code)
			if !ok {
				t.Fatalf("code %d not registered in codeMeta", tc.code)
			}
			if meta.Category != tc.wantCat || meta.Subtype != tc.wantSubtype || meta.Retryable != tc.wantRetry {
				t.Errorf("code %d: got %+v, want Category=%v Subtype=%v Retryable=%v",
					tc.code, meta, tc.wantCat, tc.wantSubtype, tc.wantRetry)
			}
		})
	}
}
