// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestLookupCodeMeta_CredentialCodes(t *testing.T) {
	cases := []struct {
		code        int
		wantCat     errs.Category
		wantSubtype errs.Subtype
		wantRetry   bool
	}{
		{99991661, errs.CategoryAuthentication, errs.SubtypeTokenMissing, false},
		{99991671, errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},
		{99991668, errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},
		{99991663, errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},
		{99991677, errs.CategoryAuthentication, errs.SubtypeTokenExpired, false},
		{20026, errs.CategoryAuthentication, errs.SubtypeRefreshTokenInvalid, false},
		{20037, errs.CategoryAuthentication, errs.SubtypeRefreshTokenExpired, false},
		{20064, errs.CategoryAuthentication, errs.SubtypeRefreshTokenRevoked, false},
		{20073, errs.CategoryAuthentication, errs.SubtypeRefreshTokenReused, false},
		{20050, errs.CategoryAuthentication, errs.SubtypeRefreshServerError, true},
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
	if got.Subtype != errs.SubtypePermissionDenied {
		t.Errorf("Subtype = %q, want %q", got.Subtype, errs.SubtypePermissionDenied)
	}
	if got.Retryable {
		t.Errorf("Retryable = true, want false")
	}
}

func TestLookupCodeMeta_MinutesEndpointSpecificCode_NotGlobal(t *testing.T) {
	if got, ok := LookupCodeMeta(2091001); ok {
		t.Fatalf("LookupCodeMeta(2091001) = %+v, want unregistered; minutes endpoints use this code for different failures", got)
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

// TestLookupCodeMeta_ConfigCode_99991543 pins the Lark "app_id or app_secret
// is incorrect" code to CategoryConfig / SubtypeInvalidClient. The CLI cannot
// retry around a wrong app credential — the operator has to edit the local
// config — so this MUST stay non-retryable and live in the config category
// (not the API category it was originally classed under).
func TestLookupCodeMeta_ConfigCode_99991543(t *testing.T) {
	meta, ok := LookupCodeMeta(99991543)
	if !ok {
		t.Fatal("99991543 not registered in codeMeta")
	}
	if meta.Category != errs.CategoryConfig {
		t.Errorf("category = %v, want %v", meta.Category, errs.CategoryConfig)
	}
	if meta.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("subtype = %v, want %v", meta.Subtype, errs.SubtypeInvalidClient)
	}
	if meta.Retryable {
		t.Errorf("Retryable = true, want false (wrong app credential is operator-fix)")
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
		for _, needle := range []string{"1470403", "permission_denied", "intruder", "test"} {
			if !strings.Contains(msg, needle) {
				t.Errorf("panic message %q missing substring %q", msg, needle)
			}
		}
	}()
	mergeCodeMeta(map[int]CodeMeta{
		1470403: {Category: errs.CategoryAPI, Subtype: errs.Subtype("intruder")},
	}, "test")
}
