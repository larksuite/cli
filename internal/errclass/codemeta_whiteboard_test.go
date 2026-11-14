// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestLookupCodeMeta_WhiteboardInvalidParameters(t *testing.T) {
	for _, code := range []int{2890002, 99992402} {
		meta, ok := LookupCodeMeta(code)
		if !ok {
			t.Fatalf("code %d not registered in codeMeta", code)
		}
		if meta.Category != errs.CategoryAPI || meta.Subtype != errs.SubtypeInvalidParameters || meta.Retryable {
			t.Fatalf("code %d: got %+v, want Category=%v Subtype=%v Retryable=false",
				code, meta, errs.CategoryAPI, errs.SubtypeInvalidParameters)
		}
	}
}
