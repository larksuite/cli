// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestLookupCodeMeta_DriveCodes pins each drive-service code registered via the
// codemeta_drive.go init() merge to its expected Category/Subtype/Retryable.
// Each case traces to repo evidence (see codemeta_drive.go comments).
func TestLookupCodeMeta_DriveCodes(t *testing.T) {
	cases := []struct {
		code        int
		wantCat     errs.Category
		wantSubtype errs.Subtype
		wantRetry   bool
	}{
		// 1061044: upload with a nonexistent parent folder token. The drive E2E
		// (tests_e2e/drive/2026_06_01_errs_migrate_drive_test.go) drives this
		// producer via a nonexistent parent folder → referenced resource missing.
		{1061044, errs.CategoryAPI, errs.SubtypeNotFound, false},
		// 1069302: comment endpoint's opaque "Invalid or missing parameters"
		// (shortcuts/drive/drive_add_comment.go) → API-side parameter rejection.
		{1069302, errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
		// Secure label endpoint codes observed from drive +secure-label-update
		// failure telemetry.
		{1063001, errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
		{1063002, errs.CategoryAuthorization, errs.SubtypePermissionDenied, false},
		{1063013, errs.CategoryValidation, errs.SubtypeFailedPrecondition, false},
		{99992402, errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
		{9499, errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
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
