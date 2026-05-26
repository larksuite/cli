// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestLookupCodeMeta_MissingScope(t *testing.T) {
	got, ok := LookupCodeMeta(99991679)
	if !ok {
		t.Fatalf("LookupCodeMeta(99991679) ok=false, want true")
	}
	want := CodeMeta{Category: errs.CategoryAuthorization, Subtype: errs.SubtypeMissingScope, Retryable: false}
	if got != want {
		t.Fatalf("LookupCodeMeta(99991679) = %+v, want %+v", got, want)
	}
}

func TestLookupCodeMeta_TaskPermissionDenied_MergedViaInit(t *testing.T) {
	got, ok := LookupCodeMeta(1470403)
	if !ok {
		t.Fatalf("LookupCodeMeta(1470403) ok=false, want true (task sub-table init merge)")
	}
	if got.Category != errs.CategoryAuthorization {
		t.Errorf("Category = %q, want %q", got.Category, errs.CategoryAuthorization)
	}
	if got.Subtype != errs.Subtype("task_permission_denied") {
		t.Errorf("Subtype = %q, want %q", got.Subtype, "task_permission_denied")
	}
	if got.Retryable {
		t.Errorf("Retryable = true, want false")
	}
}

func TestLookupCodeMeta_RetryableAuthCode(t *testing.T) {
	got, ok := LookupCodeMeta(20050)
	if !ok {
		t.Fatalf("LookupCodeMeta(20050) ok=false, want true")
	}
	if !got.Retryable {
		t.Errorf("LookupCodeMeta(20050).Retryable = false, want true (sole retryable refresh code)")
	}
	if got.Category != errs.CategoryAuthentication {
		t.Errorf("Category = %q, want %q", got.Category, errs.CategoryAuthentication)
	}
}

func TestLookupCodeMeta_RetryableRateLimit(t *testing.T) {
	got, ok := LookupCodeMeta(99991400)
	if !ok {
		t.Fatalf("LookupCodeMeta(99991400) ok=false, want true")
	}
	if !got.Retryable {
		t.Errorf("LookupCodeMeta(99991400).Retryable = false, want true (rate_limit retryable)")
	}
	if got.Subtype != errs.SubtypeRateLimit {
		t.Errorf("Subtype = %q, want %q", got.Subtype, errs.SubtypeRateLimit)
	}
}

func TestLookupCodeMeta_Unknown(t *testing.T) {
	_, ok := LookupCodeMeta(999999)
	if ok {
		t.Fatalf("LookupCodeMeta(999999) ok=true, want false for unknown code")
	}
}

func TestLookupCodeMeta_PolicyChallengeRequired(t *testing.T) {
	got, ok := LookupCodeMeta(21000)
	if !ok {
		t.Fatalf("LookupCodeMeta(21000) ok=false, want true")
	}
	if got.Category != errs.CategoryPolicy {
		t.Errorf("Category = %q, want %q", got.Category, errs.CategoryPolicy)
	}
	if got.Subtype != errs.Subtype("challenge_required") {
		t.Errorf("Subtype = %q, want %q", got.Subtype, "challenge_required")
	}
}

func TestMergeCodeMeta_PanicsOnDuplicate(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("mergeCodeMeta with duplicate code did not panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not a string: %T (%v)", r, r)
		}
		for _, needle := range []string{"1470403", "task_permission_denied", "intruder", "test"} {
			if !strings.Contains(msg, needle) {
				t.Errorf("panic message %q missing substring %q", msg, needle)
			}
		}
	}()
	mergeCodeMeta(map[int]CodeMeta{
		1470403: {Category: errs.CategoryAPI, Subtype: errs.Subtype("intruder")},
	}, "test")
}
