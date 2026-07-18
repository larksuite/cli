// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestLookupCodeMetaSparkRoleCodes(t *testing.T) {
	tests := []struct {
		code     int
		category errs.Category
		subtype  errs.Subtype
	}{
		{3340001, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{3344027, errs.CategoryAPI, errs.SubtypeQuotaExceeded},
		{3344028, errs.CategoryAPI, errs.SubtypeQuotaExceeded},
		{3344029, errs.CategoryAPI, errs.SubtypeQuotaExceeded},
		{3344030, errs.CategoryAuthorization, errs.SubtypePermissionDenied},
		{3344031, errs.CategoryAuthorization, errs.SubtypePermissionDenied},
		{3344034, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{3344035, errs.CategoryAPI, errs.SubtypeNotFound},
		{3344036, errs.CategoryAPI, errs.SubtypeAlreadyExists},
		{3344037, errs.CategoryAPI, errs.SubtypeQuotaExceeded},
		{3344038, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{3344039, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{3344040, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{3344041, errs.CategoryAPI, errs.SubtypeInvalidParameters},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.code), func(t *testing.T) {
			meta, ok := LookupCodeMeta(tt.code)
			if !ok {
				t.Fatalf("code %d is not registered", tt.code)
			}
			if meta.Category != tt.category || meta.Subtype != tt.subtype || meta.Retryable {
				t.Fatalf("code %d metadata = %+v, want category=%s subtype=%s retryable=false", tt.code, meta, tt.category, tt.subtype)
			}

			err := BuildAPIError(map[string]any{
				"code":   tt.code,
				"msg":    "spark role error",
				"log_id": "log-spark-role",
			}, ClassifyContext{Identity: "user"})
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("BuildAPIError(%d) = %#v, want typed problem", tt.code, err)
			}
			if problem.Category != tt.category || problem.Subtype != tt.subtype || problem.Code != tt.code || problem.LogID != "log-spark-role" || problem.Retryable {
				t.Fatalf("BuildAPIError(%d) problem = %+v", tt.code, problem)
			}
		})
	}
}
