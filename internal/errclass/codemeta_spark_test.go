// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
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
		{400002465, errs.CategoryValidation, errs.SubtypeFailedPrecondition},
		{500002759, errs.CategoryValidation, errs.SubtypeFailedPrecondition},
		{400002655, errs.CategoryValidation, errs.SubtypeFailedPrecondition},
		{400002469, errs.CategoryAPI, errs.SubtypeNotFound},
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

func TestLookupCodeMetaSparkDBSyncCodes(t *testing.T) {
	tests := []struct {
		code     int
		category errs.Category
		subtype  errs.Subtype
	}{
		{400002477, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{400002478, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{400002479, errs.CategoryValidation, errs.SubtypeFailedPrecondition},
		{400002480, errs.CategoryAPI, errs.SubtypeNotFound},
		{400002481, errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{400002482, errs.CategoryAPI, errs.SubtypeNotFound},
		{400002483, errs.CategoryAPI, errs.SubtypeNotFound},
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
				"msg":    "db sync error",
				"log_id": "log-db-sync",
			}, ClassifyContext{Identity: "user"})
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("BuildAPIError(%d) = %#v, want typed problem", tt.code, err)
			}
			if problem.Category != tt.category || problem.Subtype != tt.subtype || problem.Code != tt.code || problem.LogID != "log-db-sync" || problem.Retryable {
				t.Fatalf("BuildAPIError(%d) problem = %+v", tt.code, problem)
			}
		})
	}
}

// TestSparkNoDatabaseCodesExitCode pins the exit code these codes route to;
// classifying them as Validation moves it from 1 to 2.
func TestSparkNoDatabaseCodesExitCode(t *testing.T) {
	for _, code := range []int{400002465, 500002759} {
		err := BuildAPIError(map[string]any{
			"code": code,
			"msg":  "get workspace id failed by app id",
		}, ClassifyContext{Identity: "user"})
		if got := output.ExitCodeOf(err); got != 2 {
			t.Errorf("code %d exit = %d, want 2 (validation: fix state, do not retry)", code, got)
		}
	}
}

// TestSparkTableNotFoundLeavesHintToCaller keeps Hint empty here: the Apps layer
// only fills its command-scoped hint when the classifier left one empty.
func TestSparkTableNotFoundLeavesHintToCaller(t *testing.T) {
	err := BuildAPIError(map[string]any{
		"code": 400002469,
		"msg":  "数据表格不存在",
	}, ClassifyContext{Identity: "user"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("BuildAPIError = %#v, want typed problem", err)
	}
	if p.Hint != "" {
		t.Errorf("Hint = %q, want empty so the command-scoped hint still applies", p.Hint)
	}
	if got := output.ExitCodeOf(err); got != 1 {
		t.Errorf("exit = %d, want 1 (unchanged: an ordinary API lookup failure)", got)
	}
}
